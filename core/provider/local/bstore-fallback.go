package provider_local

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// fallbackFillSlots bounds the fills of one placed store that run at once.
const fallbackFillSlots = 64

// readChains builds the read paths of a placed store: reads serve the blocks of
// the local store, then the backend's packs, then the read fallback if any,
// then peers. The copy path serves the same sources without filling the local
// store.
//
// Each source fills the local store with its own function. A block read from
// the backend refills the disk without a marker, as the backend holds it. A
// block only the fallback holds is stored with an upload marker, so it uploads
// to the backend, the marker keeps eviction from dropping it before then, and
// later reads find it there instead of the fallback.
//
// With no fallback, reads and copies are the local store over the backend and
// peers, as they were before fallbacks existed.
func (p *placementState) readChains(
	recorded, local block.StoreOps,
	remote, peers block_store.StoreSource,
	fallback block.StoreOps,
) (reads, copies block.StoreOps) {
	// Read the backend and then peers when the placement has no fallback.
	if fallback == nil {
		lower := block_store.NewStoreReadThrough(remote, peers, nil)
		reads = block_store.NewStoreReadThrough(
			func() block.StoreOps { return recorded },
			func() block.StoreOps { return lower },
			p.fill,
		)
		copies = block_store.NewStoreReadThrough(
			func() block.StoreOps { return local },
			func() block.StoreOps { return lower },
			nil,
		)
		return reads, copies
	}

	// Read the local store, then the backend, then the fallback, then peers.
	// The fallback is read only: nothing writes SQL in this mode.
	fallback = block.NewReadOnlyStore(fallback)
	packs := block_store.NewStoreReadThrough(
		func() block.StoreOps { return recorded },
		remote,
		p.fill,
	)
	withFallback := &fallbackReads{StoreOps: packs, fallback: fallback, fills: &p.fallbackFills, fill: p.fillFromFallback}
	reads = block_store.NewStoreReadThrough(
		func() block.StoreOps { return withFallback },
		peers,
		func(ctx context.Context, _ block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
			return p.fill(ctx, recorded, ref, stored)
		},
	)

	// Copies read the same sources and write nothing.
	copyPacks := block_store.NewStoreReadThrough(
		func() block.StoreOps { return local },
		remote,
		nil,
	)
	copies = block_store.NewStoreReadThrough(
		func() block.StoreOps { return &fallbackReads{StoreOps: copyPacks, fallback: fallback} },
		peers,
		nil,
	)
	return reads, copies
}

// fillFromFallback stores a block only the read fallback holds, which the read
// verified against its hash. The write goes through the write-back store,
// which marks it for upload, below the ownership layer as a cache fill: the
// fallback keeps the block, so the copy takes no ownership and a collector
// sweeping it before the upload loses nothing. Tracked or not, the backend does
// not hold the block, so it never takes the refill path of a backend hit.
func (p *placementState) fillFromFallback(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock) error {
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
type fallbackFills struct {
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
	// skipped counts fills skipped for want of a slot.
	skipped atomic.Uint64
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

// start runs fill in the background when a slot is free, and counts the
// outcome. When every slot is busy it counts a skip and leaves the block for
// the next read.
func (f *fallbackFills) start(ctx context.Context, ref *block.BlockRef, stored *block.StoredBlock, fill func(context.Context, *block.BlockRef, *block.StoredBlock) error) {
	// Bound the fills, as the block store's read-through bounds its own.
	if f.running.Add(1) > fallbackFillSlots {
		f.running.Add(-1)
		f.skipped.Add(1)
		return
	}

	// Fill in the background so the read does not wait on the local writer.
	fillCtx := context.WithoutCancel(ctx)
	go func() {
		defer f.running.Add(-1)
		if err := fill(fillCtx, ref, stored); err != nil {
			f.failed.Add(1)
			return
		}
		f.filled.Add(1)
	}()
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
// or failed with primaryErr. A fallback hit is filled into the local store.
// Without a hit, it returns primaryErr, else the fallback's error, else a miss.
func (r *fallbackReads) readFallback(ctx context.Context, ref *block.BlockRef, primaryErr error) (*block.StoredBlock, error) {
	// A failure of the caller's own context is not the sources' failure.
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if primaryErr != nil && r.fills != nil {
		r.fills.packErrors.Add(1)
	}

	// Read the fallback and fill its hit.
	stored, err := r.fallback.GetStoredBlock(ctx, ref)
	if err == nil && stored != nil {
		// Serve and fill only bytes that hash to ref. Corrupt bytes are a
		// failure, not a miss.
		if !ref.GetEmpty() {
			if _, err := ref.GetHash().VerifyData(stored.Data); err != nil {
				if r.fills != nil {
					r.fills.failed.Add(1)
				}
				return nil, errors.Wrap(err, "verify fallback block")
			}
		}
		r.fillHit(ctx, ref, stored)
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
	r.fills.start(ctx, ref, stored, r.fill)
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

// StatBlock follows the same source order as GetBlock.
func (r *fallbackReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := r.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

// _ is a type assertion
var _ block.StoreOps = (*fallbackReads)(nil)
