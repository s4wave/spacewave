package provider_local

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/sirupsen/logrus"
)

// peerReadWait is the default bound of a read from peers. Without a peer
// session the solicit store waits for one indefinitely, so a block every other
// tier lacks would hang its reader; past the bound the read fails with
// block.ErrUnavailable. A read from a store that counts the bytes its peers
// deliver ends only once the bound passes with none arriving, as stallBound
// describes: on a slow link a peer's answers queue behind each other in its
// session, so one answer can take longer than the bound while the link stays
// busy.
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
// serve it. It serves reads only and never asks peers about existence.
type peerMissLog struct {
	// StoreOps is the peer store.
	block.StoreOps
	// le logs the misses.
	le *logrus.Entry
	// misses counts the misses between reports.
	misses *peerMisses
	// counts counts the outcomes of the peer reads, or is nil.
	counts *peerReadCounts
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
// that a peer still holds. Each peer read counts its outcome in counts, which
// may be nil.
func newPeerMissSource(le *logrus.Entry, peers func() block.StoreOps, wait func(context.Context) block.StoreOps, counts *peerReadCounts) func() block.StoreOps {
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
		return &peerMissLog{StoreOps: store, le: le, misses: misses, counts: counts}
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

// GetBlock waits for the peers, then reads the block from them. A wait that
// ends with no peers fails with dex_solicit.ErrNoPeerSession.
func (p *returningPeers) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	store := p.wait(ctx)
	if store == nil {
		return nil, false, noPeersErr(ctx)
	}
	return store.GetBlock(ctx, ref)
}

// GetStoredBlock waits for the peers, then reads the block and its refs. A
// wait that ends with no peers fails with dex_solicit.ErrNoPeerSession.
func (p *returningPeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	store := p.wait(ctx)
	if store == nil {
		return nil, noPeersErr(ctx)
	}
	return store.GetStoredBlock(ctx, ref)
}

// noPeersErr is the failure of a wait for peers that ended with ctx, which
// still matches ctx's own error.
func noPeersErr(ctx context.Context) error {
	return fmt.Errorf("%w: %w", dex_solicit.ErrNoPeerSession, context.Cause(ctx))
}

// BeginReadOperation opens a read scope on the peers that keeps the miss log
// and the read bound. The embedded store's own method would return the bare
// peer scope, whose reads wait for a peer session indefinitely.
func (p *peerMissLog) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := p.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &peerMissLog{StoreOps: scoped, le: p.le, misses: p.misses, counts: p.counts}, release, nil
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
	p.logMiss(ref)
	return readPeers(ctx, p.le, p.StoreOps, ref, p.counts)
}

// readPeers asks the peers in store for ref until one answers, or
// peerReadLimit ends, and counts the outcome once in counts, which may be nil.
// A read that failed while its caller still waited warns through le, once per
// block, as warnPeerFailed describes.
func readPeers(ctx context.Context, le *logrus.Entry, store block.StoreOps, ref *block.BlockRef, counts *peerReadCounts) (blk *block.StoredBlock, err error) {
	// Count the read's outcome once it ends, and warn of a failure.
	cause := peerTimeout
	defer func() {
		switch {
		case err == nil:
			counts.record(ref, blk)
		case ctx.Err() != nil:
			counts.recordCanceled()
		default:
			counts.recordFailed(cause)
			warnPeerFailed(le, ref, cause)
		}
	}()

	// Ask the peers until one answers, or the bound ends. A timeout keeps
	// the cause an earlier attempt found.
	readCtx, cancel := withStallBound(ctx, peerReadLimit(), store)
	defer cancel()
	pause := peerReadPauseMin
	for {
		blk, err := store.GetStoredBlock(readCtx, ref)
		if err != nil {
			if c := peerFailCause(err); c != peerTimeout {
				cause = c
			}
		}
		// A corrupt answer is deterministic: asking again returns the same bytes.
		if err == nil || readCtx.Err() != nil || errors.Is(err, block.ErrBlockCorrupt) {
			return blk, peerReadErr(ctx, readCtx, ref, err)
		}

		// A failed read asks again after a pause.
		select {
		case <-readCtx.Done():
			return nil, peerReadErr(ctx, readCtx, ref, err)
		//gostyle:timer backoff
		case <-time.After(pause):
		}
		pause = min(pause*2, peerReadPauseMax)
	}
}

// progressStore is a peer store that counts the bytes its peers delivered.
type progressStore interface {
	// ReceivedBytes returns the bytes read from the peers so far.
	ReceivedBytes() uint64
}

// stallBound ends a read from a progressStore once limit passes with no byte
// arriving from its peers. Each check compares the received count with the
// count the check before saw, and waits limit again while it grew. A read
// waits on the bytes of every peer of the store, so it outlasts the bound
// while any peer delivers.
type stallBound struct {
	// ctx is the bounded context, and cancel ends it.
	ctx    context.Context
	cancel context.CancelFunc
	// limit is the longest wait with nothing arriving.
	limit time.Duration
	// received returns the store's received count.
	received func() uint64
	// last is the count the previous check saw. Only the checks touch it
	// after construction, each started by the one before.
	last uint64
	// timer is the pending check.
	timer atomic.Pointer[time.Timer]
}

// withStallBound returns ctx bounded as stallBound describes, and the cancel
// that ends it. A store that counts no bytes bounds the whole read at limit.
func withStallBound(ctx context.Context, limit time.Duration, store block.StoreOps) (context.Context, context.CancelFunc) {
	// A store that counts no bytes bounds the whole read.
	ps, ok := store.(progressStore)
	if !ok {
		return context.WithTimeout(ctx, limit)
	}

	// Check for arrivals each time limit passes.
	b := &stallBound{limit: limit, received: ps.ReceivedBytes, last: ps.ReceivedBytes()}
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.timer.Store(time.AfterFunc(limit, b.check))
	return b.ctx, func() {
		b.cancel()
		b.timer.Load().Stop()
	}
}

// check ends the read when nothing arrived since the last check, and
// otherwise waits limit again.
func (b *stallBound) check() {
	if b.ctx.Err() != nil {
		return
	}
	if n := b.received(); n != b.last {
		b.last = n
		b.timer.Store(time.AfterFunc(b.limit, b.check))
		return
	}
	b.cancel()
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

// GetBlockExists reports every block missing. Peers are a read source only:
// existence is a question about the stores this placement holds, and a write
// or a copy that asks it must never wait on, or fail with, a peer exchange.
// This is the answer returningPeers gives while the peers are away.
func (p *peerMissLog) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch reports every block missing, as GetBlockExists does.
func (p *peerMissLog) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// peerReadErr reports a peer read of ref that failed or outlived its bound
// readCtx, while the caller's ctx is still live, as block.ErrUnavailable
// naming ref, so the reader learns which block no peer served. A read that
// outlived its bound keeps its last failure as text only: the failure is often
// the bound's own deadline or cancellation, which must not read as the
// caller's. An error that already is block.ErrUnavailable comes from the peer
// store, which names the block.
func peerReadErr(ctx, readCtx context.Context, ref *block.BlockRef, err error) error {
	switch {
	case err == nil, ctx.Err() != nil:
		return err
	case readCtx.Err() != nil:
		return fmt.Errorf("%w: no peer served block %s: nothing arrived for %v: %v", block.ErrUnavailable, ref.MarshalString(), peerReadLimit(), err)
	case errors.Is(err, block.ErrUnavailable), errors.Is(err, block.ErrBlockCorrupt):
		return err
	default:
		return fmt.Errorf("%w: peer read of block %s: %w", block.ErrUnavailable, ref.MarshalString(), err)
	}
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
