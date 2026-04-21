package dialer

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// TransportDialer is a transport that supports dialing string-serialized remote
// addresses. The Transport controller will call Dial if provided an address for
// the transport, and directed to connect to the peer.
type TransportDialer interface {
	// MatchTransportType checks if the given transport type ID matches this transport.
	// If returns true, the transport controller will call DialPeer with that tptaddr.
	// E.x.: "udp-quic" or "ws"
	MatchTransportType(transportType string) bool

	// GetPeerDialer returns the dialing information for a peer.
	// Called when resolving EstablishLink.
	// Return nil, nil to indicate not found or unavailable.
	GetPeerDialer(
		ctx context.Context,
		peerID peer.ID,
	) (*DialerOpts, error)

	// DialPeer dials a peer given an address. The yielded link should be
	// emitted to the transport handler. DialPeer should return nil if the link
	// was established. DialPeer will then not be called again for the same peer
	// ID and address tuple until the yielded link is lost.
	//
	// ctx will be canceled once DialPeer returns. use ctx from Execute for long-lived routines.
	//
	// Returns fatal and error.
	DialPeer(
		ctx context.Context,
		peerID peer.ID,
		addr string,
	) (lnk link.Link, fatal bool, err error)
}
