package cni

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/cybozu-go/pona/pkg/cnirpc"
)

type PluginConf struct {
	types.NetConf

	// Socket contains unix domain socket to communicate with coild
	Socket string `json:"socket"`
}

func ParseConfig(cniargs *cnirpc.CNIArgs) error {
	conf := &PluginConf{}

	if err := json.Unmarshal(cniargs.StdinData, conf); err != nil {
		return fmt.Errorf("failed to unmarshal NetConf: %w", err)
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		return fmt.Errorf("failed to parse prev result: %w", err)
	}
	result, err := conf.PrevResult.GetAsVersion(conf.CNIVersion)
	if err != nil {
		return fmt.Errorf("failed to")
	}
}
