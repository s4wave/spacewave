package transport_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// PeerAuthorizer decides whether a remote peer may hold a link. A nil return
// admits the peer; an error refuses it and the link is closed before any
// stream on it reaches a handler. The error text is sent to the peer when the
// link can carry it.
type PeerAuthorizer func(ctx context.Context, remotePeer peer.ID) error

// Option configures a Controller.
type Option func(*Controller)

// WithPeerAuthorizer checks every link, inbound or dialed, before it mounts.
func WithPeerAuthorizer(authorize PeerAuthorizer) Option {
	return func(c *Controller) {
		c.authorizePeer = authorize
	}
}

// Reauthorize applies the authorizer again to every mounted link and closes
// the links of peers it now refuses. A link still being authorized is checked
// again before it mounts. Call it after the set of admitted peers changes.
func (c *Controller) Reauthorize(ctx context.Context) {
	// Without an authorizer every peer stays admitted.
	if c.authorizePeer == nil {
		return
	}

	// Advance the epoch and capture the mounted links of each peer.
	links := make(map[peer.ID][]link.Link)
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.authEpoch++
		for remotePeer, els := range c.linksByPeerID {
			for _, el := range els {
				links[remotePeer] = append(links[remotePeer], el.lnk)
			}
		}
	})

	// Close the links of each refused peer, unless ctx ended the check.
	for remotePeer, peerLinks := range links {
		err := c.authorizePeer(ctx, remotePeer)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		for _, lnk := range peerLinks {
			c.loggerForLink(lnk).WithError(err).Warn("remote peer no longer authorized, closing link")
			_ = link.CloseWithReason(lnk, err.Error())
		}
	}
}
