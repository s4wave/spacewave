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
	if batcher, ok := k.blk.(block.BatchExistsStore); ok {
		return batcher.GetBlockExistsBatch(ctx, refs)
	}

	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := k.blk.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
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
	batcher, ok := k.blk.(block.BatchPutStore)
	if !ok {
		for _, entry := range entries {
			if entry.Tombstone {
				if err := k.blk.RmBlock(ctx, entry.Ref); err != nil {
					return err
				}
				continue
			}
			if _, _, err := k.blk.PutBlock(ctx, entry.Data, &block.PutOpts{
				ForceBlockRef: entry.Ref.Clone(),
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return batcher.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the underlying block store when supported.
func (k *KVTx) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	bg, ok := k.blk.(block.BackgroundPutStore)
	if !ok {
		return k.blk.PutBlock(ctx, data, opts)
	}
	return bg.PutBlockBackground(ctx, data, opts)
}

// BeginDeferFlush forwards deferred-flush scope entry to the underlying block store when supported.
func (k *KVTx) BeginDeferFlush() {
	if df, ok := k.blk.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards deferred-flush scope exit to the underlying block store when supported.
func (k *KVTx) EndDeferFlush(ctx context.Context) error {
	if df, ok := k.blk.(block.DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.StoreOps           = ((*KVTx)(nil))
	_ block.BatchExistsStore   = ((*KVTx)(nil))
	_ block.BatchPutStore      = ((*KVTx)(nil))
	_ block.BackgroundPutStore = ((*KVTx)(nil))
	_ block.DeferFlushable     = ((*KVTx)(nil))
)
