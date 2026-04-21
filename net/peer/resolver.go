package peer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// GetPeerResolver resolves the GetPeer directive
type GetPeerResolver struct {
	directive GetPeer
	peer      Peer
}

// NewGetPeerResolver constructs a new GetPeer resolver
func NewGetPeerResolver(
	directive GetPeer,
	peer Peer,
) *GetPeerResolver {
	peerID := directive.GetPeerIDConstraint()
	if len(peerID) != 0 {
		npID := peer.GetPeerID()
		if npID != peerID {
			return nil
		}
	}

	return &GetPeerResolver{
		directive: directive,
		peer:      peer,
	}
}

// Resolve resolves the values.
func (c *GetPeerResolver) Resolve(ctx context.Context, valHandler directive.ResolverHandler) error {
	var val Peer = c.peer //nolint:staticcheck
	_, _ = valHandler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*GetPeerResolver)(nil))
