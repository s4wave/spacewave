package psecho

import (
	"context"
	"maps"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/dex/psecho"

// syncProtocolID is the protocol ID for sync streams.
const syncProtocolID = protocol.ID("hydra/dex/psecho/sync")

// sessionKey identifies a keyed routine for stream management.
type sessionKey struct {
	PeerID peer.ID
	Nonce  uint64
}

// remotePeerState tracks what a remote peer wants and what to send.
type remotePeerState struct {
	wantRefs  map[string]*block.BlockRef
	lastTS    int64
	sendQueue []*block.BlockRef
}

// incomingStream holds a stored incoming stream pending processing.
type incomingStream struct {
	ms link.MountedStream
}

// Controller is the pub-sub DEX controller.
type Controller struct {
	le *logrus.Entry
	b  bus.Bus
	cc *Config

	bcast broadcast.Broadcast
	// All below guarded by bcast.
	wantRefs    map[string]*block.BlockRef
	remotePeers map[peer.ID]*remotePeerState
	incoming    map[sessionKey]*incomingStream
	nextNonce   uint64

	// peerID is set during Execute.
	peerID peer.ID

	incomingKeyed *keyed.Keyed[sessionKey, struct{}]
	outgoingKeyed *keyed.Keyed[sessionKey, struct{}]

	// publishNow is set to 1 to bypass debounce for immediate publish.
	publishNow atomic.Int32
}

// NewController constructs a new pub-sub DEX controller.
func NewController(le *logrus.Entry, b bus.Bus, cc *Config) (*Controller, error) {
	c := &Controller{
		le:          le,
		b:           b,
		cc:          cc,
		wantRefs:    make(map[string]*block.BlockRef),
		remotePeers: make(map[peer.ID]*remotePeerState),
		incoming:    make(map[sessionKey]*incomingStream),
	}
	c.incomingKeyed = keyed.NewKeyed(
		c.buildIncomingRoutine,
		keyed.WithExitLogger[sessionKey, struct{}](le),
	)
	c.outgoingKeyed = keyed.NewKeyed(
		c.buildOutgoingRoutine,
		keyed.WithExitLogger[sessionKey, struct{}](le),
		keyed.WithRetry[sessionKey, struct{}](cc.GetSyncBackoff()),
	)
	return c, nil
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.le.Debug("psecho controller running")

	peerID, err := c.cc.ParsePeerID()
	if err != nil {
		return err
	}

	pr, _, prRef, err := peer.GetPeerWithID(ctx, c.b, peerID, false, nil)
	if err != nil {
		return errors.Wrap(err, "get peer")
	}
	privKey, err := pr.GetPrivKey(ctx)
	if err != nil {
		prRef.Release()
		return err
	}
	peerID = pr.GetPeerID()
	c.peerID = peerID
	prRef.Release()

	c.incomingKeyed.SetContext(ctx, true)
	c.outgoingKeyed.SetContext(ctx, true)

	sub, _, subRef, err := pubsub.ExBuildChannelSubscription(
		ctx, c.b, false, c.cc.GetPubsubChannelId(), privKey, nil,
	)
	if err != nil {
		return err
	}
	defer subRef.Release()

	relHandler := sub.AddHandler(func(m pubsub.Message) {
		c.handleIncomingMessage(ctx, m, privKey)
	})
	defer relHandler()

	go c.publishLoop(ctx, sub)

	<-ctx.Done()
	return ctx.Err()
}

// publishLoop runs the debounced wantlist publish loop.
func (c *Controller) publishLoop(ctx context.Context, sub pubsub.Subscription) {
	debounce := time.Duration(c.cc.GetPublishDebounceMsOrDefault()) * time.Millisecond
	timer := time.NewTimer(debounce)
	defer timer.Stop()

	for {
		var ch <-chan struct{}
		var refs map[string]*block.BlockRef
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			refs = make(map[string]*block.BlockRef, len(c.wantRefs))
			maps.Copy(refs, c.wantRefs)
		})

		immediate := c.publishNow.Swap(0) != 0
		if !immediate {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
			select {
			case <-ctx.Done():
				return
			case <-ch:
			case <-timer.C:
			}
			// Re-snapshot after waiting.
			c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				ch = getWaitCh()
				refs = make(map[string]*block.BlockRef, len(c.wantRefs))
				maps.Copy(refs, c.wantRefs)
			})
		}

		if len(refs) == 0 {
			// Empty wantlist: wait for a directive to add something.
			select {
			case <-ctx.Done():
				return
			case <-ch:
			}
			continue
		}

		// Always re-publish non-empty wantlists on each tick.
		// Pub-sub messages can be lost (e.g. subscription not yet
		// established), so periodic re-announce ensures delivery.
		if err := c.publishWantList(sub, refs); err != nil {
			c.le.WithError(err).Warn("publish wantlist failed")
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case dex.LookupBlockFromNetwork:
		return c.resolveLookupBlockFromNetwork(ctx, di, d)
	case link.HandleMountedStream:
		return c.resolveHandleMountedStream(ctx, di, d)
	}
	return nil, nil
}

// resolveHandleMountedStream resolves a HandleMountedStream directive.
func (c *Controller) resolveHandleMountedStream(
	_ context.Context,
	_ directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	if dir.HandleMountedStreamProtocolID() != syncProtocolID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver(
		[]link.MountedStreamHandler{c},
	)), nil
}

// HandleMountedStream handles an incoming mounted stream.
func (c *Controller) HandleMountedStream(
	ctx context.Context,
	ms link.MountedStream,
) error {
	from := ms.GetPeerID()
	var key sessionKey
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.nextNonce++
		key = sessionKey{PeerID: from, Nonce: c.nextNonce}
		c.incoming[key] = &incomingStream{ms: ms}
	})
	c.incomingKeyed.SetKey(key, true)
	return nil
}

// handleIncomingMessage processes a pub-sub message from a remote peer.
func (c *Controller) handleIncomingMessage(
	ctx context.Context,
	m pubsub.Message,
	privKey crypto.PrivKey,
) {
	if !m.GetAuthenticated() || m.GetFrom().MatchesPrivateKey(privKey) {
		return
	}

	var msg PubSubMessage
	if err := msg.UnmarshalVT(m.GetData()); err != nil {
		c.le.WithError(err).Warn("cannot parse pubsub message")
		return
	}

	from := m.GetFrom()
	ts := msg.GetTimestampUnixNano()

	c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		rp, ok := c.remotePeers[from]
		if msg.GetWantEmpty() {
			if ok {
				delete(c.remotePeers, from)
				bcast()
			}
			return
		}
		if !ok {
			rp = &remotePeerState{
				wantRefs: make(map[string]*block.BlockRef),
			}
			c.remotePeers[from] = rp
		}
		if ts <= rp.lastTS {
			return
		}
		rp.lastTS = ts

		// Replace wantlist with snapshot from message.
		rp.wantRefs = make(map[string]*block.BlockRef, len(msg.GetWantRefs()))
		for _, ref := range msg.GetWantRefs() {
			if ref.GetEmpty() {
				continue
			}
			rp.wantRefs[ref.MarshalString()] = ref
		}

		// Process clear refs.
		for _, ref := range msg.GetClearRefs() {
			delete(rp.wantRefs, ref.MarshalString())
		}

		bcast()
	})

	// Check local bucket for blocks the remote peer wants.
	c.checkAndQueueBlocks(ctx, from)
}

// checkAndQueueBlocks checks the local bucket for wanted blocks and queues them.
func (c *Controller) checkAndQueueBlocks(ctx context.Context, pid peer.ID) {
	var wants []*block.BlockRef
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rp, ok := c.remotePeers[pid]
		if !ok {
			return
		}
		for _, ref := range rp.wantRefs {
			wants = append(wants, ref)
		}
	})
	if len(wants) == 0 {
		return
	}

	lk, rel, err := c.getBucketLookup(ctx)
	if err != nil || lk == nil {
		return
	}
	defer rel()

	var queued []*block.BlockRef
	for _, ref := range wants {
		_, ok, lerr := lk.LookupBlock(ctx, ref, WithLocalOnly())
		if lerr != nil || !ok {
			continue
		}
		queued = append(queued, ref)
	}
	if len(queued) == 0 {
		return
	}

	c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		rp, ok := c.remotePeers[pid]
		if !ok {
			return
		}
		rp.sendQueue = append(rp.sendQueue, queued...)
		bcast()
	})

	c.startOutgoingStreams(pid)
}

// startOutgoingStreams starts outgoing stream routines for a peer if needed.
func (c *Controller) startOutgoingStreams(pid peer.ID) {
	maxStreams := c.cc.GetMaxConcurrentStreamsOrDefault()

	// Count existing outgoing streams for this peer.
	var count uint32
	for _, key := range c.outgoingKeyed.GetKeys() {
		if key.PeerID == pid {
			count++
		}
	}

	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rp, ok := c.remotePeers[pid]
		if !ok || len(rp.sendQueue) == 0 {
			return
		}
		for count < maxStreams && len(rp.sendQueue) > 0 {
			c.nextNonce++
			key := sessionKey{PeerID: pid, Nonce: c.nextNonce}
			c.outgoingKeyed.SetKey(key, true)
			count++
		}
	})
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"pub-sub data exchange controller",
	)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*Controller)(nil))

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
