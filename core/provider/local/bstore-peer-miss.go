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

// peerMissReport is the shortest interval between two reports of the blocks
// that reached the peers. A replica that backfills reads every block from
// peers, so a report per block would log millions of lines.
const peerMissReport = time.Minute

// peerMissLog is the peer source of a placed store's reads. A read reaches it
// only after the local store, the backend and the read fallback all missed, so
// it reports the misses, at most once per peerMissReport with their count and
// the latest ref, before asking peers, and gives the peers peerReadWait to
// serve it.
type peerMissLog struct {
	// StoreOps is the peer store.
	block.StoreOps
	// le logs the misses.
	le *logrus.Entry
	// misses counts the misses between reports.
	misses *peerMisses
}

// peerMisses counts the misses since the last report.
type peerMisses struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// n is the number of misses since the last report.
	n uint64
	// last is when the last report was logged.
	last time.Time
}

// newPeerMissSource wraps peers so the blocks that reach them are reported.
// Once the bucket had peers in this process, a read while it has none, as
// across a Session transport restart or before the next P2P sync lifecycle
// publishes the bucket's store, waits for them through wait within the peer
// read bound, rather than finding no peers and reporting a block missing
// that a peer still holds.
func newPeerMissSource(le *logrus.Entry, peers func() block.StoreOps, wait func(context.Context) block.StoreOps) func() block.StoreOps {
	misses := &peerMisses{}
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
		return &peerMissLog{StoreOps: store, le: le, misses: misses}
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
	return &peerMissLog{StoreOps: scoped, le: p.le, misses: p.misses}, release, nil
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

// logMiss counts ref reaching the peers and reports the count once per
// peerMissReport.
func (p *peerMissLog) logMiss(ref *block.BlockRef) {
	// Count the miss and take the count when a report is due.
	m := p.misses
	m.mtx.Lock()
	m.n++
	now := time.Now()
	if now.Sub(m.last) < peerMissReport {
		m.mtx.Unlock()
		return
	}
	n := m.n
	m.n, m.last = 0, now
	m.mtx.Unlock()

	// Report it with the latest ref.
	p.le.WithField("ref", ref.MarshalString()).WithField("misses", n).
		Warn("blocks missing from the local store, the backend and the fallback; asking peers")
}
