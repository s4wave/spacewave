package dex_solicit

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/dex"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/dex/solicit"

// DexProtocolID is the protocol ID used for solicitation.
const DexProtocolID = protocol.ID("hydra/dex")

// maxMessageSize is the max message size for packet sessions.
//
// 10MB matches block.MaxBlockSize: see that constant for the rationale on why
// 10 MiB is comfortable headroom over the largest single block any current
// producer emits (real block types stay under blob.DefChunkingMaxSize, 768
// KiB).
const maxMessageSize = 10 * 1024 * 1024

// requestTimeout is the per-peer request timeout.
const requestTimeout = 5 * time.Second

// Controller is the solicitation-based DEX controller.
type Controller struct {
	le *logrus.Entry
	b  bus.Bus
	cc *Config

	bcast broadcast.Broadcast
	// sessions tracks active peer sessions.
	// key: remote peer ID string
	// guarded by bcast
	sessions map[string]*peerSession
	// transfer counts block payloads that actually crossed a peer stream.
	transfer      TransferSnapshot
	peerTransfers map[string]PeerTransferSnapshot
}

// TransferSnapshot reports payload traffic and current peers for this controller.
// Counters exclude cached reads and protocol framing and last for its lifetime.
type TransferSnapshot struct {
	UploadedBytes   uint64
	DownloadedBytes uint64
	LastActivity    time.Time
	Peers           []PeerTransferSnapshot
}

// PeerTransferSnapshot retains this peer's traffic across stream reconnects.
type PeerTransferSnapshot struct {
	PeerID          string
	UploadedBytes   uint64
	DownloadedBytes uint64
	Connected       bool
}

// GetTransferSnapshot returns counters and a channel for the next change.
func (c *Controller) GetTransferSnapshot() (TransferSnapshot, <-chan struct{}) {
	var result TransferSnapshot
	var wait <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		result = c.transfer
		for id, peer := range c.peerTransfers {
			_, peer.Connected = c.sessions[id]
			result.Peers = append(result.Peers, peer)
		}
		for id := range c.sessions {
			if _, exists := c.peerTransfers[id]; !exists {
				result.Peers = append(result.Peers, PeerTransferSnapshot{PeerID: id, Connected: true})
			}
		}
		wait = getWait()
	})
	return result, wait
}

// recordTransfer runs outside the peer's write lock to preserve lock ordering.
func (c *Controller) recordTransfer(peerID string, uploaded, downloaded int) {
	if uploaded < 0 {
		uploaded = 0
	}
	if downloaded < 0 {
		downloaded = 0
	}
	if uploaded == 0 && downloaded == 0 {
		return
	}
	c.bcast.HoldLock(func(changed func(), _ func() <-chan struct{}) {
		c.transfer.UploadedBytes += uint64(uploaded)     //nolint:gosec // negative callback values are normalized above.
		c.transfer.DownloadedBytes += uint64(downloaded) //nolint:gosec // negative callback values are normalized above.
		c.transfer.LastActivity = time.Now()
		if c.peerTransfers == nil {
			c.peerTransfers = make(map[string]PeerTransferSnapshot)
		}
		peer := c.peerTransfers[peerID]
		peer.PeerID = peerID
		peer.UploadedBytes += uint64(uploaded)     //nolint:gosec // negative callback values are normalized above.
		peer.DownloadedBytes += uint64(downloaded) //nolint:gosec // negative callback values are normalized above.
		c.peerTransfers[peerID] = peer
		changed()
	})
}

// NewController constructs a new solicitation-based DEX controller.
func NewController(le *logrus.Entry, b bus.Bus, cc *Config) (*Controller, error) {
	return &Controller{
		le:       le,
		b:        b,
		cc:       cc,
		sessions: make(map[string]*peerSession),
	}, nil
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	// Resolve the local peer identity.
	c.le.Debug("dex solicit controller running")

	peerID, err := c.cc.ParsePeerID()
	if err != nil {
		return err
	}

	// Publish the solicitation protocol for the configured logical store.
	solicitCtx := solicitationContext(c.cc)
	dir := link_solicit.NewSolicitProtocol(
		DexProtocolID,
		solicitCtx,
		peerID,
		c.cc.GetTransportId(),
	)

	_, solicitRef, err := c.b.AddDirective(
		dir,
		directive.NewTypedCallbackHandler[link_solicit.SolicitMountedStream](
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				c.handleSolicitedStream(ctx, v.GetValue())
			},
			nil, nil, nil,
		),
	)
	if err != nil {
		return errors.Wrap(err, "add solicit protocol directive")
	}
	defer solicitRef.Release()

	// Wait for controller cancellation.
	<-ctx.Done()
	return ctx.Err()
}

// handleSolicitedStream processes a new solicited stream from a DEX peer.
func (c *Controller) handleSolicitedStream(ctx context.Context, sms link_solicit.SolicitMountedStream) {
	// Accept the solicited stream and identify the remote peer.
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil || taken {
		return
	}

	remotePeer := ms.GetPeerID().String()
	le := c.le.WithField("remote-peer", remotePeer)

	var sess *peerSession
	sess = newPeerSession(c, le, ms, func() {
		c.removeSessionIfCurrent(remotePeer, sess)
		le.Debug("dex peer session ended")
	})

	// Replace any previous session for this peer.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace existing session if any.
		if old, ok := c.sessions[remotePeer]; ok {
			old.close()
		}
		c.sessions[remotePeer] = sess
		broadcast()
	})

	// Start the peer session.
	le.Debug("dex peer session started")
	sess.start(ctx)
}

func (c *Controller) removeSessionIfCurrent(remotePeer string, sess *peerSession) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if cur, ok := c.sessions[remotePeer]; ok && cur == sess {
			delete(c.sessions, remotePeer)
			broadcast()
		}
	})
}

// forwardToPeers forwards a block request to other connected peers,
// excluding the session that originated the request. Returns the first
// verified response, or nil when no peer has the block.
func (c *Controller) forwardToPeers(ctx context.Context, ref *block.BlockRef, hops uint32, exclude *peerSession) *DexMessage {
	var sessions []*peerSession
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, s := range c.sessions {
			if s != exclude {
				sessions = append(sessions, s)
			}
		}
	})
	return peerBlockFanout{sessions: sessions, ref: ref, hops: hops}.run(ctx)
}

func (c *Controller) snapshotSessions() []*peerSession {
	var sessions []*peerSession
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, s := range c.sessions {
			sessions = append(sessions, s)
		}
	})
	return sessions
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case dex.LookupBlockFromNetwork:
		return c.resolveLookupBlockFromNetwork(ctx, di, d)
	}
	return nil, nil
}

// resolveLookupBlockFromNetwork resolves a LookupBlockFromNetwork directive.
func (c *Controller) resolveLookupBlockFromNetwork(
	_ context.Context,
	_ directive.Instance,
	dir dex.LookupBlockFromNetwork,
) ([]directive.Resolver, error) {
	ref := dir.LookupBlockFromNetworkRef()
	if ref.GetEmpty() {
		return nil, nil
	}
	return directive.Resolvers(&lookupResolver{c: c, ref: ref}), nil
}

// lookupResolver resolves a block lookup from network peers.
type lookupResolver struct {
	c   *Controller
	ref *block.BlockRef
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	found := peerBlockFanout{
		sessions: r.c.snapshotSessions(),
		ref:      r.ref,
		hops:     r.c.cc.GetMaxForwardHops(),
	}.run(ctx)
	switch {
	case found == nil:
		handler.AddValue(dex.NewLookupBlockFromNetworkValue(nil, nil))
	case found.GetRefsKnown():
		handler.AddValue(dex.NewLookupBlockFromNetworkValueWithRefs(found.GetData(), found.GetRefs()))
	default:
		handler.AddValue(dex.NewLookupBlockFromNetworkValue(found.GetData(), nil))
	}
	return nil
}

// peerBlockFanout requests one block from several peer sessions at once.
type peerBlockFanout struct {
	sessions []*peerSession
	ref      *block.BlockRef
	hops     uint32
}

// run requests the block from every session under a bounded timeout. Returns
// the first verified response, or nil when no peer has the block.
func (f peerBlockFanout) run(ctx context.Context) *DexMessage {
	if len(f.sessions) == 0 {
		return nil
	}
	reqCtx, reqCancel := context.WithTimeout(ctx, requestTimeout)
	defer reqCancel()

	// Fan out the request to every session.
	results := make(chan *DexMessage, len(f.sessions))
	for _, sess := range f.sessions {
		go func() {
			resp, err := sess.requestBlock(reqCtx, f.ref, f.hops)
			if err != nil {
				sess.le.WithError(err).Debug("dex block request failed")
			}
			results <- resp
		}()
	}

	// Return the first successful peer response.
	for range f.sessions {
		if resp := <-results; resp != nil {
			return resp
		}
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = (*lookupResolver)(nil)

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"solicitation-based data exchange controller",
	)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
