package block_store_fillcache

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// view is the read side of a Cache as a block store: it serves the blocks the
// cache holds and nothing else. A failure of the cache is a miss, so a read
// through a view never fails for the cache's sake, and a block that no longer
// hashes to its reference is a miss and is removed. It reports no block as
// existing, so an existence check asks the store below the cache, which alone
// is the truth of what exists.
type view struct {
	// StoreOps is the cache's block store, read only.
	block.StoreOps
	// cache is the cache the view reads.
	cache *Cache
}

// BeginReadOperation returns the view itself, which holds no read state.
func (v *view) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return v, func() {}, nil
}

// GetBlock reads the block from the cache. A failure of the cache, or a block
// that fails its hash, reads as a miss.
//
// A hit is marked a durable read, so its decoded block enters the shared
// decoded-block pool: every row is a verified copy of a block the packs hold.
// The removal generation is taken before the read, so a removal reported
// while it runs keeps the block out of the pool.
func (v *view) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Serve the block when the cache holds it intact.
	gen := block.DurableReadGeneration()
	data, found, err := v.StoreOps.GetBlock(ctx, ref)
	if err == nil {
		if found {
			v.cache.counts.hits.Add(1)
			block.MarkDurableRead(ctx, ref, gen)
		} else {
			v.cache.counts.misses.Add(1)
		}
		return data, found, nil
	}
	v.cache.counts.misses.Add(1)

	// Leave a canceled read to the caller.
	if ctx.Err() != nil {
		return nil, false, context.Cause(ctx)
	}

	// Remove a block that failed its hash, so the fill that follows the miss
	// can land, and read any other failure as a miss. The cache holds only
	// copies of blocks a lower store holds, so removing one loses nothing:
	// this is the cache's only remove, it drops the one row the corrupt read
	// named, and the read falls through to the lower store, which serves and
	// refills the block.
	if errors.Is(err, block.ErrBlockCorrupt) {
		v.cache.remove(ref)
	}
	v.cache.warn.warn(err, "the block fill cache failed a read, reading the packs")
	return nil, false, nil
}

// GetStoredBlock reads the block from the cache. The cache keeps the bytes of
// a block without its references, so refs are unknown.
func (v *view) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, v, ref)
}

// GetBlockExists reports that the block is absent, as the view's doc states.
func (v *view) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch reports every block absent, as the view's doc states.
func (v *view) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// StatBlock reports the block as absent, as the view's doc states.
func (v *view) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

// _ is a type assertion
var _ block.StoreOps = (*view)(nil)
