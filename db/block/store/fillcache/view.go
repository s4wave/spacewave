package block_store_fillcache

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// view is the read side of a Cache as a block store: it serves the blocks the
// cache holds and nothing else. A failure of the cache is a miss, so a read
// through a view never fails for the cache's sake, and a block that no longer
// hashes to its reference is a miss and is removed. It reports no block as
// existing, so an existence check asks the store below the cache, which alone
// is the truth of what exists, and it takes no writes.
type view struct {
	// cache is the cache the view reads.
	cache *Cache
}

// GetHashType reports no hash type, so a read-through takes the lower store's.
func (v *view) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures reports no native features.
func (v *view) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the view itself, which holds no read state.
func (v *view) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return v, func() {}, nil
}

// PutBlock refuses the write: only Fill adds to the cache.
func (v *view) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch refuses the writes: only Fill adds to the cache.
func (v *view) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, block_store.ErrReadOnly
}

// GetBlock serves the block the cache holds.
func (v *view) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored := v.get(ctx, ref)
	if stored == nil {
		return nil, false, nil
	}
	return stored.Data, true, nil
}

// GetStoredBlock serves the block the cache holds, with the refs it was read
// with.
func (v *view) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return v.get(ctx, ref), nil
}

// GetStoredBlockBatch serves each block the cache holds in turn: a lookup is
// a local read, so the batch runs no goroutines.
func (v *view) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	for i, ref := range refs {
		if err := ctx.Err(); err != nil {
			return context.Cause(ctx)
		}
		if err := fn(i, v.get(ctx, ref), nil); err != nil {
			return err
		}
	}
	return nil
}

// get returns the block the cache holds intact, or nil.
//
// A hit is marked a durable read, so its decoded block enters the shared
// decoded-block pool: every copy is a verified copy of a block the durable
// store holds. The removal generation is taken before the read, so a removal
// reported while it runs keeps the block out of the pool.
//
// A copy that fails to decode or to hash to ref is removed, so the fill that
// follows the miss can land: the cache holds only copies of blocks a lower
// store holds, so removing one loses nothing, and the read falls through to
// the lower store, which serves and refills the block.
func (v *view) get(ctx context.Context, ref *block.BlockRef) *block.StoredBlock {
	// Read the copy, reading a failure as a miss.
	if ref.GetEmpty() {
		return nil
	}
	c := v.cache
	gen := block.DurableReadGeneration()
	value, found, err := c.packs.Get(packfile.BlockKey(ref.GetHash()))
	if err != nil {
		c.warn.warn(err, "the block fill cache failed a read, reading the durable store")
	}
	if err != nil || !found {
		c.counts.misses.Add(1)
		return nil
	}

	// Check it, removing a damaged copy.
	stored, err := block.DecodeBlockObject(value)
	if err == nil {
		err = block.VerifyRead(ref, stored.Data, "")
	}
	if err != nil {
		c.remove(ref)
		c.counts.misses.Add(1)
		c.warn.warn(err, "the block fill cache held a damaged block, reading the durable store")
		return nil
	}

	// Serve it.
	c.counts.hits.Add(1)
	block.MarkDurableRead(ctx, ref, gen)
	return stored
}

// GetBlockExists reports the block absent, as the view's doc states.
func (v *view) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch reports every block absent, as the view's doc states.
func (v *view) GetBlockExistsBatch(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// RmBlock refuses the removal: the cache forgets blocks only by eviction.
func (v *view) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// StatBlock reports the block absent, as the view's doc states.
func (v *view) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

// Sync reports nothing to make durable: the view takes no writes.
func (v *view) Sync(context.Context) (bool, error) {
	return true, nil
}

// GetRoot reports that no name is held: the view holds no roots.
func (v *view) GetRoot(context.Context, string) (*block.BlockRef, error) {
	return nil, nil
}

// ListRoots reports no roots.
func (v *view) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots refuses the write: only Fill adds to the cache.
func (v *view) SetRoots(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error {
	return block_store.ErrReadOnly
}

// _ is a type assertion
var (
	_ block.StoreOps           = (*view)(nil)
	_ block.StoredBlockBatcher = (*view)(nil)
)
