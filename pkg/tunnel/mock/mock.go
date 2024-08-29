package mock

import (
	"net/netip"

	"github.com/vishvananda/netlink"
)

type mockTunnel struct {
	Tunnels map[netip.Addr]struct{}
}

func NewMockTunnel() mockTunnel {
	return mockTunnel{
		Tunnels: make(map[netip.Addr]struct{}),
	}
}

func (m mockTunnel) AddPeer(addr netip.Addr) (netlink.Link, error) {
	m.Tunnels[addr] = struct{}{}
	link := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name:  "dummy",
			Index: 1,
		},
	}
	return link, nil
}

func (m mockTunnel) DelPeer(addr netip.Addr) error {
	delete(m.Tunnels, addr)
	return nil
}

func (m mockTunnel) Init() error {
	return nil
}

func (m mockTunnel) IsInitialized() bool {
	return true
}
