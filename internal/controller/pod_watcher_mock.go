package controller

import (
	"fmt"
	"net/netip"

	"github.com/cybozu-go/pona/internal/tunnel"
	"github.com/vishvananda/netlink"
)

type mockTunnel struct {
	tunnels map[netip.Addr]struct{}
}

var _ tunnel.Controller = &mockTunnel{}

func NewMockTunnel() mockTunnel {
	return mockTunnel{
		tunnels: make(map[netip.Addr]struct{}),
	}
}

func (m mockTunnel) AddPeer(addr netip.Addr) (netlink.Link, error) {
	m.tunnels[addr] = struct{}{}
	link := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name:  "dummy",
			Index: 1,
		},
	}
	return link, nil
}

func (m mockTunnel) DelPeer(addr netip.Addr) error {
	delete(m.tunnels, addr)
	return nil
}

func (m mockTunnel) Init() error {
	return nil
}

func (m mockTunnel) IsInitialized() bool {
	return true
}

type linkName string

type mockNAT struct {
	clients map[netip.Addr]linkName
}

func NewMockNat() *mockNAT {
	return &mockNAT{
		clients: make(map[netip.Addr]linkName),
	}
}

func (m *mockNAT) Init() error {
	return nil
}

func (m *mockNAT) AddClient(addr netip.Addr, link netlink.Link) error {
	if link.Attrs() == nil {
		return fmt.Errorf("link.Attrs() returns nil")
	}
	m.clients[addr] = linkName(link.Attrs().Name)
	return nil
}
