package tunnel

import (
	"errors"
	"net/netip"

	"github.com/vishvananda/netlink"
)

type Controller interface {
	// Init starts FoU listening socket.
	Init() error

	// IsInitialized checks if this Controller has been initialized
	IsInitialized() bool

	// Add setups tunnel devices to the given peer and returns them.
	// If Controller does not setup for the IP family of the given address,
	// this returns ErrIPFamilyMismatch error.
	AddPeer(netip.Addr) (netlink.Link, error)

	// Del deletes tunnel for the peer, if any.
	DelPeer(netip.Addr) error
}

var ErrIPFamilyMismatch = errors.New("no matching IP family")
