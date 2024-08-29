package main

import (
	"context"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/cybozu-go/pona/internal/constants"
	"github.com/cybozu-go/pona/pkg/cnirpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
)

// PluginEnvArgs represents CNI_ARG
type PluginEnvArgs struct {
	types.CommonArgs
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

// Map returns a map[string]string
func (e PluginEnvArgs) Map() map[string]string {
	return map[string]string{
		constants.PodNamespaceKey: string(e.K8S_POD_NAMESPACE),
		constants.PodNameKey:      string(e.K8S_POD_NAME),
		constants.PodContainerKey: string(e.K8S_POD_INFRA_CONTAINER_ID),
	}
}

func makeCNIArgs(args *skel.CmdArgs) (*cnirpc.CNIArgs, error) {
	a := &PluginEnvArgs{}
	if err := types.LoadArgs(args.Args, a); err != nil {
		return nil, fmt.Errorf("failed to load args: %w", err)
	}
	return &cnirpc.CNIArgs{
		ContainerId: args.ContainerID,
		Netns:       args.Netns,
		Ifname:      args.IfName,
		Args:        a.Map(),
		Path:        args.Path,
		StdinData:   args.StdinData,
	}, nil
}

func connect(sockPath string) (*grpc.ClientConn, error) {
	dialer := &net.Dialer{}
	dialFunc := func(ctx context.Context, a string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", a)
	}
	resolver.SetDefaultScheme("passthrough")

	conn, err := grpc.NewClient(sockPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialFunc))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", sockPath, err)
	}
	return conn, nil
}
