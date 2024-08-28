package fou

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/pona/pkg/tunnel"
	"github.com/cybozu-go/pona/pkg/util/netiputil"
	"github.com/vishvananda/netlink"
)

// Prefixes for Foo-over-UDP tunnel link names
const (
	FoU4LinkPrefix = "fou4_"
	FoU6LinkPrefix = "fou6_"
)

const fouDummy = "fou-dummy"

func fouName(addr netip.Addr) (string, error) {
	if addr.Is4() {
		return fmt.Sprintf("%s%x", FoU4LinkPrefix, addr.As4()), nil
	} else if addr.Is6() {
		addrSlice := addr.As16()
		hash := sha1.Sum(addrSlice[:])
		return fmt.Sprintf("%s%x", FoU6LinkPrefix, hash[:4]), nil
	}
	return "", fmt.Errorf("unknown ip families ip=%s", addr.String())
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

type FouTunnelController struct {
	port   int
	local4 *netip.Addr
	local6 *netip.Addr
}

// NewFoUTunnel creates a new fouTunnel.
// port is the UDP port to receive FoU packets.
// localIPv4 is the local IPv4 address of the IPIP tunnel.  This can be nil.
// localIPv6 is the same as localIPv4 for IPv6.
func NewFoUTunnelController(port int, localIPv4, localIPv6 *netip.Addr) (*FouTunnelController, error) {
	if localIPv4 != nil && !localIPv4.Is4() {
		return nil, tunnel.ErrIPFamilyMismatch
	}
	if localIPv6 != nil && !localIPv6.Is6() {
		return nil, tunnel.ErrIPFamilyMismatch
	}
	return &FouTunnelController{
		port:   port,
		local4: localIPv4,
		local6: localIPv6,
	}, nil
}

func (t *FouTunnelController) Init() error {
	_, err := netlink.LinkByName(fouDummy)
	if err == nil {
		return nil
	}
	var linkNotFoundError netlink.LinkNotFoundError
	if !errors.As(err, &linkNotFoundError) {
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

	if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
		return fmt.Errorf("failed to add dummy device: %w", err)
	}
	return nil
}

func (t *FouTunnelController) initIPTables(p iptables.Protocol) error {
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

func (t *FouTunnelController) IsInitialized() bool {
	_, err := netlink.LinkByName(fouDummy)
	return err == nil
}

func (t *FouTunnelController) AddPeer(addr netip.Addr) (netlink.Link, error) {
	if addr.Is4() {
		return t.addPeer4(addr)
	} else if addr.Is6() {
		return t.addPeer6(addr)
	}
	return nil, fmt.Errorf("unknown ip families ip=%s", addr.String())
}

func (t *FouTunnelController) addPeer4(addr netip.Addr) (netlink.Link, error) {
	if t.local4 == nil {
		return nil, tunnel.ErrIPFamilyMismatch
	}

	linkname, err := fouName(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate fou name: %w", err)
	}
	link, err := netlink.LinkByName(linkname)
	if err == nil {
		// if already exists, return old link
		return link, nil
	} else {
		var linkNotFoundError netlink.LinkNotFoundError
		if !errors.As(err, &linkNotFoundError) {
			return nil, fmt.Errorf("netlink: failed to get link by name: %w", err)
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkname
	link = &netlink.Iptun{
		LinkAttrs:  attrs,
		Ttl:        64,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: 0, // sportauto is always on
		Remote:     netiputil.FromAddr(addr),
		Local:      netiputil.FromAddr(*t.local4),
	}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("netlink: failed to add fou link: %w", err)
	}

	if err := setupFlowBasedIP4TunDevice(); err != nil {
		return nil, fmt.Errorf("netlink: failed to setup ipip device: %w", err)
	}

	return link, nil
}

func (t *FouTunnelController) addPeer6(addr netip.Addr) (netlink.Link, error) {
	if t.local6 == nil {
		return nil, tunnel.ErrIPFamilyMismatch
	}

	linkname, err := fouName(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate fou name: %w", err)
	}
	link, err := netlink.LinkByName(linkname)
	if err == nil {
		// if already exists, return old link
		return link, nil
	} else {
		var linkNotFoundError netlink.LinkNotFoundError
		if !errors.As(err, &linkNotFoundError) {
			return nil, fmt.Errorf("netlink: failed to get link by name: %w", err)
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = linkname
	link = &netlink.Iptun{
		LinkAttrs:  attrs,
		Ttl:        64,
		EncapType:  netlink.FOU_ENCAP_DIRECT,
		EncapDport: uint16(t.port),
		EncapSport: 0, // sportauto is always on
		Remote:     netiputil.FromAddr(addr),
		Local:      netiputil.FromAddr(*t.local6),
	}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("netlink: failed to add fou link: %w", err)
	}

	if err := setupFlowBasedIP6TunDevice(); err != nil {
		return nil, fmt.Errorf("netlink: failed to setup ipip device: %w", err)
	}

	return link, nil
}

func (t *FouTunnelController) DelPeer(addr netip.Addr) error {
	linkName, err := fouName(addr)
	if err != nil {
		return fmt.Errorf("failed to generate fou name: %w", err)
	}

	link, err := netlink.LinkByName(linkName)
	if err != nil {
		var linkNotFoundError netlink.LinkNotFoundError
		if errors.As(err, &linkNotFoundError) {
			return nil
		}
		return fmt.Errorf("failed to delete interface: %w", err)
	}
	return netlink.LinkDel(link)
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
	if err := renameDevice("tunl0", "pona_tunl"); err != nil {
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

// setupDevice creates and configures a device based on the given netlink attrs.
func setupDevice(link netlink.Link) error {
	name := link.Attrs().Name

	// Reuse existing tunnel interface created by previous runs.
	l, err := netlink.LinkByName(name)
	if err != nil {
		var linkNotFoundError netlink.LinkNotFoundError
		if !errors.As(err, &linkNotFoundError) {
			return err
		}

		if err := netlink.LinkAdd(link); err != nil {
			return fmt.Errorf("netlink: failed to create device %s: %w", name, err)
		}

		// Fetch the link we've just created.
		l, err = netlink.LinkByName(name)
		if err != nil {
			return fmt.Errorf("netlink: failed to retrieve created device %s: %w", name, err)
		}
	}

	if err := configureDevice(l); err != nil {
		return fmt.Errorf("failed to set up device %s: %w", l.Attrs().Name, err)
	}

	return nil
}

// configureDevice puts the given link into the up state
func configureDevice(link netlink.Link) error {
	ifName := link.Attrs().Name

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to set link %s up: %w", ifName, err)
	}
	return nil
}

// renameDevice renames a network device from and to a given value. Returns nil
// if the device does not exist.
func renameDevice(from, to string) error {
	link, err := netlink.LinkByName(from)
	if err != nil {
		return nil
	}

	if err := netlink.LinkSetName(link, to); err != nil {
		return fmt.Errorf("netlink: failed to rename device %s to %s: %w", from, to, err)
	}

	return nil
}
