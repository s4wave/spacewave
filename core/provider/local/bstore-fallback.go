package provider_local

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// fallbackFillSlots bounds the fills of one placed store that run at once.
const fallbackFillSlots = 64

// fallbackFillQueueBlocks bounds the fills of one placed store waiting for a
// slot, and fallbackFillBudgetBytes the block data waiting in the queues of
// every placed store of a Provider, so a restart in which many Spaces miss
// their local stores at once queues no more than one would. A restart's
// readers ask the old owner for a burst of blocks the local store lacks; a
// block whose fill is dropped is asked of it again at its next read, and at
// every start after.
const (
	fallbackFillQueueBlocks = 16384
	fallbackFillBudgetBytes = 128 << 20
)

// fillBudget counts the block data queued for fallback fills across the
// placed stores that share it, up to fallbackFillBudgetBytes.
type fillBudget struct {
	// queued is the size of the queued blocks.
	queued atomic.Int64
}

// take charges n bytes to the budget, or reports false when they do not fit.
func (b *fillBudget) take(n int) bool {
	for {
		queued := b.queued.Load()
		if queued+int64(n) > fallbackFillBudgetBytes {
			return false
		}
		if b.queued.CompareAndSwap(queued, queued+int64(n)) {
			return true
		}
	}
}

// give returns n bytes taken from the budget.
func (b *fillBudget) give(n int) {
	b.queued.Add(-int64(n))
}

// readChains builds the read paths of a placed store: reads serve the blocks of
// the local store, then the backend's packs, then cold packs, SQL, and peers
// when those sources are available. Copies use the same lower tiers without
// filling the local store.
//
// Each source fills the local store with its own function. A block read from
// the backend refills the disk without a marker, as the backend holds it. A
// block only the fallback holds is stored with an upload marker, so it uploads
// to the backend, the marker keeps eviction from dropping it before then, and
// later reads find it there instead of the fallback.
//
// With a fill cache, the backend's reads go through it: a block read from the
// packs is copied into the cache instead of the local store, and the next read
// finds it in the cache. The cache holds only copies of blocks the packs hold,
// so nothing but a read of the packs fills it: not the fallback, which keeps
// the marker path above, and not peers, which fill the local store as before.
// The copy path never reads the cache.
//
// Copies read the linked files after every Volume tier and before peers; see
// linkedThen. Reads find them in recorded, which serves them as local hits.
//
// With no fallback, reads and copies are the local store over the backend and
// peers, as they were before fallbacks existed. When peer fills upload, see
// SetPeerFillsUpload, a peer's block is stored like the fallback's, with an
// upload marker, so the reads keep the backend's fill apart from the peers'.
func (p *placementState) readChains(
	recorded, local block.StoreOps,
	remote, peers block_store.StoreSource,
	fallback block.StoreOps,
) (reads, copies block.StoreOps) {
	// Keep a peer's block served without refs once its reader decodes them.
	peers = p.peerDecoded(peers)

	// Read the backend and then peers when the placement has no fallback.
	if fallback == nil && p.cold == nil {
		lower := block_store.NewStoreReadThrough(remote, peers, nil)
		if p.cache != nil || p.peerUpload {
			packs := p.packReads(recorded, remote)
			reads = block_store.NewStoreReadThrough(
				func() block.StoreOps { return packs },
				peers,
				p.peerFill(recorded),
			)
		} else {
			reads = block_store.NewStoreReadThrough(
				func() block.StoreOps { return recorded },
				func() block.StoreOps { return lower },
				p.fill,
			)
		}
		copyLower := block_store.NewStoreReadThrough(remote, p.linkedThen(peers), nil)
		copies = block_store.NewStoreReadThrough(
			func() block.StoreOps { return local },
			func() block.StoreOps { return copyLower },
			nil,
		)
		return reads, copies
	}

	// Keep the legacy backend path when the placement had no SQL or peer
	// upload policy and did not route reads through a fill cache.
	packs := p.packReads(recorded, remote)
	if fallback == nil && p.cache == nil && !p.peerUpload {
		packs = p.packReadsBehind(recorded, remote)
	}

	// Cold hits must bypass both the backend and the SQL migration fill.
	primary := p.withColdPacks(packs)
	if fallback != nil {
		primary = &fallbackReads{
			StoreOps: primary,
			fallback: block.NewReadOnlyStore(fallback),
			fills:    &p.fallbackFills,
			fill:     p.fillFromFallback,
		}
	}
	reads = block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		peers,
		p.peerFill(recorded),
	)

	// Copies use the same lower tiers as serving, then the linked files, with
	// peers last.
	copyPrimary := p.serveReads(local, remote, fallback)
	copies = block_store.NewStoreReadThrough(
		func() block.StoreOps { return copyPrimary },
		p.linkedThen(peers),
		nil,
	)
	return reads, copies
}

// linkedThen returns the linked files over peers, for a copy that read every
// Volume tier first: a block the files' copy tier serves reads as Linked, which a copy within
// this Volume counts as present and a copy to another store writes. Without
// linked files it returns peers.
func (p *placementState) linkedThen(peers block_store.StoreSource) block_store.StoreSource {
	if p.fileSegments == nil {
		return peers
	}
	files := p.fileSegments.GetCopyTier()
	linked := block_store.NewStoreReadThrough(func() block.StoreOps { return files }, peers, nil)
	return func() block.StoreOps { return linked }
}

// serveReads builds the reads that answer a peer's request for a block: the
// local store, the backend, cold packs, then SQL. They never ask peers, so
// two backends that read each other cannot ask in a loop, and they fill
// nothing, so serving a peer adds nothing to the local store.
func (p *placementState) serveReads(local block.StoreOps, remote block_store.StoreSource, fallback block.StoreOps) block.StoreOps {
	// Serve resident and backend blocks before consulting cold packs.
	reads := p.withColdPacks(block_store.NewStoreReadThrough(
		func() block.StoreOps { return local },
		remote,
		nil,
	))

	// Serve legacy SQL blocks last without starting migration fills.
	if fallback == nil {
		return reads
	}
	return &fallbackReads{StoreOps: reads, fallback: block.NewReadOnlyStore(fallback)}
}

// withColdPacks adds the cold read-only tier. A cold fill must never make the
// hot index or uploader claim the block. Even an unmarked local cache fill
// can be queued by writeback.Backfill, so these reads only count repeated GETs.
func (p *placementState) withColdPacks(primary block.StoreOps) block.StoreOps {
	if p.cold == nil {
		return primary
	}
	return &fallbackReads{StoreOps: primary, fallback: p.cold}
}

// packReads builds the reads of the local store over the backend. With a fill
// cache, a backend hit is copied into the cache and not the local store, and the
// cache is read between the two. Without one, a backend hit fills the local
// store.
func (p *placementState) packReads(recorded block.StoreOps, remote block_store.StoreSource) block.StoreOps {
	// Read the front store, when registered, before the cache and the packs.
	if p.front != nil {
		front, lower := p.front, p.cacheReads(recorded, remote)
		return block_store.NewStoreReadThrough(
			func() block.StoreOps { return recorded },
			func() block.StoreOps {
				return block_store.NewStoreReadThrough(func() block.StoreOps { return front }, lower, nil)
			},
			nil,
		)
	}
	return p.packReadsBehind(recorded, remote)
}

// cacheReads returns the source below the local store: the fill cache over
// the backend, or the backend alone.
func (p *placementState) cacheReads(recorded block.StoreOps, remote block_store.StoreSource) block_store.StoreSource {
	if p.cache == nil {
		return remote
	}
	return p.cache.Reads(remote)
}

// packReadsBehind is packReads without a front store.
func (p *placementState) packReadsBehind(recorded block.StoreOps, remote block_store.StoreSource) block.StoreOps {
	if p.cache == nil {
		return block_store.NewStoreReadThrough(
			func() block.StoreOps { return recorded },
			remote,
			p.fill,
		)
	}
	return block_store.NewStoreReadThrough(
		func() block.StoreOps { return recorded },
		p.cache.Reads(remote),
		nil,
	)
}

// peerFill fills the local store with a block read from a peer, and counts
// each fill that succeeds. A peer is not the durable store, so the block never
// enters the fill cache.
func (p *placementState) peerFill(recorded block.StoreOps) block_store.Fill {
	return func(ctx context.Context, _ block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
		// Store the block as a fallback fill when peer fills upload.
		var err error
		if p.peerUpload {
			err = p.fillFromFallback(ctx, ref, stored)
		} else {
			err = p.fill(ctx, recorded, ref, stored)
		}

		// Count a fill that stored the block.
		if err == nil && !p.serveOnly {
			p.peerReads.filled.Add(1)
		}
		return err
	}
}

// fillFromFallback stores a block only the read fallback holds, which the read
// verified against its hash. The write goes through the write-back store,
// which marks it for upload, below the ownership layer as a cache fill: the
// fallback keeps the block, so the copy takes no ownership and a collector
// sweeping it before the upload loses nothing. Tracked or not, the backend does
// not hold the block, so it never takes the refill path of a backend hit.
func (p *placementState) fillFromFallback(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock) error {
	// Serve mode writes nothing.
	if p.serveOnly {
		return nil
	}

	// Store the block with its refs and its upload marker.
	opts := stored.PutOpts(ref)
	opts.CacheFill = true
	got, _, err := p.wb.PutBlock(ctx, stored.Data, opts)
	if err != nil {
		return err
	}
	if !got.GetHash().CompareHash(ref.GetHash()) {
		return errors.New("fallback block stored under another hash")
	}
	return nil
}

// fallbackFills is the state of a placed store's fallback fills, shared by the
// read scopes of its fallback reads.
//
// A fill is Queued, then Filling on a worker, then gone: Filled, Failed, or
// Skipped when the queue was full, the budget spent, or the mount ended
// before its turn. FallbackFillQueue.tla models these states.
type fallbackFills struct {
	// ctx is the mount's context. The fills run under it, and once it ends
	// the queue takes no fill and its workers drop the rest without running
	// them.
	ctx context.Context
	// budget bounds the queued block data across the Provider's stores.
	budget *fillBudget

	// running counts the fills running.
	running atomic.Int64
	// packErrors counts backend errors that fell through to the fallback.
	packErrors atomic.Uint64
	// hits counts blocks only the fallback held.
	hits atomic.Uint64
	// refsUnknown counts hits whose refs the fallback does not hold.
	refsUnknown atomic.Uint64
	// filled counts fills that landed.
	filled atomic.Uint64
	// failed counts fills that failed.
	failed atomic.Uint64
	// skipped counts fills dropped because the queue or the budget was full,
	// or the mount ended.
	skipped atomic.Uint64

	// mtx guards the queue.
	mtx sync.Mutex
	// queue holds the fills waiting for a slot, oldest first.
	queue []fallbackFill
	// queued holds the refs queued or filling, by marshaled ref, so a block
	// read again before its fill lands is not queued twice.
	queued map[string]struct{}
}

// fallbackFill is one block waiting to fill the local store.
type fallbackFill struct {
	key    string
	ref    *block.BlockRef
	stored *block.StoredBlock
	fill   func(context.Context, *block.BlockRef, *block.StoredBlock) error
}

// status returns the counters.
func (f *fallbackFills) status() FallbackStatus {
	return FallbackStatus{
		PackErrors:  f.packErrors.Load(),
		Hits:        f.hits.Load(),
		RefsUnknown: f.refsUnknown.Load(),
		Filled:      f.filled.Load(),
		FillFailed:  f.failed.Load(),
		FillSkipped: f.skipped.Load(),
	}
}

// start queues fill to run in the background, and starts a worker when a
// slot is free; the workers run the queue in order, so a burst of fills waits
// for a slot rather than being dropped. A block already queued or filling is
// not queued again. When the queue or the budget is full, or the mount has
// ended, it counts a skip and leaves the block for the next read. The fill runs
// under the mount's context, not the reader's, which ends with the read.
func (f *fallbackFills) start(ref *block.BlockRef, stored *block.StoredBlock, fill func(context.Context, *block.BlockRef, *block.StoredBlock) error) {
	// Key the fill by its block and size it by its data.
	key := ref.MarshalString()
	size := len(stored.Data)

	// Queue the fill, unless it is queued, nothing has room, or the mount
	// ended.
	f.mtx.Lock()
	if _, ok := f.queued[key]; ok {
		f.mtx.Unlock()
		return
	}
	if f.ctx.Err() != nil || len(f.queue) >= fallbackFillQueueBlocks || !f.budget.take(size) {
		f.mtx.Unlock()
		f.skipped.Add(1)
		return
	}
	if f.queued == nil {
		f.queued = make(map[string]struct{})
	}
	f.queued[key] = struct{}{}
	f.queue = append(f.queue, fallbackFill{
		key:    key,
		ref:    ref,
		stored: stored,
		fill:   fill,
	})

	// Start a worker when a slot is free. A full set of workers drains the
	// queue before any exits, so a queued fill always has one.
	spawn := f.running.Load() < fallbackFillSlots
	if spawn {
		f.running.Add(1)
	}
	f.mtx.Unlock()
	if spawn {
		go f.drain()
	}
}

// drain runs queued fills until the queue is empty, then frees its slot. The
// fills run in the background so a read does not wait on the local writer.
// Once the mount ends it drops each fill it takes without running it.
func (f *fallbackFills) drain() {
	for {
		// Take the oldest fill, or free the slot.
		f.mtx.Lock()
		if len(f.queue) == 0 {
			f.queue = nil
			f.running.Add(-1)
			f.mtx.Unlock()
			return
		}
		job := f.queue[0]
		f.queue[0] = fallbackFill{}
		f.queue = f.queue[1:]
		f.budget.give(len(job.stored.Data))
		f.mtx.Unlock()

		// Fill it, unless the mount ended; a dropped or failed fill leaves
		// the block to its next read.
		ended := f.ctx.Err() != nil
		var err error
		if !ended {
			err = job.fill(f.ctx, job.ref, job.stored)
		}
		f.mtx.Lock()
		delete(f.queued, job.key)
		f.mtx.Unlock()
		switch {
		case ended:
			f.skipped.Add(1)
		case err != nil:
			f.failed.Add(1)
		default:
			f.filled.Add(1)
		}
	}
}

// fallbackReads reads a placed store's own sources, then its read fallback.
// The fallback answers when the sources miss or fail, so an unreachable bucket
// does not hide a block the fallback holds. A failure is never an answer of
// absence: when the fallback misses too, the read returns the error.
type fallbackReads struct {
	// StoreOps is the placed store's own sources, which reject writes.
	block.StoreOps
	// fallback is the read-only fallback.
	fallback block.StoreOps
	// fills runs and counts the fills of the fallback's hits, or holds nil
	// for a read that fills nothing.
	fills *fallbackFills
	// fill stores a fallback hit, or holds nil for a read that fills nothing.
	fill func(context.Context, *block.BlockRef, *block.StoredBlock) error
}

// BeginReadOperation opens a read scope on the sources and the fallback.
func (r *fallbackReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Scope the sources, then the fallback, releasing the first on failure.
	primary, releasePrimary, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	fallback, releaseFallback, err := r.fallback.BeginReadOperation(ctx)
	if err != nil {
		releasePrimary()
		return nil, nil, err
	}
	return &fallbackReads{StoreOps: primary, fallback: fallback, fills: r.fills, fill: r.fill}, func() {
		releaseFallback()
		releasePrimary()
	}, nil
}

// GetBlock reads the sources, then the fallback.
func (r *fallbackReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Serve a block the sources hold.
	data, found, err := r.StoreOps.GetBlock(ctx, ref)
	if err == nil && found {
		return data, true, nil
	}

	// Ask the fallback about a miss or a failure.
	stored, err := r.readFallback(ctx, ref, err)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock reads the block and its refs from the sources, then the
// fallback.
func (r *fallbackReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Serve a block the sources hold.
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if err == nil && stored != nil {
		return stored, nil
	}

	// Ask the fallback about a miss or a failure.
	return r.readFallback(ctx, ref, err)
}

// readFallback reads the block from the fallback after the sources missed it,
// or failed with primaryErr. A fallback hit is filled into the local store,
// except past a corrupt local copy: that copy stays as it is, and replacing it
// is a separate repair. Without a hit, it returns primaryErr, else the
// fallback's error, else a miss.
func (r *fallbackReads) readFallback(ctx context.Context, ref *block.BlockRef, primaryErr error) (*block.StoredBlock, error) {
	// A failure of the caller's own context is not the sources' failure.
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if primaryErr != nil && r.fills != nil {
		r.fills.packErrors.Add(1)
	}

	// Read the fallback and fill its hit. The fallback checks its own bytes;
	// corrupt bytes are a failure, not a miss, and fill nothing.
	stored, err := r.fallback.GetStoredBlock(ctx, ref)
	if errors.Is(err, block.ErrBlockCorrupt) {
		if r.fills != nil {
			r.fills.failed.Add(1)
		}
		return nil, errors.Wrap(err, "read fallback block")
	}
	if err == nil && stored != nil {
		if !errors.Is(primaryErr, block.ErrBlockCorrupt) {
			r.fillHit(ctx, ref, stored)
		}
		return stored, nil
	}
	if primaryErr != nil {
		return nil, primaryErr
	}
	return nil, err
}

// fillHit counts a fallback hit and stores it locally with its upload marker.
// A hit without its refs serves but is not stored, since the pack would keep it
// as a leaf.
func (r *fallbackReads) fillHit(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock) {
	if r.fills == nil || r.fill == nil {
		return
	}
	r.fills.hits.Add(1)
	if !stored.RefsKnown {
		r.fills.refsUnknown.Add(1)
		return
	}
	r.fills.start(ref, stored, r.fill)
}

// GetBlockExists checks the sources, then the fallback.
func (r *fallbackReads) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Report a block the sources hold.
	found, err := r.StoreOps.GetBlockExists(ctx, ref)
	if err == nil && found {
		return true, nil
	}

	// Ask the fallback about a miss or a failure.
	if ctx.Err() != nil {
		return false, context.Cause(ctx)
	}
	fallbackFound, fallbackErr := r.fallback.GetBlockExists(ctx, ref)
	if fallbackErr == nil && fallbackFound {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, fallbackErr
}

// GetBlockExistsBatch checks the sources for every ref, then asks the fallback
// about the refs they did not confirm.
func (r *fallbackReads) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check the sources, treating a failure as no answer.
	out := make([]bool, len(refs))
	found, err := r.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err == nil {
		copy(out, found)
	}

	// Collect the refs the sources did not confirm.
	var missIdx []int
	var missRefs []*block.BlockRef
	for i, ok := range out {
		if !ok {
			missIdx = append(missIdx, i)
			missRefs = append(missRefs, refs[i])
		}
	}
	if len(missRefs) == 0 {
		return out, nil
	}

	// Ask the fallback about them. A miss counts only when nothing failed.
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	fallbackFound, fallbackErr := r.fallback.GetBlockExistsBatch(ctx, missRefs)
	if fallbackErr == nil {
		for j, i := range missIdx {
			out[i] = fallbackFound[j]
		}
	}
	if err != nil {
		return nil, err
	}
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return out, nil
}

// StatBlock uses each tier's metadata path, without writing a cache fill.
func (r *fallbackReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Prefer the local and hot sources, as for a body read.
	stat, err := r.StoreOps.StatBlock(ctx, ref)
	if err == nil && stat != nil {
		return stat, nil
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}

	// A fallback hit can answer past a primary failure, but a miss cannot.
	fallbackStat, fallbackErr := r.fallback.StatBlock(ctx, ref)
	if fallbackErr == nil && fallbackStat != nil {
		return fallbackStat, nil
	}
	if errors.Is(fallbackErr, block.ErrBlockCorrupt) {
		return nil, errors.Wrap(fallbackErr, "stat fallback block")
	}
	if err != nil {
		return nil, err
	}
	return nil, fallbackErr
}

// _ is a type assertion
var _ block.StoreOps = (*fallbackReads)(nil)
