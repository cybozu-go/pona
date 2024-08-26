package nat

import (
	"fmt"
	"net/netip"

	"github.com/vishvananda/netlink"
)

const (
	egressTableID    = 118
	egressProtocolID = 30
	egressRulePrio   = 2000

	egressDummy = "pona-egress-dummy"
)

type Controller interface {
	Init() error
	AddClient(netip.Addr, netlink.Link) error
}

type controller struct {
	iface string
	ipv4  *netip.Addr
	ipv6  *netip.Addr
}

func NewController(iface string, ipv4, ipv6 *netip.Addr) (Controller, error) {
	if ipv4 != nil && !ipv4.Is4() {
		return nil, fmt.Errorf("invalid IPv4 address, ip=%s", ipv4.String())
	}
	if ipv6 != nil && !ipv6.Is6() {
		return nil, fmt.Errorf("invalid IPv6 address, ip=%s", ipv6.String())
	}

	return &controller{
		iface: iface,
		ipv4:  ipv4,
		ipv6:  ipv6,
	}
}

func (c *controller) Init() error {

}
func (c *controller) AddClient(addr netip.Addr, link netlink.Link) error {

}
