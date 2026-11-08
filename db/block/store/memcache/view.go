package block_store_memcache

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// view is the read side of a Cache as a block store: it serves the blocks the
// cache holds and nothing else. It reports no block as existing, so an
// existence check asks the store below the cache, which alone is the truth of
// what exists, and it takes no writes.
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
func (v *view) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored := v.cache.get(ref)
	if stored == nil {
		return nil, false, nil
	}
	return stored.Data, true, nil
}

// GetStoredBlock serves the block the cache holds, with the refs it was read
// with.
func (v *view) GetStoredBlock(_ context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return v.cache.get(ref), nil
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
var _ block.StoreOps = (*view)(nil)
