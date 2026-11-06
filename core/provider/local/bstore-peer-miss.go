package provider_local

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// peerReadWait bounds a read from peers. Without a peer session the solicit
// store waits for one indefinitely, so a block every other tier lacks would
// hang its reader; past this wait the read fails with block.ErrUnavailable.
const peerReadWait = 3 * time.Second

// peerMissLog is the peer source of a placed store's reads. A read reaches it
// only after the local store, the backend and the read fallback all missed, so
// it warns once per block with the block's ref before asking peers, and gives
// the peers peerReadWait to serve it.
type peerMissLog struct {
	// StoreOps is the peer store.
	block.StoreOps
	// le logs the misses.
	le *logrus.Entry
	// seen holds the refs already logged, keyed by their marshaled string.
	seen *sync.Map
}

// newPeerMissSource wraps peers so each block that reaches it is logged once.
// Once the bucket had peers in this process, a read while it has none, as
// across a Session transport restart or before the next P2P sync lifecycle
// publishes the bucket's store, waits for them through wait within the peer
// read bound, rather than finding no peers and reporting a block missing
// that a peer still holds.
func newPeerMissSource(le *logrus.Entry, peers func() block.StoreOps, wait func(context.Context) block.StoreOps) func() block.StoreOps {
	seen := &sync.Map{}
	var hadPeers atomic.Bool
	return func() block.StoreOps {
		store := peers()
		switch {
		case store != nil:
			hadPeers.Store(true)
		case hadPeers.Load():
			store = &returningPeers{wait: wait}
		default:
			return nil
		}
		return &peerMissLog{StoreOps: store, le: le, seen: seen}
	}
}

// returningPeers is the peer store while P2P sync restarts: each read waits
// for the next lifecycle's store, then reads it. Other calls answer as an
// empty store.
type returningPeers struct {
	block.NopStoreOps
	// wait returns the next lifecycle's store, or nil when its context ends.
	wait func(context.Context) block.StoreOps
}

// BeginReadOperation returns the store itself.
func (p *returningPeers) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return p, func() {}, nil
}

// GetBlock waits for the peers, then reads the block from them.
func (p *returningPeers) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	store := p.wait(ctx)
	if store == nil {
		return nil, false, context.Cause(ctx)
	}
	return store.GetBlock(ctx, ref)
}

// GetStoredBlock waits for the peers, then reads the block and its refs.
func (p *returningPeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	store := p.wait(ctx)
	if store == nil {
		return nil, context.Cause(ctx)
	}
	return store.GetStoredBlock(ctx, ref)
}

// BeginReadOperation opens a read scope on the peers that keeps the miss log
// and the read bound. The embedded store's own method would return the bare
// peer scope, whose reads wait for a peer session indefinitely.
func (p *peerMissLog) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := p.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &peerMissLog{StoreOps: scoped, le: p.le, seen: p.seen}, release, nil
}

// GetBlock logs the miss and reads the block from peers.
func (p *peerMissLog) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Log the first miss of the block.
	p.logMiss(ref)

	// Ask the peers for at most peerReadWait.
	readCtx, cancel := context.WithTimeout(ctx, peerReadWait)
	defer cancel()
	data, ok, err := p.StoreOps.GetBlock(readCtx, ref)
	return data, ok, peerReadErr(ctx, err)
}

// GetStoredBlock logs the miss and reads the block and its refs from peers.
func (p *peerMissLog) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Log the first miss of the block.
	p.logMiss(ref)

	// Ask the peers for at most peerReadWait.
	readCtx, cancel := context.WithTimeout(ctx, peerReadWait)
	defer cancel()
	blk, err := p.StoreOps.GetStoredBlock(readCtx, ref)
	return blk, peerReadErr(ctx, err)
}

// peerReadErr reports a peer read that outlived peerReadWait, while the
// caller's ctx is still live, as block.ErrUnavailable.
func peerReadErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return block.ErrUnavailable
	}
	return err
}

// logMiss warns the first time ref reaches the peers.
func (p *peerMissLog) logMiss(ref *block.BlockRef) {
	key := ref.MarshalString()
	if _, loaded := p.seen.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	p.le.WithField("ref", key).Warn("block missing from the local store, the backend and the fallback; asking peers")
}
