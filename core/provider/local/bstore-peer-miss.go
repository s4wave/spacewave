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
func newPeerMissSource(le *logrus.Entry, peers func() block.StoreOps) func() block.StoreOps {
	seen := &sync.Map{}
	return func() block.StoreOps {
		store := peers()
		if store == nil {
			return nil
		}
		return &peerMissLog{StoreOps: store, le: le, seen: seen}
	}
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

// logMiss warns the first time ref reaches the peers.
func (p *peerMissLog) logMiss(ref *block.BlockRef) {
	key := ref.MarshalString()
	if _, loaded := p.seen.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	p.le.WithField("ref", key).Warn("block missing from the local store, the backend and the fallback; asking peers")
}
