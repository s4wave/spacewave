package transport_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
)

// PeerAuthorizer decides whether a remote peer may hold a link. A nil return
// admits the peer; an error refuses it and the link is closed before any
// stream on it reaches a handler.
type PeerAuthorizer func(ctx context.Context, remotePeer peer.ID) error

// Option configures a Controller.
type Option func(*Controller)

// WithPeerAuthorizer checks every link, inbound or dialed, before it mounts.
func WithPeerAuthorizer(authorize PeerAuthorizer) Option {
	return func(c *Controller) {
		c.authorizePeer = authorize
	}
}
