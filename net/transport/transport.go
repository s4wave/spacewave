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
