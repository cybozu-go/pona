package ponad

import (
	"context"

	"github.com/cybozu-go/pona/pkg/cnirpc"
	"github.com/cybozu-go/pona/pkg/tunnel"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Keys in CNI_ARGS
const (
	PodNameKey      = "K8S_POD_NAME"
	PodNamespaceKey = "K8S_POD_NAMESPACE"
	PodContainerKey = "K8S_POD_INFRA_CONTAINER_ID"
)

type server struct {
	cnirpc.UnimplementedCNIServer
	tun tunnel.Controller
}

func NewServer(tun tunnel.Controller) *server {
	return &server{
		tun: tun,
	}
}

var _ cnirpc.CNIServer = &server{}

func (s *server) Add(ctx context.Context, args *cnirpc.CNIArgs) (*cnirpc.AddResponse, error) {
	podName := args.Args[PodNameKey]
	podNS := args.Args[PodNamespaceKey]
	if podName == "" || podNS == "" {

	}
	t := s.tun.AddPeer()
}

func (s *server) Del(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}

func (s *server) Check(ctx context.Context, args *cnirpc.CNIArgs) (*emptypb.Empty, error) {

}
