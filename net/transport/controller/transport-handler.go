package transport_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/sirupsen/logrus"
)

// transportHandler handles callbacks from a transport.
type transportHandler struct {
	// authorizePeer checks whether a remote peer may mount.
	authorizePeer func(context.Context, peer.ID) (uint64, error)
	// loggerForLink decorates callback logs with link identity.
	loggerForLink func(link.Link) *logrus.Entry
	// registerLink reconciles an authorized link with the live indexes.
	registerLink func(context.Context, func(context.Context) (transport.Transport, error), link.Link, uint64, func(link.Link))
	// removeLink unregisters a lost link.
	removeLink func(link.Link)
	// ctx is the context.
	ctx context.Context
	// tpt contains the transport.
	tpt *promise.Promise[transport.Transport]

	// authMtx guards authorizing.
	authMtx sync.Mutex
	// authorizing holds links whose peer is being authorized. A lost link
	// leaves the map so its authorization does not mount the link.
	authorizing map[link.Link]struct{}
}

// buildTransportHandler binds transport callbacks to the controller operations.
func (c *Controller) buildTransportHandler(ctx context.Context) *transportHandler {
	// Preserve the direct-mount path when peer authorization is disabled.
	var authorizePeer func(context.Context, peer.ID) (uint64, error)
	if c.authorizePeer != nil {
		authorizePeer = c.authorizeLinkPeer
	}

	// Bind callback operations and allocate this execution's authorization set.
	return &transportHandler{
		ctx:           ctx,
		authorizePeer: authorizePeer,
		loggerForLink: c.loggerForLink,
		registerLink:  c.mountLink,
		removeLink:    c.removeLink,
		tpt:           promise.NewPromise[transport.Transport](),
		authorizing:   make(map[link.Link]struct{}),
	}
}

// HandleLinkEstablished is called by the transport when a link is established.
// A configured peer authorizer runs off the transport's callback path, since
// the authorizer may read account state. A refused link closes without
// mounting.
func (h *transportHandler) HandleLinkEstablished(lnk link.Link) {
	// Mount directly when every peer is admitted.
	if h.authorizePeer == nil {
		h.registerLink(h.ctx, h.tpt.Await, lnk, 0, h.queueAuthorization)
		return
	}

	// Authorize the peer off the callback path.
	h.queueAuthorization(lnk)
}

// authorizeLink authorizes the remote peer of lnk, then mounts the link
// unless it was lost meanwhile. A refused link closes with the reason.
func (h *transportHandler) authorizeLink(lnk link.Link) {
	// Note the authorization epoch, then authorize the peer.
	epoch, err := h.authorizePeer(h.ctx, lnk.GetRemotePeer())

	// Drop a lost link, close a refused one, and mount the rest.
	if !h.takeAuthorizing(lnk) {
		return
	}
	if err != nil {
		h.loggerForLink(lnk).WithError(err).Warn("remote peer refused, closing link")
		_ = link.CloseWithReason(lnk, err.Error())
		return
	}
	h.registerLink(h.ctx, h.tpt.Await, lnk, epoch, h.queueAuthorization)
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

// queueAuthorization records an outstanding authorization before starting it.
func (h *transportHandler) queueAuthorization(lnk link.Link) {
	h.authMtx.Lock()
	h.authorizing[lnk] = struct{}{}
	h.authMtx.Unlock()
	go h.authorizeLink(lnk)
}

// setTransport supplies the constructed transport to waiting link callbacks.
func (h *transportHandler) setTransport(tpt transport.Transport) {
	h.tpt.SetResult(tpt, nil)
}

// HandleLinkLost is called when a link is lost.
func (h *transportHandler) HandleLinkLost(lnk link.Link) {
	// A link still awaiting authorization was never mounted.
	if h.takeAuthorizing(lnk) {
		return
	}

	// Ask the link index owner to remove the mounted link.
	h.removeLink(lnk)
}

// _ is a type assertion
var _ transport.TransportHandler = (*transportHandler)(nil)
