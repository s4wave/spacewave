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
	le := h.c.loggerForLink(lnk)

	luuid := lnk.GetUUID()
	remotePeer := lnk.GetRemotePeer()

	// if this is called we always store
	tpt, err := h.tpt.Await(h.ctx)
	if err != nil {
		le.WithError(err).Warn("link established while transport exited, closing link")
		go lnk.Close()
		return
	}

	// use MaybeAsync to avoid deadlocks if the transport author was not careful.
	h.c.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		execCtx := h.c.execCtx
		if execCtx == nil {
			le.Warn("link established while transport exited, closing link")
			go lnk.Close()
			return
		}

		// quick sanity check
		if remotePeer == h.c.peerID {
			le.Warn("self-dial detected, closing link")
			go lnk.Close()
			return
		}

		el, elOk := h.c.links[luuid]
		if elOk {
			if el.lnk == lnk {
				// duplicate HandleLinkEstablished call
				le.Debug("duplicate handle-link-established call")
				return
			}

			// close dupe
			le.Debug("closing existing link identical to incoming link")
			h.c.flushEstablishedLink(el, true)
			broadcast()
		}

		mlnk := newMountedLink(h.c, tpt, lnk)
		el, err := newEstablishedLink(h.c.le, execCtx, h.c.bus, lnk, mlnk, tpt, h.c)
		if err != nil {
			h.c.le.WithError(err).Warn("unable to construct established link")
			go lnk.Close()
			return
		}

		h.c.links[luuid] = el
		h.c.linksByPeerID[remotePeer] = append(h.c.linksByPeerID[remotePeer], el)

		le.Info("link established")
		broadcast()
	})
}

// HandleLinkLost is called when a link is lost.
func (h *transportHandler) HandleLinkLost(lnk link.Link) {
	h.c.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// fast path: clear by uuid
		luuid := lnk.GetUUID()
		if el, elOk := h.c.links[luuid]; elOk {
			delete(h.c.links, luuid)
			h.c.flushEstablishedLink(el, false)
			return
		}

		// slow path: equality check
		// only taken if the uuid somehow changed on the link.
		for k, l := range h.c.links {
			if l.lnk == lnk {
				delete(h.c.links, k)
				h.c.flushEstablishedLink(l, false)
				break
			}
		}
	})
}

// _ is a type assertion
var _ transport.TransportHandler = ((*transportHandler)(nil))
