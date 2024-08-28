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

func ToIPNet(prefix netip.Prefix) net.IPNet {
	ip := FromAddr(prefix.Addr())
	return net.IPNet{IP: ip, Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen())}
}
