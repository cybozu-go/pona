package tunnel

import (
	"errors"
	"net/netip"
)

type Tunnel interface {
	// Init starts FoU listening socket.
	Init() error

	// IsInitialized checks if this Tunnel has been initialized
	IsInitialized() bool

	// Add setups tunnel devices to the given peer and returns them.
	// If Tunnel does not setup for the IP family of the given address,
	// this returns ErrIPFamilyMismatch error.
	Add(netip.Addr) error

	// Del deletes tunnel for the peer, if any.
	Del(netip.Addr) error
}

var ErrIPFamilyMismatch = errors.New("no matching IP family")
