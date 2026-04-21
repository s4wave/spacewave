package link_solicit_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// controlStreamMountedHandler implements MountedStreamHandler for control streams.
type controlStreamMountedHandler struct {
	c *Controller
}

// HandleMountedStream handles an incoming control stream.
func (h *controlStreamMountedHandler) HandleMountedStream(
	ctx context.Context,
	ms link.MountedStream,
) error {
	lnk := ms.GetLink()
	uuid := lnk.GetLinkUUID()

	var ls *linkState
	h.c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ls = h.c.links[uuid]
	})

	if ls == nil {
		// Link not yet tracked via EstablishLinkWithPeer watcher.
		// Add it now from the mounted stream's link info.
		h.c.addLink(lnk)

		h.c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			ls = h.c.links[uuid]
		})
		if ls == nil {
			return nil
		}
	}

	sess := stream_packet.NewSession(ms.GetStream(), maxMessageSize)
	go h.c.runControlStream(ctx, ls, sess)
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*controlStreamMountedHandler)(nil))
