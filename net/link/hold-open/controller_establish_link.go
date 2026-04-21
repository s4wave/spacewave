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
	handler := newEstablishLinkHandler(c, c.le, di, d.EstablishLinkTargetPeerId())
	ref := di.AddReference(handler, true)
	if ref == nil {
		return
	}
	handler.ref = ref
	c.cleanupRefs = append(c.cleanupRefs, ref)
}
