package ponad

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"

	"github.com/cybozu-go/pona/internal/constants"
	"github.com/cybozu-go/pona/pkg/cnirpc"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Keys in CNI_ARGS
const (
	PodNameKey      = "K8S_POD_NAME"
	PodNamespaceKey = "K8S_POD_NAMESPACE"
	PodContainerKey = "K8S_POD_INFRA_CONTAINER_ID"
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

	listener  net.Listener
	apiReader client.Reader
}

func NewServer(l net.Listener, r client.Reader) *server {
	return &server{
		listener:  l,
		apiReader: r,
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
	podName := args.Args[PodNameKey]
	podNS := args.Args[PodNamespaceKey]
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

type gateway netip.Addr
type destination netip.Prefix
type gwToDests map[gateway][]destination

func (s *server) collectDestinationsForEgress(ctx context.Context, egName client.ObjectKey) (*gateway, []destination, error) {
	eg := &ponav1.Egress{}
	svc := &corev1.Service{}

	if err := s.apiReader.Get(ctx, egName, eg); err != nil {
		return nil, nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
			"failed to get Egress "+egName.Name, err.Error())
	}
	if err := s.client.Get(ctx, n, svc); err != nil {
		return nil, newError(codes.FailedPrecondition, cnirpc.ErrorCode_INTERNAL,
			"failed to get Service "+n.String(), err.Error())
	}

	// coil doesn't support dual stack services for now, although it's stable from k8s 1.23
	// https://kubernetes.io/docs/concepts/services-networking/dual-stack/
	svcIP := net.ParseIP(svc.Spec.ClusterIP)
	if svcIP == nil {
		return nil, newError(codes.Internal, cnirpc.ErrorCode_INTERNAL,
			"invalid ClusterIP in Service "+n.String(), svc.Spec.ClusterIP)
	}
	var subnets []*net.IPNet
	if ip4 := svcIP.To4(); ip4 != nil {
		svcIP = ip4
		for _, sn := range eg.Spec.Destinations {
			_, subnet, err := net.ParseCIDR(sn)
			if err != nil {
				return nil, newInternalError(err, "invalid network in Egress "+n.String())
			}
			if subnet.IP.To4() != nil {
				subnets = append(subnets, subnet)
			}
		}
	} else {
		for _, sn := range eg.Spec.Destinations {
			_, subnet, err := net.ParseCIDR(sn)
			if err != nil {
				return nil, newInternalError(err, "invalid network in Egress "+n.String())
			}
			if subnet.IP.To4() == nil {
				subnets = append(subnets, subnet)
			}
		}
	}

	if len(subnets) > 0 {
		gwlist = append(gwlist, GWNets{Gateway: svcIP, Networks: subnets, SportAuto: eg.Spec.FouSourcePortAuto})
	}

}

func (s *server) Del(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}

func (s *server) Check(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}
