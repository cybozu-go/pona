package ponad

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/cybozu-go/pona/pkg/cnirpc"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
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
		}
	}

}

func (s *server) Del(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}

func (s *server) Check(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}
