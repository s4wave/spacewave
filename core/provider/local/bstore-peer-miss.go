package provider_local

import (
	"context"
	"errors"
	"sync"
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
