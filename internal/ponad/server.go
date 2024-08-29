package ponad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	"github.com/cybozu-go/pona/internal/constants"
	"github.com/cybozu-go/pona/pkg/cni"
	"github.com/cybozu-go/pona/pkg/cnirpc"
	"github.com/cybozu-go/pona/pkg/nat"
	"github.com/cybozu-go/pona/pkg/tunnel/fou"
	"github.com/cybozu-go/pona/pkg/util/netiputil"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newError(c codes.Code, cniCode cnirpc.ErrorCode, msg, details string) error {
	st := status.New(c, msg)
	st, err := st.WithDetails(&cnirpc.CNIError{Code: cniCode, Msg: msg, Details: details})
	if err != nil {
		panic(err)
	}

	return st.Err()
}

func newInternalError(err error, msg string) error {
	return newError(codes.Internal, cnirpc.ErrorCode_INTERNAL, msg, err.Error())
}

// InterceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

type server struct {
	cnirpc.UnimplementedCNIServer

	listener   net.Listener
	apiReader  client.Reader
	egressPort int
}

func NewServer(l net.Listener, r client.Reader, egressPort int) *server {
	return &server{
		listener:   l,
		apiReader:  r,
		egressPort: egressPort,
	}
}

var _ cnirpc.CNIServer = &server{}

func (s *server) Start(ctx context.Context) error {
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		logging.UnaryServerInterceptor(InterceptorLogger(slog.Default())),
	))
	cnirpc.RegisterCNIServer(grpcServer, s)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	return grpcServer.Serve(s.listener)
}

func (s *server) Add(ctx context.Context, args *cnirpc.CNIArgs) (*cnirpc.AddResponse, error) {
	podName := args.Args[constants.PodNameKey]
	podNS := args.Args[constants.PodNamespaceKey]
	if podName == "" || podNS == "" {
		return nil, fmt.Errorf("missing pod name or namespace, args: %#v", args.Args)
	}

	pod := &corev1.Pod{}
	if err := s.apiReader.Get(ctx, client.ObjectKey{Namespace: podNS, Name: podName}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, newError(codes.NotFound, cnirpc.ErrorCode_UNKNOWN_CONTAINER, "pod not found", err.Error())
		}
		return nil, newInternalError(err, "failed to get pod")
	}

	egNames, err := s.listEgress(pod)
	if err != nil {
		return nil, newInternalError(err, "failed to list eggress from annotations")
	}
	if len(egNames) == 0 {
		return nil, nil
	}

	p, err := cni.GetPrevResult(args)
	if err != nil {
		return nil, newInternalError(err, "failed to get previous result")
	}

	var local4, local6 *netip.Addr
	for _, ipc := range p.IPs {
		ip, ok := netiputil.ToAddr(ipc.Gateway)
		if !ok {
			return nil, newInternalError(errors.New("failed to parse ip"), "failed to parse ip")
		}
		if local4 == nil && ip.Is4() {
			local4 = &ip
		}
		if local6 == nil && ip.Is6() {
			local6 = &ip
		}
	}

	ft, err := fou.NewFoUTunnelController(s.egressPort, local4, local6)
	if err != nil {
		return nil, newInternalError(err, "failed to create FoUTunnelController")
	}
	if err := ft.Init(); err != nil {
		return nil, newInternalError(err, "failed to initialize FoUTunnel")
	}
	nt, err := nat.NewNatClient(local4 != nil, local6 != nil)
	if err != nil {
		return nil, newInternalError(err, "failed to create Nat client")
	}
	if err := nt.Init(); err != nil {
		return nil, newInternalError(err, "failed to initialize Nat client")
	}

	for _, egName := range egNames {
		g, ds, err := s.collectDestinationsForEgress(ctx, egName)
		if err != nil {
			return nil, newInternalError(err, "failed to collect destinations for egress")
		}

		link, err := ft.AddPeer(netip.Addr(g))
		if err != nil {
			return nil, newInternalError(err, fmt.Sprintf("failed to add peer for %v", g))
		}
		if err := nt.UpdateRoutes(link, ds); err != nil {
			return nil, newInternalError(err, "failed to update routes")
		}
	}

	b, err := json.Marshal(p)
	if err != nil {
		return nil, newInternalError(err, "failed to marshal result")
	}

	return &cnirpc.AddResponse{Result: b}, nil
}

func (s *server) listEgress(pod *corev1.Pod) ([]client.ObjectKey, error) {
	if pod.Spec.HostNetwork {
		// pods running in the host network cannot use egress NAT.
		// In fact, such a pod won't call CNI, so this is just a safeguard.
		return nil, nil
	}

	var egNames []client.ObjectKey

	for k, v := range pod.Annotations {
		if !strings.HasPrefix(k, constants.EgressAnnotationPrefix) {
			continue
		}

		ns := k[len(constants.EgressAnnotationPrefix):]
		for _, name := range strings.Split(v, ",") {
			egNames = append(egNames, client.ObjectKey{Namespace: ns, Name: name})
		}
	}
	return egNames, nil
}

func (s *server) collectDestinationsForEgress(ctx context.Context, egName client.ObjectKey) (netip.Addr, []netip.Prefix, error) {
	eg := &ponav1beta1.Egress{}
	svc := &corev1.Service{}

	if err := s.apiReader.Get(ctx, egName, eg); err != nil {
		return netip.Addr{}, nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
			"failed to get Egress "+egName.String(), err.Error())
	}

	if err := s.apiReader.Get(ctx, egName, svc); err != nil {
		return netip.Addr{}, nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
			"failed to get Service "+egName.String(), err.Error())
	}

	// pona doesn't support dual stack services for now, although it's stable from k8s 1.23
	// https://kubernetes.io/docs/concepts/services-networking/dual-stack/
	svcIP, err := netip.ParseAddr(svc.Spec.ClusterIP)
	if err != nil {
		return netip.Addr{}, nil, newError(codes.Internal, cnirpc.ErrorCode_INTERNAL,
			"invalid ClusterIP in Service "+egName.String(), svc.Spec.ClusterIP)
	}

	var subnets []netip.Prefix
	if svcIP.Is4() {
		for _, sn := range eg.Spec.Destinations {
			prefix, err := netip.ParsePrefix(sn)
			if err != nil {
				return netip.Addr{}, nil, newInternalError(err, "invalid network in Egress "+egName.String())
			}

			if prefix.Addr().Is4() {
				subnets = append(subnets, prefix)
			}
		}
	} else if svcIP.Is6() {
		for _, sn := range eg.Spec.Destinations {
			prefix, err := netip.ParsePrefix(sn)
			if err != nil {
				return netip.Addr{}, nil, newInternalError(err, "invalid network in Egress "+egName.String())
			}

			if prefix.Addr().Is6() {
				subnets = append(subnets, prefix)
			}
		}
	} else {
		return netip.Addr{}, []netip.Prefix{}, errors.New("invalid service ip")
	}
	return svcIP, subnets, nil
}

func (s *server) Del(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {
	return nil, nil
}

func (s *server) Check(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {
	return nil, nil
}
