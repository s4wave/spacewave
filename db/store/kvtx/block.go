package store_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	hash "github.com/s4wave/spacewave/net/hash"
)

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (k *KVTx) GetHashType() hash.HashType {
	return k.blk.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (k *KVTx) GetSupportedFeatures() block.StoreFeature {
	return k.blk.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the underlying block store.
func (k *KVTx) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	blk, release, err := k.blk.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &KVTx{
		conf:  k.conf,
		kvkey: k.kvkey,
		blk:   blk,
		store: k.store,
	}, release, nil
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
func (k *KVTx) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return k.blk.PutBlock(ctx, data, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (k *KVTx) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return k.blk.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (k *KVTx) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return k.blk.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the underlying block store when supported.
func (k *KVTx) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return k.blk.GetBlockExistsBatch(ctx, refs)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (k *KVTx) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return k.blk.StatBlock(ctx, ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (k *KVTx) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return k.blk.RmBlock(ctx, ref)
}

// PutBlockBatch forwards batched writes to the underlying block store when supported.
func (k *KVTx) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return k.blk.PutBlockBatch(ctx, entries)
}

// Sync forwards the durability barrier to the underlying block store.
func (k *KVTx) Sync(ctx context.Context) (bool, error) {
	return k.blk.Sync(ctx)
}

// BeginDeferFlush forwards the GC defer-flush scope to the underlying block store.
func (k *KVTx) BeginDeferFlush() {
	block.BeginDeferFlush(k.blk)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the underlying block store.
func (k *KVTx) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, k.blk)
}

// _ is a type assertion
var (
	_ block.StoreOps     = ((*KVTx)(nil))
	_ block.DeferFlusher = ((*KVTx)(nil))
)
