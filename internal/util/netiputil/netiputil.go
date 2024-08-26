package netiputil

import (
	"net"
	"net/netip"
)

func ConvNetIP(addr netip.Addr) net.IP {
	return net.IP(addr.AsSlice())
}
