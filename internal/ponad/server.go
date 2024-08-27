package ponad

import "github.com/cybozu-go/pona/pkg/cnirpc"

type server struct {
	cnirpc.UnimplementedCNIServer
}
