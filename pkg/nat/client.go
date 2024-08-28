package nat

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/vishvananda/netlink"
)

// IDs
const (
	ncProtocolID = 30

	ncTableID   = 117
	mainTableID = 254
)

// rule priorities
const (
	ncPrio = 1900
)

// special subnets
var (
	v4PrivateList = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/32"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}

	v6PrivateList = []netip.Prefix{
		netip.MustParsePrefix("fc00::/7"),
	}

	v4LinkLocal = netip.MustParsePrefix("169.254.0.0/16")
	v6LinkLocal = netip.MustParsePrefix("fe80::/10")

	v4DefaultGW = netip.MustParsePrefix("0.0.0.0/0")
	v6DefaultGW = netip.MustParsePrefix("::/0")
)

// Client configures routes for NAT Gateway
type Client interface {
	Init() error
	IsInitialized() (bool, error)
	AddEgress(link netlink.Link, subnets []netip.Prefix) error
}

type natClient struct {
	ipv4 bool
	ipv6 bool

	v4priv []*netip.Prefix
	v6priv []*netip.Prefix

	mu sync.Mutex
}

func NewNatClient(ipv4, ipv6 netip.Addr, podNodeNet []netip.Prefix) (Client, error) {
	if !ipv4.Is4() {
		return nil, errors.New("invalid ipv4 address")
	}
	if !ipv6.Is6() {
		return nil, errors.New("invalid ipv6 address")
	}

	var v4priv, v6priv []netip.Prefix
	if len(podNodeNet) > 0 {
		for _, n := range podNodeNet {
			if n.Addr().Is4() {
				v4priv = append(v4priv, n)
			} else if n.Addr().Is6() {
				v6priv = append(v6priv, n)
			}
		}
	} else {
		v4priv = v4PrivateList
		v6priv = v6PrivateList
	}

	return &natClient{
		ipv4: ipv4 != nil,
		ipv6: ipv6 != nil,

		v4priv: v4priv,
		v6priv: v4priv,
	}, nil
}

func newRuleForClient(family, table, prio int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.Table = table
	r.Priority = prio
	return r
}

func (c *natClient) clear(family int) error {
	var defaultGW netip.Prefix
	if family == netlink.FAMILY_V4 {
		defaultGW = v4DefaultGW
	} else {
		defaultGW = v6DefaultGW
	}

	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("netlink: rule list failed: %w", err)
	}
	for _, r := range rules {
		if r.Priority != ncPrio {
			continue
		}
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = net.IPNet
			defaultGW
		}
		if err := netlink.RuleDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a rule: %+v, %w", r, err)
		}
	}

	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: ncNarrowTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = defaultGW
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncNarrowTableID, r, err)
		}
	}

	routes, err = netlink.RouteListFiltered(family, &netlink.Route{Table: ncWideTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil {
			// workaround for a library issue
			r.Dst = defaultGW
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncWideTableID, r, err)
		}
	}

	return nil
}

func (c *natClient) Init() error {
	if c.ipv4 {
		if err := c.clear(netlink.FAMILY_V4); err != nil {
			return err
		}
		rule := newRuleForClient(netlink.FAMILY_V4, ncTableID, ncPrio)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add v4 natclient rule: %w", err)
		}
	}

	if c.ipv6 {
		if err := c.clear(netlink.FAMILY_V6); err != nil {
			return err
		}

		rule := newRuleForClient(netlink.FAMILY_V6, ncTableID, ncPrio)
		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink: failed to add v6 natclient rule: %w", err)
		}
	}

	return nil
}
