package provider_local

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// fallbackFillSlots bounds the fills of one placed store that run at once.
const fallbackFillSlots = 64

// fallbackFillQueueBlocks bounds the fills of one placed store waiting for a
// slot, and fallbackFillBudgetBytes the block data waiting in the queues of
// every placed store of a Provider, so a restart in which many Spaces miss
// their local stores at once queues no more than one would. A restart's
// readers ask the old owner for a burst of blocks the local store lacks. A
// block whose fill is dropped is asked of the old owner again at its next
// read, and at every start after.
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

// readChains builds the read paths of a placed store: reads serve the blocks
// of the local store, then the backend's packs, then cold packs, SQL, and
// peers when those sources are available. Copies use the same lower tiers
// without filling the local store.
//
// Each source fills the local store with its own function. A block read from
// the backend refills the disk without a marker, as the backend holds it. A
// block only the fallback holds is stored with an upload marker, so it uploads
// to the backend, the marker keeps eviction from dropping it before then, and
// later reads find it there instead of the fallback.
//
// With a fill cache, reads ask it first, before the local store, and the
// backend's reads go through it: a block read from the packs is copied into
// the cache instead of the local store, and the next read finds it in the
// cache without reading the Volume. The cache holds only copies of blocks the
// packs hold, so nothing but a read of the packs fills it: not the fallback,
// which keeps the marker path above, and not peers, which fill the local store
// as before. The copy path never reads the cache.
//
// Copies read the linked files after every Volume tier and before peers. See
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

	// While the backend is paused a read races the sources below it, the
	// fallback and then the peers, as pausedReads describes, each with the
	// fill the chain gives it.
	pause := watchBackendPause(remote)
	localSrc := func() block.StoreOps { return local }

	// Read the backend and then peers when the placement has no fallback.
	if fallback == nil && p.cold == nil {
		lower := block_store.NewStoreReadThrough(remote, peers, nil)
		fill := p.fill
		if p.cache != nil || p.peerUpload {
			packs := p.packReads(recorded, remote)
			fill = p.peerFill(recorded)
			reads = block_store.NewStoreReadThrough(func() block.StoreOps { return packs }, peers, fill)
		} else {
			reads = block_store.NewStoreReadThrough(
				func() block.StoreOps { return recorded },
				func() block.StoreOps { return lower },
				fill,
			)
		}
		reads = newPausedReads(reads, pause, readSource(peerReadsWithFill(localSrc, peers, fill)))
		copyPeers := p.linkedThen(peers)
		copyLower := block_store.NewStoreReadThrough(remote, copyPeers, nil)
		copies = block_store.NewStoreReadThrough(localSrc, func() block.StoreOps { return copyLower }, nil)
		copies = newPausedReads(copies, pause, readSource(copyPeers))
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
	var fallbackHit readAlternate
	if fallback != nil {
		withFallback := &fallbackReads{
			StoreOps: primary,
			fallback: block.NewReadOnlyStore(fallback),
			pause:    pause,
			fills:    &p.fallbackFills,
			fill:     p.fillFromFallback,
		}
		primary, fallbackHit = withFallback, withFallback.fallbackHit
	}
	fill := p.peerFill(recorded)
	reads = newPausedReads(
		block_store.NewStoreReadThrough(func() block.StoreOps { return primary }, peers, fill),
		pause,
		fallbackHit,
		readSource(peerReadsWithFill(localSrc, peers, fill)),
	)

	// Copies use the same lower tiers as serving, then the linked files, with
	// peers last.
	copyServe, copyHit := p.serveSources(local, remote, fallback, pause)
	copyPeers := p.linkedThen(peers)
	copies = newPausedReads(
		block_store.NewStoreReadThrough(func() block.StoreOps { return copyServe }, copyPeers, nil),
		pause,
		copyHit,
		readSource(copyPeers),
	)
	return reads, copies
}

// peerReadsWithFill is the peer source of a read that races a paused backend:
// it reads the local store, which missed, then peers, and fills a peer's
// block with fill as the chain's own peer read does. local is the store
// without its read recorder, since the chain it races already recorded any
// hit.
func peerReadsWithFill(local, peers block_store.StoreSource, fill block_store.Fill) block_store.StoreSource {
	if peers == nil {
		return nil
	}
	reads := block_store.NewStoreReadThrough(local, peers, fill)
	return func() block.StoreOps { return reads }
}

// linkedThen returns the linked files over peers, for a copy that read every
// Volume tier first: a block the files' copy tier serves reads as Linked. A
// copy within this Volume counts that block as present and a copy to another
// store writes that block. Without linked files linkedThen returns peers.
func (p *placementState) linkedThen(peers block_store.StoreSource) block_store.StoreSource {
	if p.fileSegments == nil {
		return peers
	}
	files := p.fileSegments.GetCopyTier()
	linked := block_store.NewStoreReadThrough(func() block.StoreOps { return files }, peers, nil)
	return func() block.StoreOps { return linked }
}

// withLinkedFiles returns local over the linked files, which fill nothing: a
// linked file's block is served from the file, its only copy. A peer's serve
// reads use withLinkedFiles, so a member reads the owner's linked files. A
// copy reads the files only after every Volume tier, see linkedThen. The files
// are read through their copy tier, so a block they serve reads as Linked and
// the DEX can answer a peer that asked for it linked, with no data.
func (p *placementState) withLinkedFiles(local block.StoreOps) block.StoreOps {
	if p.fileSegments == nil {
		return local
	}
	return &fallbackReads{StoreOps: local, fallback: p.fileSegments.GetCopyTier()}
}

// serveReads builds the reads that answer a peer's request for a block: the
// local store, the backend, cold packs, then SQL. They never ask peers, so
// two backends that read each other cannot ask in a loop, and they fill
// nothing, so serving a peer adds nothing to the local store. While the
// backend is paused the fallback races it.
func (p *placementState) serveReads(local block.StoreOps, remote block_store.StoreSource, fallback block.StoreOps) block.StoreOps {
	pause := watchBackendPause(remote)
	serve, hit := p.serveSources(local, remote, fallback, pause)
	return newPausedReads(serve, pause, hit)
}

// serveSources builds the sources serveReads reads, and the fallback's read
// that races them while pause reports the backend paused, or nil without a
// fallback.
func (p *placementState) serveSources(
	local block.StoreOps,
	remote block_store.StoreSource,
	fallback block.StoreOps,
	pause backendPause,
) (block.StoreOps, readAlternate) {
	// Serve resident and backend blocks before consulting cold packs.
	reads := p.withColdPacks(block_store.NewStoreReadThrough(
		func() block.StoreOps { return local },
		remote,
		nil,
	))

	// Serve legacy SQL blocks last without starting migration fills.
	if fallback == nil {
		return reads, nil
	}
	serve := &fallbackReads{StoreOps: reads, fallback: block.NewReadOnlyStore(fallback), pause: pause}
	return serve, serve.fallbackHit
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
// cache, a backend hit is copied into the cache and not the local store, and
// the cache is read first. See cacheFirst. Without one, a backend hit fills the
// local store.
func (p *placementState) packReads(recorded block.StoreOps, remote block_store.StoreSource) block.StoreOps {
	// Read the front store, when registered, after the local store and before
	// the packs.
	if p.front != nil {
		front, lower := p.front, p.cacheReads(remote)
		return p.cacheFirst(block_store.NewStoreReadThrough(
			func() block.StoreOps { return recorded },
			func() block.StoreOps {
				return block_store.NewStoreReadThrough(func() block.StoreOps { return front }, lower, nil)
			},
			nil,
		))
	}
	return p.packReadsBehind(recorded, remote)
}

// cacheReads returns the source below the local store: the fill cache over
// the backend, or the backend alone.
func (p *placementState) cacheReads(remote block_store.StoreSource) block_store.StoreSource {
	if p.cache == nil {
		return remote
	}
	return p.cache.Reads(remote)
}

// cacheFirst returns reads behind the fill cache's view, or reads alone
// without a cache. The view holds only copies of blocks the packs hold, so
// serving one before the local store serves the same bytes. A hit skips the
// Volume's read and leaves no read record. That omission only lets local
// eviction drop a copy the cache holds. The view is the cache's only read:
// cacheReads reads the backend alone and fills the cache, so each read looks a
// block up once.
func (p *placementState) cacheFirst(reads block.StoreOps) block.StoreOps {
	if p.cache == nil {
		return reads
	}
	cache := p.cache
	return block_store.NewStoreReadThrough(
		func() block.StoreOps { return cache.View() },
		func() block.StoreOps { return reads },
		nil,
	)
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
	return p.cacheFirst(block_store.NewStoreReadThrough(
		func() block.StoreOps { return recorded },
		p.cacheReads(remote),
		nil,
	))
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

// fillFromFallback stores a block the read fallback or a peer served, which
// the read verified against its hash. The write goes through the write-back
// store, which marks it for upload, below the ownership layer as a cache fill:
// the fallback keeps the block, so the copy takes no ownership and a collector
// sweeping it before the upload loses nothing.
//
// A block the backend holds is the backend's, whichever source served it, as
// when a backend read failed and the fallback answered, or a pack read missed
// a block its row references. It fills as a backend hit does, without a
// marker, so a block the backend holds never queues to upload again. Such a
// fill queues for eviction, so backendPlaces must prove the block durable, not
// take a bare index row's word. Eviction checks the same proof before dropping
// the block.
func (p *placementState) fillFromFallback(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock) error {
	// Serve mode writes nothing.
	if p.serveOnly {
		return nil
	}

	// Refill a block the backend holds without a marker.
	if p.backendPlaces(ctx, ref) {
		return p.fill(ctx, p.local, ref, stored)
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

// backendPlaces reports whether the open backend holds the block in a pack it
// never reclaims. A backend that proves durability from its local index, as
// block_store_writeback.DurableConfirmer describes, answers from the index
// alone, with the proof eviction trusts to drop a last copy, so a fill while
// the bucket fails still needs no upload. Any other backend must have its
// local pack index place the block and its existence check, which probes the
// packs' own indexes, confirm it: a bare row can reference a pack that lacks
// the block or one gone from the bucket, and a fill on the row's word alone
// would queue for eviction a copy that may be the only one. A backend without
// an index, one that reclaims blocks, a backend the placement left, an
// unconfirmed block, or a failed lookup reports false, so the block keeps its
// marker: a block uploaded twice is only stored twice.
func (p *placementState) backendPlaces(ctx context.Context, ref *block.BlockRef) bool {
	// Ask only the open backend of the current placement.
	if p.remote == nil {
		return false
	}
	remote := p.remote.Load()
	if remote == nil || !remote.backend.EqualVT(p.backend.Load()) {
		return false
	}
	locator, ok := remote.store.(block_store_writeback.BlockLocator)
	if !ok || !locator.KeepsBlocks() {
		return false
	}

	// Prove the block from the local index where the backend can.
	refs := []*block.BlockRef{ref}
	if c, ok := remote.store.(block_store_writeback.DurableConfirmer); ok && c.ConfirmsDurable() {
		held, err := c.ConfirmDurable(ctx, refs)
		return err == nil && len(held) == 1 && held[0]
	}

	// Otherwise look the block up in the index.
	placed, err := locator.LocateBlocks(ctx, refs)
	if err != nil || len(placed) != 1 || !placed[0] {
		return false
	}

	// Confirm the block in the backend's packs.
	held, err := remote.store.GetBlockExists(ctx, ref)
	return err == nil && held
}

// fallbackFills is the state of a placed store's fallback fills, shared by the
// read scopes of its fallback reads.
//
// A fill is Queued, then Filling on a worker, then gone: Filled, Dropped by
// the Volume, Failed, or Skipped when the queue was full, the budget spent, or
// the mount ended before its turn. FallbackFillQueue.tla models these states.
type fallbackFills struct {
	// ctx is the mount's context. The fills run under it, and once it ends
	// the queue takes no fill and its workers drop the rest without running
	// them.
	ctx context.Context
	// budget bounds the queued block data across the Provider's stores.
	budget *fillBudget

	// running counts the fills running.
	running atomic.Int64
	// packErrors counts primary-source errors that fell through to the fallback.
	packErrors atomic.Uint64
	// hits counts fallback hits offered for local filling.
	hits atomic.Uint64
	// refsUnknown counts hits whose refs the fallback does not hold.
	refsUnknown atomic.Uint64
	// filled counts fills that landed.
	filled atomic.Uint64
	// dropped counts fills the Volume did not store, as block.ErrRefsUnknown
	// describes.
	dropped atomic.Uint64
	// failed counts failed fills and corrupt fallback blocks rejected before filling.
	failed atomic.Uint64
	// skipped counts fills dropped because the queue or the budget was full,
	// or the mount ended.
	skipped atomic.Uint64

	// mtx guards the queue and the queued fills.
	mtx sync.Mutex
	// queue holds the fills waiting for a slot, oldest first.
	queue []*fallbackFill
	// queued holds the fills queued or filling, by marshaled ref, so a block
	// read again before its fill lands is not queued twice.
	queued map[string]*fallbackFill
}

// fallbackFill is one block waiting to fill the local store.
type fallbackFill struct {
	key    string
	ref    *block.BlockRef
	stored *block.StoredBlock
	fill   func(context.Context, *block.BlockRef, *block.StoredBlock) error
	// taken records that a worker took the fill.
	taken bool
	// again holds the fill with asserted refs that arrived while a worker
	// wrote this one with refs it did not assert, to queue once it ends.
	again *fallbackFill
}

// status returns the counters.
func (f *fallbackFills) status() FallbackStatus {
	return FallbackStatus{
		PackErrors:  f.packErrors.Load(),
		Hits:        f.hits.Load(),
		RefsUnknown: f.refsUnknown.Load(),
		Filled:      f.filled.Load(),
		FillDropped: f.dropped.Load(),
		FillFailed:  f.failed.Load(),
		FillSkipped: f.skipped.Load(),
	}
}

// recordPackError counts a primary-source failure that reached the fallback.
func (f *fallbackFills) recordPackError() {
	f.packErrors.Add(1)
}

// recordHit counts a fallback hit and whether it lacked the refs needed to fill.
func (f *fallbackFills) recordHit(refsKnown bool) {
	f.hits.Add(1)
	if !refsKnown {
		f.refsUnknown.Add(1)
	}
}

// recordFailedFill counts a failed local fill or a corrupt fallback block.
func (f *fallbackFills) recordFailedFill() {
	f.failed.Add(1)
}

// start queues fill to run in the background, and starts a worker when a
// slot is free. The workers run the queue in order, so a burst of fills waits
// for a slot rather than being dropped. A block already queued or filling is
// not queued again, unless the new fill asserts refs the queued one does not:
// it then replaces a waiting fill, or queues after a running one, since the
// Volume may drop the fill whose refs it does not trust. When the queue or the
// budget is full, or the mount has ended, it counts a skip and leaves the
// block for the next read. The fill runs under the mount's context, not the
// reader's, which ends with the read.
func (f *fallbackFills) start(ref *block.BlockRef, stored *block.StoredBlock, fill func(context.Context, *block.BlockRef, *block.StoredBlock) error) {
	// Key the fill by its block.
	job := &fallbackFill{key: ref.MarshalString(), ref: ref, stored: stored, fill: fill}

	// Upgrade a queued fill to asserted refs, or skip the block.
	f.mtx.Lock()
	if queued, ok := f.queued[job.key]; ok {
		if stored.RefsSource.Asserted() && !queued.stored.RefsSource.Asserted() {
			if queued.taken {
				queued.again = job
			} else {
				queued.stored, queued.fill = stored, fill
			}
		}
		f.mtx.Unlock()
		return
	}

	// Queue the fill, unless nothing has room or the mount ended.
	if !f.pushLocked(job) {
		f.mtx.Unlock()
		f.skipped.Add(1)
		return
	}

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

// pushLocked queues job, taking its share of the budget, and reports false
// when the queue or the budget is full or the mount ended. The caller holds
// mtx.
func (f *fallbackFills) pushLocked(job *fallbackFill) bool {
	// Refuse the fill when nothing has room or the mount ended.
	if f.ctx.Err() != nil || len(f.queue) >= fallbackFillQueueBlocks || !f.budget.take(len(job.stored.Data)) {
		return false
	}

	// Queue it under its key.
	if f.queued == nil {
		f.queued = make(map[string]*fallbackFill)
	}
	f.queued[job.key] = job
	f.queue = append(f.queue, job)
	return true
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
		f.queue[0] = nil
		f.queue = f.queue[1:]
		job.taken = true
		f.budget.give(len(job.stored.Data))
		f.mtx.Unlock()

		// Fill the block, unless the mount ended. A dropped or failed
		// fill leaves the block to its next read.
		ended := f.ctx.Err() != nil
		var err error
		if !ended {
			err = job.fill(f.ctx, job.ref, job.stored)
		}

		// Release the block, or queue the asserted fill that waited on it.
		f.mtx.Lock()
		delete(f.queued, job.key)
		again := job.again != nil && f.pushLocked(job.again)
		f.mtx.Unlock()
		if job.again != nil && !again {
			f.skipped.Add(1)
		}
		switch {
		case ended:
			f.skipped.Add(1)
		case errors.Is(err, block.ErrRefsUnknown):
			f.dropped.Add(1)
		case err != nil:
			f.recordFailedFill()
		default:
			f.filled.Add(1)
		}
	}
}

// fallbackReads reads a placed store's own sources, then its read fallback.
// The fallback answers when the sources miss or fail, so an unreachable bucket
// does not hide a block the fallback holds. A failure is never an answer of
// absence: when the fallback misses too, the read returns the error.
//
// A paused backend fails nothing: its reads wait out the pause. So a read of
// the sources races the fallback while the backend is paused, as pausedReads
// and raceAlternates describe, and the fallback serves a block it holds
// without waiting for the backend.
type fallbackReads struct {
	// StoreOps is the placed store's own sources, which reject writes.
	block.StoreOps
	// fallback is the read-only fallback.
	fallback block.StoreOps
	// pause watches the backend below the sources, or holds nil for sources
	// that never wait on a pause.
	pause backendPause
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
	return &fallbackReads{StoreOps: primary, fallback: fallback, pause: r.pause, fills: r.fills, fill: r.fill}, func() {
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
	r.recordPrimaryErr(primaryErr)

	// Read the fallback, filling its hit unless a corrupt local copy failed.
	stored, err := r.readHit(ctx, ref, fillPast(primaryErr))
	return settleFallback(primaryErr, stored, err)
}

// GetStoredBlockBatch reads refs from the sources in one batch, then the
// blocks they missed or failed from the fallback in one batch, answering each
// as GetStoredBlock does.
func (r *fallbackReads) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	// Serve the blocks the sources hold, keeping the rest.
	var miss []*block.BlockRef
	var missAt []int
	var missErr []error
	err := block.GetStoredBlockBatch(ctx, r.StoreOps, refs, func(i int, stored *block.StoredBlock, err error) error {
		if err == nil && stored != nil {
			return fn(i, stored, nil)
		}
		miss, missAt, missErr = append(miss, refs[i]), append(missAt, i), append(missErr, err)
		return nil
	})
	if err != nil {
		return err
	}
	if len(miss) == 0 {
		return nil
	}

	// Ask the fallback about the misses and failures.
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	for _, err := range missErr {
		r.recordPrimaryErr(err)
	}
	return block.GetStoredBlockBatch(ctx, r.fallback, miss, func(j int, stored *block.StoredBlock, err error) error {
		stored, err = r.hit(ctx, miss[j], fillPast(missErr[j]), stored, err)
		stored, err = settleFallback(missErr[j], stored, err)
		return fn(missAt[j], stored, err)
	})
}

// recordPrimaryErr counts a failed read of the sources.
func (r *fallbackReads) recordPrimaryErr(primaryErr error) {
	if primaryErr != nil && r.fills != nil {
		r.fills.recordPackError()
	}
}

// fillPast reports whether a fallback hit after the sources' primaryErr fills
// the local store: not past a corrupt local copy.
func fillPast(primaryErr error) bool {
	return !errors.Is(primaryErr, block.ErrBlockCorrupt)
}

// settleFallback answers a read whose sources failed with primaryErr, or
// missed, and whose fallback read returned stored and err: the hit, else
// primaryErr, else the fallback's error, else a miss.
func settleFallback(primaryErr error, stored *block.StoredBlock, err error) (*block.StoredBlock, error) {
	if err == nil && stored != nil {
		return stored, nil
	}
	if primaryErr != nil && !errors.Is(err, block.ErrBlockCorrupt) {
		return nil, primaryErr
	}
	return nil, err
}

// fallbackHit reads ref from the fallback and fills a hit, as a read racing a
// paused backend asks the fallback. See pausedReads. It returns nil on a miss.
func (r *fallbackReads) fallbackHit(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return r.readHit(ctx, ref, true)
}

// readHit reads ref from the fallback and, when fill is set, fills a hit, as
// hit describes.
func (r *fallbackReads) readHit(ctx context.Context, ref *block.BlockRef, fill bool) (*block.StoredBlock, error) {
	stored, err := r.fallback.GetStoredBlock(ctx, ref)
	return r.hit(ctx, ref, fill, stored, err)
}

// hit answers the fallback's read of ref, stored and err, and fills a hit when
// fill is set. The fallback checks its own bytes: corrupt bytes are a failure,
// not a miss, and fill nothing.
func (r *fallbackReads) hit(ctx context.Context, ref *block.BlockRef, fill bool, stored *block.StoredBlock, err error) (*block.StoredBlock, error) {
	// Count a corrupt block as a failed fill.
	if errors.Is(err, block.ErrBlockCorrupt) {
		if r.fills != nil {
			r.fills.recordFailedFill()
		}
		return nil, errors.Wrap(err, "read fallback block")
	}
	if err != nil || stored == nil {
		return nil, err
	}

	// Serve the block, filling it when asked.
	if fill {
		r.fillHit(ctx, ref, stored)
	}
	return stored, nil
}

// fillHit counts a fallback hit and stores it locally with its upload marker.
// A hit without its refs serves but is not stored, since the pack would keep it
// as a leaf. A hit with listed refs fills, and the local Volume stores only
// what it can take without trusting them, such as the bytes of a block whose
// list is already marked; so the hit is also kept once the reader decodes it.
func (r *fallbackReads) fillHit(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock) {
	// Count only hits on a read path that fills the local store.
	if r.fills == nil || r.fill == nil {
		return
	}
	r.fills.recordHit(stored.RefsSource.Recorded())

	// Leave a block without refs to serve until its reader decodes them.
	if !stored.RefsSource.Recorded() {
		return
	}

	// Fill it, and refill it with asserted refs once the reader decodes it.
	fills, fill := r.fills, r.fill
	fills.start(ref, stored, fill)
	block.KeepDecodedRefs(ctx, ref, stored, func(_ context.Context, kept *block.StoredBlock) {
		fills.start(ref, kept, fill)
	})
}

// GetBlockExists checks the sources, then the fallback.
func (r *fallbackReads) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Report a block the sources hold, or the fallback confirmed in a pause.
	res := raceAlternates(ctx, r.pause, func(ctx context.Context) (bool, error) {
		return r.StoreOps.GetBlockExists(ctx, ref)
	}, existsHit, func(ctx context.Context) (bool, error) {
		return r.fallback.GetBlockExists(ctx, ref)
	})
	if res.fromAlternate || (res.err == nil && res.v) {
		return true, nil
	}

	// Ask the fallback about a miss or a failure, unless the race did.
	if ctx.Err() != nil {
		return false, context.Cause(ctx)
	}
	var fallbackErr error
	if len(res.missed) != 0 {
		fallbackErr = res.missed[0].err
	} else {
		var fallbackFound bool
		fallbackFound, fallbackErr = r.fallback.GetBlockExists(ctx, ref)
		if fallbackErr == nil && fallbackFound {
			return true, nil
		}
	}
	if res.err != nil {
		return false, res.err
	}
	return false, fallbackErr
}

// GetBlockExistsBatch checks the sources for every ref, then asks the fallback
// about the refs they did not confirm. While the backend is paused the
// fallback is asked about every ref beside the sources and answers when it
// confirms them all. A partial answer keeps the refs it confirmed, the
// sources answer the rest, and the fallback is not asked again.
func (r *fallbackReads) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check the sources, racing the fallback in a pause, and treat a failure
	// as no answer.
	res := raceAlternates(ctx, r.pause, func(ctx context.Context) ([]bool, error) {
		return r.StoreOps.GetBlockExistsBatch(ctx, refs)
	}, allExistHit, func(ctx context.Context) ([]bool, error) {
		return r.fallback.GetBlockExistsBatch(ctx, refs)
	})
	out := make([]bool, len(refs))
	if res.fromAlternate {
		copy(out, res.v)
		return out, nil
	}
	err := res.err
	if err == nil {
		copy(out, res.v)
	}

	// Keep what the race's fallback answer confirmed. That answer covers
	// every ref for the fallback.
	if len(res.missed) != 0 && res.missed[0].err == nil {
		for i, ok := range res.missed[0].v {
			if i < len(out) {
				out[i] = out[i] || ok
			}
		}
		if err != nil {
			return nil, err
		}
		return out, nil
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
var (
	_ block.StoreOps           = (*fallbackReads)(nil)
	_ block.StoredBlockBatcher = (*fallbackReads)(nil)
)
