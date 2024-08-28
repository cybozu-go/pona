package main

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/cybozu-go/pona"
)

func cmdAdd(args *skel.CmdArgs) error

func cmdDel(args *skel.CmdArgs) error

func cmdCheck(args *skel.CmdArgs) error

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Del: cmdDel, Check: cmdCheck, GC: nil, Status: nil}, version.PluginSupports("0.3.1", "0.4.0", "1.0.0"), fmt.Sprintf("coil %s", pona.Version))
}
