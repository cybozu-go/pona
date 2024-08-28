package nat

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"sync"

	"github.com/cybozu-go/pona/pkg/util/netiputil"
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
	UpdateRoutes(link netlink.Link, subnets []netip.Prefix) error
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
			n := netiputil.ToIPNet(defaultGW)
			r.Dst = &n
		}
		if err := netlink.RuleDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a rule: %+v, %w", r, err)
		}
	}

	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: ncTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: route list failed: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil {
			// workaround for a library issue
			n := netiputil.ToIPNet(defaultGW)
			r.Dst = &n
		}
		if err := netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("netlink: failed to delete a route in table %d: %+v, %w", ncTableID, r, err)
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

func (c *natClient) IsInitialized() (bool, error) {
	if c.ipv4 {
		// check whether exact one rule exists
		rules, err := netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{Table: ncTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v4 rule: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}
		return true, nil
	}
	if c.ipv6 {
		// check whether exact one rule exists
		rules, err := netlink.RuleListFiltered(netlink.FAMILY_V6, &netlink.Rule{Table: ncTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return false, fmt.Errorf("netlink: failed to list v6 rule: %w", err)
		}
		if len(rules) != 1 {
			return false, nil
		}
		return true, nil
	}
	return true, nil
}

func (c *natClient) UpdateRoutes(link netlink.Link, subnets []netip.Prefix) error {
	routes, err := collectRoutes(link.Attrs().Index)
	if err != nil {
		return fmt.Errorf("failed to collect routes: %w", err)
	}

	var adds []netip.Prefix
	var dels []netip.Prefix

	for _, n := range subnets {
		if _, ok := routes[n]; !ok {
			adds = append(adds, n)
		}
	}

	slices.DeleteFunc(routes, func(p netip.Prefix) bool {

	})
}

func collectRoutes(linkIndex int) (map[netip.Prefix]netlink.Route, error) {
	r4, err := collectRoute1(linkIndex, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("failed to collect routes: %w", err)
	}
	r6, err := collectRoute1(linkIndex, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("failed to collect routes: %w", err)
	}
	maps.Copy(r4, r6)
	return r4, nil
}

func collectRoute1(linkIndex, family int) (map[netip.Prefix]netlink.Route, error) {
	routes := make(map[netip.Prefix]netlink.Route)

	ro, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: ncTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, fmt.Errorf("netlink: failed to list v4 routes: %w", err)
	}
	for _, r := range ro {
		if r.LinkIndex == linkIndex && r.Dst != nil {
			d, ok := netiputil.FromIPNet(*r.Dst)
			if !ok {
				return nil, fmt.Errorf("failed to convert to netip.Addr from net.IP: %w", err)
			}
			routes[d] = r
		}
	}
	return routes, nil
}
