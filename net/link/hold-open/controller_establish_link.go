package link_holdopen_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
)

// handleEstablishLink handles an EstablishLink directive.
func (c *Controller) handleEstablishLink(
	ctx context.Context,
	di directive.Instance,
	d link.EstablishLinkWithPeer,
) {
	// Register a value handler for the target peer.
	handler := newEstablishLinkHandler(c, c.le, di, d.EstablishLinkTargetPeerId())

	// Retain the directive reference until controller close.
	ref := di.AddReference(handler, true)
	if ref == nil {
		return
	}

	// Store the reference for cleanup and disposal.
	handler.ref = ref
	c.cleanupRefs = append(c.cleanupRefs, ref)
}
