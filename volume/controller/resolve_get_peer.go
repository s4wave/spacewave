package volume_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// getPeerResolver resolves the GetPeer directive
type getPeerResolver struct {
	c         *Controller
	directive peer.GetPeer
}

// newGetPeerResolver constructs a new GetPeer resolver
func newGetPeerResolver(
	c *Controller,
	directive peer.GetPeer,
) directive.Resolver {
	peerID := directive.GetPeerIDConstraint()
	if len(peerID) != 0 {
		select {
		case v := <-c.volumeCh:
			c.volumeCh <- v
			if v.GetPeerID() != peerID {
				return nil
			}
		default:
		}
	}

	return &getPeerResolver{
		c:         c,
		directive: directive,
	}
}

// Resolve resolves the values.
func (c *getPeerResolver) Resolve(ctx context.Context, valHandler directive.ResolverHandler) error {
	var v volume.Volume
	select {
	case <-ctx.Done():
		return ctx.Err()
	case v = <-c.c.volumeCh:
		c.c.volumeCh <- v
	}

	peerID := c.directive.GetPeerIDConstraint()
	if len(peerID) != 0 {
		npID := v.GetPeerID()
		if npID != peerID {
			return nil
		}
	}

	_, _ = valHandler.AddValue(peer.Peer(v))
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*getPeerResolver)(nil))
