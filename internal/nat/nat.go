package nat

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/pona/internal/util/netiputil"
	"github.com/vishvananda/netlink"
)

const (
	egressTableID    = 118
	egressProtocolID = 30
	egressRulePrio   = 2000

	egressDummy = "nat-dummy"
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

var ErrIPFamilyMismatch = errors.New("no matching IP family")

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
	}, nil
}

func (c *controller) newRule(family int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.IifName = c.iface
	r.Table = egressTableID
	r.Priority = egressRulePrio
	return r
}

func (c *controller) Init() error {
	// avoid double initialization in case the program restarts
	_, err := netlink.LinkByName(egressDummy)
	if err == nil {
		return nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return err
	}

	if c.ipv4 != nil {
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		ipn := netlink.NewIPNet(netiputil.ConvNetIP(*c.ipv4))
		err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", c.iface, "-j", "MASQUERADE")
		if err != nil {
			return fmt.Errorf("failed to setup masquerade rule for IPv4: %w", err)
		}

		rule := c.newRule(netlink.FAMILY_V4)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add egress rule for IPv4: %w", err)
		}
	}
	if c.ipv6 != nil {
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		ipn := netlink.NewIPNet(netiputil.ConvNetIP(*c.ipv6))
		err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", c.iface, "-j", "MASQUERADE")
		if err != nil {
			return fmt.Errorf("failed to setup masquerade rule for IPv6: %w", err)
		}

		rule := c.newRule(netlink.FAMILY_V6)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add egress rule for IPv6: %w", err)
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = egressDummy

	if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
		return fmt.Errorf("failed to add dummy device: %w", err)
	}
	return nil
}

func (c *controller) AddClient(addr netip.Addr, link netlink.Link) error {
	// Note:
	// The following checks are not necessary in fact because,
	// prior to this point, the support for the IP family is tested
	// by FouTunnel.AddPeer().  If the test fails, then no `link`
	// is created and this method will not be called.
	// Just as a safeguard.
	if addr.Is4() && c.ipv4 == nil {
		return ErrIPFamilyMismatch
	}
	if addr.Is6() && c.ipv6 == nil {
		return ErrIPFamilyMismatch
	}

	family := netlink.FAMILY_V4
	if addr.Is6() {
		family = netlink.FAMILY_V6
	}

	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: egressTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: failed to list routes in table %d: %w", egressTableID, err)
	}

	for _, r := range routes {
		if r.Dst == nil {
			continue
		}
		if r.Dst.IP.Equal(netiputil.ConvNetIP(addr)) {
			return nil
		}
	}

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}
	if err := netlink.RouteAdd(&netlink.Route{
		Dst:       netlink.NewIPNet(netiputil.ConvNetIP(addr)),
		LinkIndex: link.Attrs().Index,
		Table:     egressTableID,
		Protocol:  egressProtocolID,
	}); err != nil {
		return fmt.Errorf("netlink: failed to add %s to table %d: %w", addr.String(), egressTableID, err)
	}

	return nil
}
