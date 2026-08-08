package transport_controller

import (
	"context"

	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport"
)

// transportHandler handles callbacks from a transport.
type transportHandler struct {
	// c is the controller
	c *Controller
	// ctx is the context
	ctx context.Context
	// tpt contains the transport
	tpt *promise.Promise[transport.Transport]
}

// newTransportHandler constructs the transport handler.
func newTransportHandler(ctx context.Context, c *Controller) *transportHandler {
	return &transportHandler{ctx: ctx, c: c, tpt: promise.NewPromise[transport.Transport]()}
}

// HandleLinkEstablished is called by the transport when a link is established.
func (h *transportHandler) HandleLinkEstablished(lnk link.Link) {
	// Capture link identity for registration and logging.
	le := h.c.loggerForLink(lnk)

	luuid := lnk.GetUUID()
	remotePeer := lnk.GetRemotePeer()

	// Await the constructed transport before accepting the link.
	tpt, err := h.tpt.Await(h.ctx)
	if err != nil {
		le.WithError(err).Warn("link established while transport exited, closing link")
		go lnk.Close()
		return
	}

	// Reconcile the link under the controller broadcast lock.
	h.c.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Require an active execution context before registering links.
		execCtx := h.c.execCtx
		if execCtx == nil {
			le.Warn("link established while transport exited, closing link")
			go lnk.Close()
			return
		}

		// Reject self-dialed links before registration.
		if remotePeer == h.c.peerID {
			le.Warn("self-dial detected, closing link")
			go lnk.Close()
			return
		}

		el, elOk := h.c.links[luuid]
		if elOk {
			if el.lnk == lnk {
				// Ignore duplicate callbacks for the same link instance.
				le.Debug("duplicate handle-link-established call")
				return
			}

			// Close the previous link before replacing it.
			le.Debug("closing existing link identical to incoming link")
			h.c.flushEstablishedLink(el, true)
			broadcast()
		}

		// Build the mounted and established link state.
		mlnk := newMountedLink(h.c, tpt, lnk)
		el, err := newEstablishedLink(h.c.le, execCtx, h.c.bus, lnk, mlnk, tpt, h.c)
		if err != nil {
			h.c.le.WithError(err).Warn("unable to construct established link")
			go lnk.Close()
			return
		}

		// Publish the new link in both indexes.
		h.c.links[luuid] = el
		h.c.linksByPeerID[remotePeer] = append(h.c.linksByPeerID[remotePeer], el)

		le.Info("link established")
		broadcast()
	})
}

// HandleLinkLost is called when a link is lost.
func (h *transportHandler) HandleLinkLost(lnk link.Link) {
	h.c.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Remove the link by UUID on the common loss path.
		luuid := lnk.GetUUID()
		if el, elOk := h.c.links[luuid]; elOk {
			delete(h.c.links, luuid)
			h.c.flushEstablishedLink(el, false)
			broadcast()
			return
		}

		// Fall back to identity comparison if the UUID changed.
		for k, l := range h.c.links {
			if l.lnk == lnk {
				delete(h.c.links, k)
				h.c.flushEstablishedLink(l, false)
				broadcast()
				break
			}
		}
	})
}

// _ is a type assertion
var _ transport.TransportHandler = (*transportHandler)(nil)
