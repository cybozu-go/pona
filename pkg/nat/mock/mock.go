package mock

import (
	"fmt"
	"net/netip"

	"github.com/vishvananda/netlink"
)

type linkName string

type mockNAT struct {
	Clients map[netip.Addr]linkName
}

func NewMockNat() *mockNAT {
	return &mockNAT{
		Clients: make(map[netip.Addr]linkName),
	}
}

func (m *mockNAT) Init() error {
	return nil
}

func (m *mockNAT) AddClient(addr netip.Addr, link netlink.Link) error {
	if link.Attrs() == nil {
		return fmt.Errorf("link.Attrs() returns nil")
	}
	m.Clients[addr] = linkName(link.Attrs().Name)
	return nil
}
