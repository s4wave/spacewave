// Package transport defines how a peer reaches other peers.
//
// A Transport owns one way of carrying packets, such as UDP, WebSocket,
// WebRTC or an in-process network, and yields authenticated Links to its
// TransportHandler: HandleLinkEstablished when a link is up and HandleLinkLost
// once when it ends. Its UUID is unique on the host and is part of every link
// it produces. The handler is usually the transport controller
// (transport/controller), which mounts each link, negotiates the protocol of
// each stream, and admits or refuses a remote peer through its PeerAuthorizer
// before any stream on the link reaches a handler.
//
// Every transport ends in a QUIC link: common/pconn runs QUIC over a packet
// connection (udp, inproc), common/conn over a stream connection (websocket),
// and webrtc over a data channel. The TLS handshake binds the link to both
// peers' keys, so a Link's remote peer ID is authenticated by construction.
// LookupTransport finds a running transport by peer or transport ID.
package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// Transport is similar to a NIC, yielding links to remote peers.
type Transport interface {
	// Execute executes the transport as configured, returning any fatal error.
	Execute(ctx context.Context) error

	// GetUUID returns a host-unique ID for this transport.
	GetUUID() uint64
	// GetPeerID returns the peer ID.
	GetPeerID() peer.ID

	// Close closes the transport, returning any errors closing.
	Close() error
}

// TransportHandler manages a Transport and receives event callbacks.
// This is typically fulfilled by the transport controller.
type TransportHandler interface {
	// HandleLinkEstablished is called when a link is established.
	HandleLinkEstablished(lnk link.Link)
	// HandleLinkLost is called when a link is lost.
	HandleLinkLost(lnk link.Link)
}

// Controller is a transport controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller

	// GetTransport returns the controlled transport.
	// This may wait for the controller to be ready.
	GetTransport(ctx context.Context) (Transport, error)
}
