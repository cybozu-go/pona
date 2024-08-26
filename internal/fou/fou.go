package fou

import (
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"sync"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/pona/internal/tunnel"
	"github.com/vishvananda/netlink"
)

const fouDummy = "fou-dummy"

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

	mu sync.Mutex
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
	t.mu.Lock()
	defer t.mu.Unlock()

	if addr.Is4() {
		return t.addPeer4(addr)
	} else if addr.Is6() {
		return t.addPeer6(addr)
	}
	return errors.New("unknown ip families")
}

func (t *fouTunnel) addPeer4(addr netip.Addr) (netlink.Link, error) {

}
func (t *fouTunnel) addPeer6(addr netip.Addr) (netlink.Link, error)
