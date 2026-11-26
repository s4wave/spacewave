package provider_local

import (
	"context"
	"errors"
	"fmt"
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
// after the local store, backend and read fallback miss, or while racing a
// paused backend. A chain miss logs once per block. A racing read skips
// that log and asks only when a peer can answer.
type peerMissLog struct {
	// StoreOps is the peer store.
	block.StoreOps
	// le logs the misses.
	le *logrus.Entry
	// seen holds the refs already logged, at most seenRefsCap of them.
	seen *seenRefs
	// peers returns the unscoped peer store, which a read deferred by
	// block.WithPeerReads fetches from after its read scope ends.
	peers func() block.StoreOps
	// counts counts the outcomes of the peer reads, or is nil.
	counts *peerReadCounts
	// unserved holds the refs the peers refused, or is nil to always ask.
	unserved *unservedRefs
}

// newPeerMissSource wraps peers so each block that reaches it is logged once.
// After this bucket has had peers, wait lets a read ride out a restart.
// Each peer read counts its outcome in counts, which may be nil.
func newPeerMissSource(le *logrus.Entry, peers func() block.StoreOps, wait func(context.Context) block.StoreOps, counts *peerReadCounts) func() block.StoreOps {
	seen, unserved := newSeenRefs(), newUnservedRefs()
	var hadPeers atomic.Bool
	return func() block.StoreOps {
		// Resolve the active store or wait for the next lifecycle.
		store := peers()
		switch {
		case store != nil:
			hadPeers.Store(true)
		case hadPeers.Load() && wait != nil:
			store = &returningPeers{wait: wait}
		default:
			return nil
		}

		// Keep the logging, read bound and deferred reads across lifecycles.
		return &peerMissLog{StoreOps: store, le: le, seen: seen, peers: peers, counts: counts, unserved: unserved}
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
	return &peerMissLog{StoreOps: scoped, le: p.le, seen: p.seen, peers: p.peers, counts: p.counts, unserved: p.unserved}, release, nil
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
//
// Under block.WithPeerReads the read asks no peer: it answers a block the
// PeerReads fetched, and otherwise defers the block to it and returns
// block.ErrUnavailable at once, since its caller may hold the World's writer.
func (p *peerMissLog) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Answer from, or defer to, a caller's PeerReads.
	if reads := block.GetPeerReads(ctx); reads != nil {
		if stored := reads.Lookup(ref); stored != nil {
			return stored, nil
		}
		p.logMiss(ref)
		reads.Defer(ref, p.fetch(ref))
		return nil, fmt.Errorf("%w: block %s deferred to a peer fetch", block.ErrUnavailable, ref.MarshalString())
	}

	// Report chain misses and skip racing reads when no peer can answer.
	switch {
	case !racingBackend(ctx):
		p.logMiss(ref)
	case !peersConnected(p.StoreOps):
		return nil, nil
	}

	return readPeers(ctx, p.le, p.StoreOps, ref, p.counts, p.unserved)
}

// fetch returns the deferred peer read of ref, which reads the unscoped peer
// store within the peer read bound.
func (p *peerMissLog) fetch(ref *block.BlockRef) block.PeerFetch {
	return func(ctx context.Context) (*block.StoredBlock, error) {
		store := p.StoreOps
		if p.peers != nil {
			if peers := p.peers(); peers != nil {
				store = peers
			}
		}
		return readPeers(block.WithPeerReads(ctx, nil), p.le, store, ref, p.counts, p.unserved)
	}
}

// peerPresence is a peer store that reports whether a peer is connected.
type peerPresence interface {
	// HasPeerSession reports whether a peer session is open now.
	HasPeerSession() bool
}

// peersConnected reports whether store has a peer to ask now. returningPeers
// has none while P2P sync restarts, and a store that cannot tell may have one.
func peersConnected(store block.StoreOps) bool {
	switch s := store.(type) {
	case *returningPeers:
		return false
	case peerPresence:
		return s.HasPeerSession()
	default:
		return true
	}
}

// readPeers asks the peers in store for ref until one answers, or
// peerReadLimit ends, and counts the outcome once in counts, which may be nil.
// A read that failed while its caller still waited warns through le, once per
// block, as warnPeerFailed describes. A read that failed only because peers
// refused the block, as peerRefused proves, is remembered in unserved, which
// may be nil, and a read of a ref unserved holds fails with
// block.ErrUnavailable at once, asks no peer and counts as remembered instead.
// A denial, a failed exchange, a missing session or a timeout in any attempt
// leaves the ref unremembered, so a blip to a peer, or a grant the requester
// lacked, costs no more than the read it failed.
func readPeers(ctx context.Context, le *logrus.Entry, store block.StoreOps, ref *block.BlockRef, counts *peerReadCounts, unserved *unservedRefs) (blk *block.StoredBlock, err error) {
	// Answer a ref the peers recently refused from memory.
	key := ref.MarshalString()
	if unserved.holds(key, time.Now()) {
		counts.recordRemembered()
		return nil, fmt.Errorf("%w: block %s recently refused by the peers", block.ErrUnavailable, key)
	}

	// Count the read's outcome once it ends, warn of a failure and remember
	// a block the peers refused.
	cause := peerTimeout
	refused, unproven := false, false
	defer func() {
		switch {
		case err == nil:
			counts.record(ref, blk)
		case ctx.Err() != nil:
			counts.recordCanceled()
		default:
			counts.recordFailed(cause)
			warnPeerFailed(le, ref, cause)
			if refused && !unproven {
				unserved.remember(key, time.Now())
			}
		}
	}()

	// Ask the peers until one answers, or the bound ends. A timeout keeps
	// the cause an earlier attempt found, and an attempt the bound cut off
	// neither proves nor disproves a refusal.
	readCtx, cancel := context.WithTimeout(ctx, peerReadLimit())
	defer cancel()
	pause := peerReadPauseMin
	for {
		blk, err := store.GetStoredBlock(readCtx, ref)
		if err != nil {
			if c := peerFailCause(err); c != peerTimeout {
				cause = c
			}
			switch {
			case peerRefused(err):
				refused = true
			case readCtx.Err() == nil:
				unproven = true
			}
		}
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

// peerReadCounts counts a placed store's peer reads by outcome, and the
// blocks they filled.
type peerReadCounts struct {
	// served counts the reads a peer served the block to.
	served atomic.Uint64
	// notHeld counts the reads every peer asked answered it lacks the block.
	notHeld atomic.Uint64
	// failed counts the reads that failed while their caller waited, by
	// peerCause.
	failed [peerCauseCount]atomic.Uint64
	// canceled counts the reads whose caller went away first, which say
	// nothing of the peers.
	canceled atomic.Uint64
	// remembered counts the reads answered unavailable from the memory of
	// a recent peer error, which asked no peer.
	remembered atomic.Uint64
	// filled counts the blocks a peer served that filled the local store.
	filled atomic.Uint64
	// classes counts the served blocks by peerClass.
	classes [peerClassCount]atomic.Uint64
	// queue takes the served blocks to classify, or is nil when the
	// placement classifies none.
	queue chan *block.BlockRef
}

// record counts one peer read of ref that ended without an error, and hands a
// served block to the classifier. The peer store reports a block every peer
// lacks as no block and no error.
func (c *peerReadCounts) record(ref *block.BlockRef, blk *block.StoredBlock) {
	switch {
	case c == nil:
	case blk != nil:
		c.served.Add(1)
		c.classify(ref)
	default:
		c.notHeld.Add(1)
	}
}

// recordFailed counts one peer read that failed while its caller waited.
func (c *peerReadCounts) recordFailed(cause peerCause) {
	if c != nil {
		c.failed[cause].Add(1)
	}
}

// recordCanceled counts one peer read whose caller went away first.
func (c *peerReadCounts) recordCanceled() {
	if c != nil {
		c.canceled.Add(1)
	}
}

// recordRemembered counts one read answered from the memory of a recent peer
// error.
func (c *peerReadCounts) recordRemembered() {
	if c != nil {
		c.remembered.Add(1)
	}
}

// GetBlockExists reports that the peers hold no block, without asking them.
// The peers are a read tier the placed store fills from, not a store of its
// own, and an existence probe comes from a put deciding whether to store a
// block: the local store must keep a block that only a peer holds, and a new
// block is never on a peer. Asking would hold the put, often under the
// World's writer lock, for a peer's answer to a block it cannot have.
func (p *peerMissLog) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch reports each block absent, as GetBlockExists does.
func (p *peerMissLog) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// StatBlock reads the block from peers within the bound, as GetStoredBlock
// does, since the peer store's own stat reads the whole block unbounded.
func (p *peerMissLog) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	blk, err := p.GetStoredBlock(ctx, ref)
	if blk == nil || err != nil {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(blk.Data))}, nil
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
	if !p.seen.add(key) {
		return
	}
	p.le.WithField("ref", key).Warn("block missing from the local store, the backend and the fallback; asking peers")
}
