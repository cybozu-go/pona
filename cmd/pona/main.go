package main

import (
	"context"
	"fmt"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"

	"github.com/containernetworking/cni/pkg/version"
	"github.com/cybozu-go/pona"
	"github.com/cybozu-go/pona/pkg/cni"
	"github.com/cybozu-go/pona/pkg/cnirpc"
)

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := cni.ParseConfig(args.StdinData)
	if err != nil {
		return types.NewError(types.ErrDecodingFailure, "failed to parse config from stdin data", err.Error())
	}
	if conf.PrevResult == nil {
		return types.NewError(types.ErrInternal, "ponad must be called as chained plugin", "")
	}

	cniArgs, err := makeCNIArgs(args)
	if err != nil {
		return types.NewError(types.ErrInvalidNetworkConfig, "failed to transform args to RPC arg", err.Error())
	}

	conn, err := connect(conf.Socket)
	if err != nil {
		return types.NewError(types.ErrTryAgainLater, "failed to connect to socket", err.Error())
	}

	client := cnirpc.NewCNIClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	resp, err := client.Add(ctx, cniArgs)
	if err != nil {
		return convertError(err)
	}

	result, err := cni100.NewResult(resp.Result)
	if err != nil {
		return types.NewError(types.ErrDecodingFailure, "failed to unmarshal result", err.Error())
	}

	return types.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Del: cmdDel, Check: cmdCheck, GC: nil, Status: nil}, version.PluginSupports("0.3.1", "0.4.0", "1.0.0", "1.1.0"), fmt.Sprintf("pona %s", pona.Version))
}
