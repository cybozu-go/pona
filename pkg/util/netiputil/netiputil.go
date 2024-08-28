package netiputil

import (
	"net"
	"net/netip"
)

func FromAddr(addr netip.Addr) net.IP {
	return net.IP(addr.AsSlice())
}

func ToAddr(ip net.IP) (netip.Addr, bool) {
	return netip.AddrFromSlice(ip)
}

func IsFamilyMatched(a, b netip.Addr) bool {
	return (a.Is4() && b.Is4()) || (a.Is6() && b.Is6())
}
