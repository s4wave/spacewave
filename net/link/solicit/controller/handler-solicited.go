package link_solicit_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
)

// solicitedStreamMountedHandler implements MountedStreamHandler for solicited streams.
type solicitedStreamMountedHandler struct {
	c       *Controller
	hashHex string
}

// HandleMountedStream handles an incoming solicited stream.
func (h *solicitedStreamMountedHandler) HandleMountedStream(
	_ context.Context,
	ms link.MountedStream,
) error {
	h.c.handleIncomingSolicitedStream(h.hashHex, ms)
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*solicitedStreamMountedHandler)(nil))
