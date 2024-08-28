package cni

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/cybozu-go/pona/pkg/cnirpc"
)

type PluginConf struct {
	types.NetConf

	// Socket contains unix domain socket to communicate with coild
	Socket string `json:"socket"`
}

func GetPrevResult(cniargs *cnirpc.CNIArgs) (*cni100.Result, error) {
	conf := &PluginConf{}

	if err := json.Unmarshal(cniargs.StdinData, conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal NetConf: %w", err)
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return nil, fmt.Errorf("failed to parse prev result: %w", err)
	}
	r, err := cni100.GetResult(conf.NetConf.PrevResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get prevresult")
	}

	return r, nil
}
