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
	// sessionStarts counts the peer sessions started, guarded by bcast.
	sessionStarts uint64
	// settled reports that the solicitation has delivered a session for every
	// peer it matched, guarded by bcast.
	settled bool
	// transfer counts block payloads that actually crossed a peer stream.
	transfer      TransferSnapshot
	peerTransfers map[string]PeerTransferSnapshot
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

// GetSessionStarts returns how many peer sessions have started.
func (c *Controller) GetSessionStarts() uint64 {
	var starts uint64
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		starts = c.sessionStarts
	})
	return starts
}

// WaitSessionStart waits until a peer session starts after GetSessionStarts
// returned starts. A block every connected peer lacks may arrive with a peer
// that connects later.
func (c *Controller) WaitSessionStart(ctx context.Context, starts uint64) error {
	return c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return c.sessionStarts != starts, nil
	})
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
		// Update the controller payload totals and activity time.
		c.transfer.UploadedBytes += uint64(uploaded)     //nolint:gosec // negative callback values are normalized above.
		c.transfer.DownloadedBytes += uint64(downloaded) //nolint:gosec // negative callback values are normalized above.
		c.transfer.LastActivity = time.Now()

		// Retain the peer payload totals and notify transfer watchers.
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
	di, solicitRef, err := c.b.AddDirective(
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

	// Track settlement. Idle callbacks follow the value callbacks queued
	// before them, so an idle solicitation has registered its sessions. Once
	// the controller stops, no session can arrive.
	defer c.setSettled(true)
	releaseIdle := di.AddIdleCallback(func(idle bool, _ []error) {
		c.setSettled(idle)
	})
	defer releaseIdle()

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

	// Identify the accepted stream in peer session logs.
	remotePeer := ms.GetPeerID().String()
	le := c.le.WithField("remote-peer", remotePeer)

	// Construct the replacement session with cleanup bound to its identity.
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
		c.sessionStarts++
		broadcast()
	})

	// Start the peer session.
	le.Debug("dex peer session started")
	sess.start(ctx)
}

// setSettled records whether the solicitation has settled.
func (c *Controller) setSettled(settled bool) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.settled != settled {
			c.settled = settled
			broadcast()
		}
	})
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
// excluding the session that originated the request. It returns what
// peerBlockFanout.run returns.
func (c *Controller) forwardToPeers(ctx context.Context, ref *block.BlockRef, hops uint32, exclude *peerSession) (*DexMessage, error) {
	sessions, err := c.waitSessions(ctx, exclude, false)
	if err != nil {
		return nil, err
	}
	return peerBlockFanout{sessions: sessions, ref: ref, hops: hops}.run(ctx)
}

// waitSessions waits until the solicitation has settled and returns the peer
// sessions other than exclude. A peer the solicitation is still negotiating
// with may hold the block, so a read must not report a miss before then.
//
// When wantPeer is set it also waits until at least one such session exists.
// The solicitation settles while no link is up, and a miss that no peer was
// asked about is not a miss: a read waits for a peer to connect instead.
func (c *Controller) waitSessions(ctx context.Context, exclude *peerSession, wantPeer bool) ([]*peerSession, error) {
	var sessions []*peerSession
	err := c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if !c.settled {
			return false, nil
		}
		sessions = sessions[:0]
		for _, s := range c.sessions {
			if s != exclude {
				sessions = append(sessions, s)
			}
		}
		return !wantPeer || len(sessions) != 0, nil
	})
	return sessions, err
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
	// Ask the settled peer sessions for the block. The resolver does not wait
	// for a peer, so a lookup that ends at idle still completes offline.
	sessions, err := r.c.waitSessions(ctx, nil, false)
	if err != nil {
		return err
	}
	found, err := peerBlockFanout{
		sessions: sessions,
		ref:      r.ref,
		hops:     r.c.cc.GetMaxForwardHops(),
	}.run(ctx)

	// Emit the result, including a miss or a failed exchange.
	switch {
	case err != nil:
		handler.AddValue(dex.NewLookupBlockFromNetworkValue(nil, err))
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

// fanoutResult is one peer's answer to a fanout request.
type fanoutResult struct {
	resp *DexMessage
	err  error
}

// run requests the block from every session and waits until each answers,
// its session closes, or ctx ends. It returns the first verified response and
// cancels the other requests, which tells their peers to stop serving them. It returns nil and no error only when
// every peer answered that it does not have the block, and an error when no
// peer had the block and a request failed, so a dropped link or a timeout is
// never mistaken for a missing block.
func (f peerBlockFanout) run(ctx context.Context) (*DexMessage, error) {
	// Scope the peer requests to this fanout.
	if len(f.sessions) == 0 {
		return nil, nil
	}
	reqCtx, reqCancel := context.WithCancel(ctx)
	defer reqCancel()

	// Fan out the request to every session.
	results := make(chan fanoutResult, len(f.sessions))
	for _, sess := range f.sessions {
		go func() {
			resp, err := sess.requestBlock(reqCtx, f.ref, f.hops)
			if err != nil {
				sess.le.WithError(err).Debug("dex block request failed")
			}
			results <- fanoutResult{resp: resp, err: err}
		}()
	}

	// Return the first successful peer response, remembering the first
	// failure.
	var firstErr error
	for range f.sessions {
		res := <-results
		if res.resp != nil {
			return res.resp, nil
		}
		if firstErr == nil {
			firstErr = res.err
		}
	}

	// Report a failed exchange, or a miss every peer confirmed.
	if firstErr != nil {
		return nil, errors.Wrapf(firstErr, "no peer served block %s", f.ref.MarshalString())
	}
	return nil, nil
}

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
var (
	_ controller.Controller = (*Controller)(nil)
	_ directive.Resolver    = (*lookupResolver)(nil)
)
