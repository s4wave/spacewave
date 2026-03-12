package dex_solicit

import (
	"context"
	"sync"
	"time"

	link_solicit "github.com/aperturerobotics/bifrost/link/solicit"
	"github.com/aperturerobotics/bifrost/protocol"
	stream_packet "github.com/aperturerobotics/bifrost/stream/packet"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/dex/solicit"

// DexProtocolID is the protocol ID used for solicitation.
const DexProtocolID = protocol.ID("hydra/dex")

// maxMessageSize is the max message size for packet sessions.
// 10MB to accommodate large blocks.
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
	c.le.Debug("dex solicit controller running")

	peerID, err := c.cc.ParsePeerID()
	if err != nil {
		return err
	}

	// Emit SolicitProtocol directive with bucket ID as context.
	solicitCtx := []byte(c.cc.GetBucketId())
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

	<-ctx.Done()
	return ctx.Err()
}

// handleSolicitedStream processes a new solicited stream from a DEX peer.
func (c *Controller) handleSolicitedStream(ctx context.Context, sms link_solicit.SolicitMountedStream) {
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil || taken {
		return
	}

	remotePeer := ms.GetPeerID().String()
	le := c.le.WithField("remote-peer", remotePeer)

	sess := &peerSession{
		c:       c,
		le:      le,
		ms:      ms,
		sess:    stream_packet.NewSession(ms.GetStream(), maxMessageSize),
		pending: make(map[uint32]chan *DexMessage),
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace existing session if any.
		if old, ok := c.sessions[remotePeer]; ok {
			old.close()
		}
		c.sessions[remotePeer] = sess
		broadcast()
	})

	le.Debug("dex peer session started")

	go func() {
		sess.run(ctx)
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if cur, ok := c.sessions[remotePeer]; ok && cur == sess {
				delete(c.sessions, remotePeer)
				broadcast()
			}
		})
		le.Debug("dex peer session ended")
	}()
}

// forwardToPeers forwards a block request to other connected peers,
// excluding the session that originated the request. Returns (data, true)
// on first successful response.
func (c *Controller) forwardToPeers(ctx context.Context, ref *block.BlockRef, hops uint32, exclude *peerSession) ([]byte, bool) {
	var sessions []*peerSession
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, s := range c.sessions {
			if s != exclude {
				sessions = append(sessions, s)
			}
		}
	})
	if len(sessions) == 0 {
		return nil, false
	}

	type result struct {
		data  []byte
		found bool
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, requestTimeout)
	defer reqCancel()

	results := make(chan result, len(sessions))
	var wg sync.WaitGroup
	wg.Add(len(sessions))
	for _, sess := range sessions {
		go func() {
			defer wg.Done()
			data, found, err := sess.requestBlock(reqCtx, ref, hops)
			if err != nil || !found {
				results <- result{}
				return
			}
			results <- result{data: data, found: true}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()
	for res := range results {
		if res.found {
			reqCancel()
			return res.data, true
		}
	}
	return nil, false
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
	for {
		// Snapshot current sessions and get wait channel atomically.
		var sessions []*peerSession
		var ch <-chan struct{}
		r.c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			for _, s := range r.c.sessions {
				sessions = append(sessions, s)
			}
		})

		// Query all known peers in parallel with a per-request timeout.
		if data, found := r.queryPeers(ctx, sessions); found {
			if storeErr := r.storeBlock(ctx, data, r.ref); storeErr != nil {
				r.c.le.WithError(storeErr).Warn("failed to store received block")
			}
			handler.AddValue(dex.NewLookupBlockFromNetworkValue(data, nil))
			return nil
		}

		// Wait for session changes (new peer connections / disconnections).
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// queryPeers queries all sessions in parallel for the block.
// Returns (data, true) on first successful response.
func (r *lookupResolver) queryPeers(ctx context.Context, sessions []*peerSession) ([]byte, bool) {
	if len(sessions) == 0 {
		return nil, false
	}

	type result struct {
		data  []byte
		found bool
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, requestTimeout)
	defer reqCancel()

	results := make(chan result, len(sessions))
	var wg sync.WaitGroup
	wg.Add(len(sessions))
	for _, sess := range sessions {
		go func() {
			defer wg.Done()
			data, found, err := sess.requestBlock(reqCtx, r.ref, r.c.cc.GetMaxForwardHops())
			if err != nil || !found {
				results <- result{}
				return
			}
			results <- result{data: data, found: true}
		}()
	}

	// Collect results; return early on first found.
	go func() {
		wg.Wait()
		close(results)
	}()
	for res := range results {
		if res.found {
			reqCancel()
			return res.data, true
		}
	}
	return nil, false
}

// storeBlock stores a received block in the local bucket.
func (r *lookupResolver) storeBlock(ctx context.Context, data []byte, ref *block.BlockRef) error {
	lkv, _, lkRel, err := bucket_lookup.ExBuildBucketLookup(ctx, r.c.b, false, r.c.cc.GetBucketId(), nil)
	if err != nil {
		return err
	}
	defer lkRel.Release()

	lk, err := lkv.GetLookup(ctx)
	if err != nil {
		return err
	}
	if lk == nil {
		return nil
	}

	_, _, err = lk.PutBlock(ctx, data, &block.PutOpts{
		HashType:      ref.GetHash().GetHashType(),
		ForceBlockRef: ref.Clone(),
	})
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupResolver)(nil))

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
var _ controller.Controller = ((*Controller)(nil))
