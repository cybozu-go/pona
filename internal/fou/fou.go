package fou

import (
	"net/netip"

	"github.com/vishvananda/netlink"
)

type Tunnel interface {
	// Init starts FoU listening socket.
	Init() error

	// IsInitialized checks if this Tunnel has been initialized
	IsInitialized() bool

	// Add setups tunnel devices to the given peer and returns them.
	// If Tunnel does not setup for the IP family of the given address,
	// this returns ErrIPFamilyMismatch error.
	Add(netip.Addr, bool) (netlink.Link, error)

	// Del deletes tunnel for the peer, if any.
	Del(netip.Addr) error
}
