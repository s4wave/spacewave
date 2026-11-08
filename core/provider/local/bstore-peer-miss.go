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

// peerReadWait is the default bound of a read from peers. Without a peer
// session the solicit store waits for one indefinitely, so a block every other
// tier lacks would hang its reader; past the bound the read fails with
// block.ErrUnavailable.
const peerReadWait = 3 * time.Second

// peerReadPauseMin and peerReadPauseMax bound the pause after a peer read
// that failed, before asking again.
const (
	peerReadPauseMin = 50 * time.Millisecond
	peerReadPauseMax = 5 * time.Second
)

// peerReadBound is the bound of a read from peers, SetPeerReadWait's value.
var peerReadBound atomic.Int64

// SetPeerReadWait bounds each read from peers, from the next read on; zero or
// less restores peerReadWait. Within the bound a failed read, such as one the
// peer answered with an error because a tier of its own was away, asks again
// after a pause that backs off to peerReadPauseMax, and a read with no peer
// session waits for one and asks as soon as it connects. A peer's answer that
// it lacks the block is final. A backend whose blocks are still served by the
// World's old owner sets a bound long enough to outlast that owner's restarts.
func SetPeerReadWait(d time.Duration) {
	peerReadBound.Store(int64(max(d, 0)))
}

// peerReadLimit returns the bound of a read from peers.
func peerReadLimit() time.Duration {
	if d := time.Duration(peerReadBound.Load()); d > 0 {
		return d
	}
	return peerReadWait
}

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
	blk, err := p.GetStoredBlock(ctx, ref)
	if blk == nil || err != nil {
		return nil, false, err
	}
	return blk.Data, true, nil
}

// GetStoredBlock logs the miss and reads the block and its refs from peers,
// asking again after a failure until peerReadLimit, as SetPeerReadWait
// describes.
func (p *peerMissLog) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Log the first miss of the block.
	p.logMiss(ref)

	// Ask the peers until one answers, or the bound ends.
	readCtx, cancel := context.WithTimeout(ctx, peerReadLimit())
	defer cancel()
	pause := peerReadPauseMin
	for {
		blk, err := p.StoreOps.GetStoredBlock(readCtx, ref)
		if err == nil || readCtx.Err() != nil {
			return blk, peerReadErr(ctx, err)
		}

		// A failed read asks again after a pause.
		select {
		case <-readCtx.Done():
			return nil, peerReadErr(ctx, err)
		case <-time.After(pause):
		}
		pause = min(pause*2, peerReadPauseMax)
	}
}

// peerReadErr reports a peer read that failed or outlived its bound, while
// the caller's ctx is still live, as block.ErrUnavailable.
func peerReadErr(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil || errors.Is(err, block.ErrUnavailable) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return block.ErrUnavailable
	}
	return errors.Join(block.ErrUnavailable, err)
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
