package transport_controller

import (
	"context"
	"sync"

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

	// authMtx guards authorizing.
	authMtx sync.Mutex
	// authorizing holds links whose peer is being authorized; a lost link
	// leaves it so its authorization does not mount it.
	authorizing map[link.Link]struct{}
}

// newTransportHandler constructs the transport handler.
func newTransportHandler(ctx context.Context, c *Controller) *transportHandler {
	return &transportHandler{
		ctx:         ctx,
		c:           c,
		tpt:         promise.NewPromise[transport.Transport](),
		authorizing: make(map[link.Link]struct{}),
	}
}

// HandleLinkEstablished is called by the transport when a link is established.
// A configured peer authorizer runs off the transport's callback path, since
// it may read account state; a refused link closes without mounting.
func (h *transportHandler) HandleLinkEstablished(lnk link.Link) {
	// Mount directly when every peer is admitted.
	if h.c.authorizePeer == nil {
		h.mountLink(lnk, 0)
		return
	}

	// Authorize the peer off the callback path.
	h.authMtx.Lock()
	h.authorizing[lnk] = struct{}{}
	h.authMtx.Unlock()
	go h.authorizeLink(lnk)
}

// authorizeLink authorizes the remote peer of lnk, then mounts the link
// unless it was lost meanwhile. A refused link closes with the reason.
func (h *transportHandler) authorizeLink(lnk link.Link) {
	// Note the authorization epoch, then authorize the peer.
	var epoch uint64
	h.c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		epoch = h.c.authEpoch
	})
	err := h.c.authorizePeer(h.ctx, lnk.GetRemotePeer())

	// Drop a lost link, close a refused one, and mount the rest.
	if !h.takeAuthorizing(lnk) {
		return
	}
	if err != nil {
		h.c.loggerForLink(lnk).WithError(err).Warn("remote peer refused, closing link")
		_ = link.CloseWithReason(lnk, err.Error())
		return
	}
	h.mountLink(lnk, epoch)
}

// takeAuthorizing removes lnk from the links awaiting authorization and
// reports whether it was there.
func (h *transportHandler) takeAuthorizing(lnk link.Link) bool {
	// Remove the entry under the lock that HandleLinkLost shares.
	h.authMtx.Lock()
	defer h.authMtx.Unlock()
	_, ok := h.authorizing[lnk]
	delete(h.authorizing, lnk)
	return ok
}

// mountLink registers an admitted link with the controller. epoch is the
// authorization epoch the admission read; a link admitted under an older
// epoch is authorized again instead.
func (h *transportHandler) mountLink(lnk link.Link, epoch uint64) {
	// Capture link identity for registration and logging.
	le := h.c.loggerForLink(lnk)

	// Capture the incoming link identity for the controller indexes.
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

		// Authorize again when the admitted peers changed meanwhile.
		if h.c.authEpoch != epoch {
			h.authMtx.Lock()
			h.authorizing[lnk] = struct{}{}
			h.authMtx.Unlock()
			go h.authorizeLink(lnk)
			return
		}

		// Reconcile an existing registration before accepting the incoming link.
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

		// Report the established link and wake transport state subscribers.
		le.Info("link established")
		broadcast()
	})
}

// HandleLinkLost is called when a link is lost.
func (h *transportHandler) HandleLinkLost(lnk link.Link) {
	// A link still awaiting authorization was never mounted.
	if h.takeAuthorizing(lnk) {
		return
	}

	// Unregister the mounted link.
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
