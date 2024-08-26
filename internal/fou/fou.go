package fou

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/pona/internal/tunnel"
	"github.com/vishvananda/netlink"
)

// Prefixes for Foo-over-UDP tunnel link names
const (
	FoU4LinkPrefix = "fou4_"
	FoU6LinkPrefix = "fou6_"
)

const fouDummy = "fou-dummy"

func convNetIP(addr netip.Addr) net.IP {
	return net.IP(addr.AsSlice())
}

func fouName(addr netip.Addr) string {
	if addr.Is4() {
		return fmt.Sprintf("%s%x", FoU4LinkPrefix, addr.As4())
	}

	addrSlice := addr.As16()
	hash := sha1.Sum(addrSlice[:])
	return fmt.Sprintf("%s%x", FoU6LinkPrefix, hash[:4])
}

func modProbe(module string) error {
	out, err := exec.Command("/sbin/modprobe", module).CombinedOutput()
	if err != nil {
		return fmt.Errorf("modprobe %s failed with %w: %s", module, err, string(out))
	}
	return nil
}

func disableRPFilter() error {
	if _, err := sysctl.Sysctl("net.ipv4.conf.default.rp_filter", "0"); err != nil {
		return fmt.Errorf("setting net.ipv4.conf.default.rp_filter=0 failed: %w", err)
	}
	if _, err := sysctl.Sysctl("net.ipv4.conf.all.rp_filter", "0"); err != nil {
		return fmt.Errorf("setting net.ipv4.conf.all.rp_filter=0 failed: %w", err)
	}
	return nil
}

type fouTunnel struct {
	port   int
	local4 *netip.Addr
	local6 *netip.Addr
}

var _ tunnel.Tunnel = &fouTunnel{}

func (t *fouTunnel) Init() error {
	_, err := netlink.LinkByName(fouDummy)
	if err == nil {
		return nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return fmt.Errorf("failed to initialize fou tunnel: %w", err)
	}

	if t.local4 != nil {
		if err := disableRPFilter(); err != nil {
			return fmt.Errorf("failed to disable RP Filter: %w", err)
		}
		if err := ip.EnableIP4Forward(); err != nil {
			return fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
		}

		if err := modProbe("fou"); err != nil {
			return fmt.Errorf("failed to load fou module: %w", err)
		}
		err := netlink.FouAdd(netlink.Fou{
			Family:    netlink.FAMILY_V4,
			Protocol:  4, // IPv4 over IPv4
			Port:      t.port,
			EncapType: netlink.FOU_ENCAP_DIRECT,
		})
		if err != nil {
			return fmt.Errorf("netlink: fou addlink failed: %w", err)
		}

		if err := t.initIPTables(iptables.ProtocolIPv4); err != nil {
			return err
		}
	}
	if t.local6 != nil {
		if err := ip.EnableIP6Forward(); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
		}

		if err := modProbe("fou6"); err != nil {
			return fmt.Errorf("failed to load fou module: %w", err)
		}
		err := netlink.FouAdd(netlink.Fou{
			Family:    netlink.FAMILY_V6,
			Protocol:  41, // IPv6 over IPv6
			Port:      t.port,
			EncapType: netlink.FOU_ENCAP_DIRECT,
		})
		if err != nil {
			return fmt.Errorf("netlink: fou addlink failed: %w", err)
		}

		if err := t.initIPTables(iptables.ProtocolIPv6); err != nil {
			return err
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = fouDummy
	return netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs})

}

func (t *fouTunnel) initIPTables(p iptables.Protocol) error {
	ipt, err := iptables.NewWithProtocol(p)
	if err != nil {
		return err
	}
	// workaround for kube-proxy's double NAT problem
	rulespec := []string{
		"-p", "udp", "--dport", strconv.Itoa(t.port), "-j", "CHECKSUM", "--checksum-fill",
	}
	if err := ipt.Insert("mangle", "POSTROUTING", 1, rulespec...); err != nil {
		return fmt.Errorf("failed to setup mangle table: %w", err)
	}

	return nil
}

func (t *fouTunnel) IsInitialized() bool {
	_, err := netlink.LinkByName(fouDummy)
	return err == nil
}

func (t *fouTunnel) AddPeer(addr netip.Addr) (netlink.Link, error) {
	if addr.Is4() {
		return t.addPeer4(addr)
	} else if addr.Is6() {
		return t.addPeer6(addr)
	}
	return nil, errors.New("unknown ip families")
}

func (t *fouTunnel) addPeer4(addr netip.Addr) (netlink.Link, error) {
	if t.local4 == nil {
		return nil, tunnel.ErrIPFamilyMismatch
	}

	linkname := fouName(addr)

	link, err := netlink.LinkByName(linkname)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); !ok {
			return nil, fmt.Errorf("netlink: failed to get link by name: %w", err)
		}
		// ignore LinkNotFoundError
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkname
	link = &netlink.Iptun{
		LinkAttrs:  attrs,
		Ttl:        64,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: 0, // sportauto is always on
		Remote:     convNetIP(addr),
		Local:      convNetIP(*t.local4),
	}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("netlink: failed to add fou link: %w", err)
	}

	return link, nil
}
func (t *fouTunnel) addPeer6(addr netip.Addr) (netlink.Link, error) {

}

// setupFlowBasedIP[4,6]TunDevice creates an IPv4 or IPv6 tunnel device
//
// This flow based IPIP tunnel device is used to decapsulate packets from
// the router Pods.
//
// Calling this function may result in tunl0 (v4) or ip6tnl0 (v6)
// fallback interface being renamed to coil_tunl or coil_ip6tnl.
// This is to communicate to the user that this plugin has taken
// control of the encapsulation stack on the netns, as it currently
// doesn't explicitly support sharing it with other tools/CNIs.
// Fallback devices are left unused for production traffic.
// Only devices that were explicitly created are used.
//
// This fallback interface is present as a result of loading the
// ipip and ip6_tunnel kernel modules by fou tunnel interfaces.
// These are catch-all interfaces for the ipip decapsulation stack.
// By default, these interfaces will be created in new network namespaces,
// but this behavior can be disabled by setting net.core.fb_tunnels_only_for_init_net = 2.
func setupFlowBasedIP4TunDevice() error {
	ipip4Device := "coil_ipip4"
	// Set up IPv4 tunnel device if requested.
	if err := setupDevice(&netlink.Iptun{
		LinkAttrs: netlink.LinkAttrs{Name: ipip4Device},
		FlowBased: true,
	}); err != nil {
		return fmt.Errorf("creating %s: %w", ipip4Device, err)
	}

	// Rename fallback device created by potential kernel module load after
	// creating tunnel interface.
	if err := renameDevice("tunl0", "coil_tunl"); err != nil {
		return fmt.Errorf("renaming fallback device %s: %w", "tunl0", err)
	}

	return nil
}

// See setupFlowBasedIP4TunDevice
func setupFlowBasedIP6TunDevice() error {
	ipip6Device := "coil_ipip6"

	// Set up IPv6 tunnel device if requested.
	if err := setupDevice(&netlink.Ip6tnl{
		LinkAttrs: netlink.LinkAttrs{Name: ipip6Device},
		FlowBased: true,
	}); err != nil {
		return fmt.Errorf("creating %s: %w", ipip6Device, err)
	}

	// Rename fallback device created by potential kernel module load after
	// creating tunnel interface.
	if err := renameDevice("ip6tnl0", "coil_ip6tnl"); err != nil {
		return fmt.Errorf("renaming fallback device %s: %w", "tunl0", err)
	}

	return nil
}
