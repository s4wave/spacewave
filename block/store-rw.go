package block

import (
	"context"

	hash "github.com/aperturerobotics/bifrost/hash"
)

// StoreRW combines a read and write store together.
type StoreRW struct {
	readHandle  StoreOps
	writeHandle StoreOps
}

// NewStoreRW constructs a new Store handle using a read handle and an optional
// write handle. If the write handle is not nil, the write (put and delete)
// calls will go to it. Otherwise, all calls are sent to the read handle.
func NewStoreRW(readHandle, writeHandle StoreOps) StoreOps {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &StoreRW{
		readHandle:  readHandle,
		writeHandle: writeHandle,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *StoreRW) GetHashType() hash.HashType {
	if b.writeHandle != nil {
		return b.writeHandle.GetHashType()
	}
	if b.readHandle != nil {
		return b.readHandle.GetHashType()
	}
	return 0
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *StoreRW) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return b.writeHandle.PutBlock(ctx, data, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *StoreRW) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	return b.readHandle.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *StoreRW) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	return b.readHandle.GetBlockExists(ctx, ref)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (b *StoreRW) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	return b.readHandle.StatBlock(ctx, ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *StoreRW) RmBlock(ctx context.Context, ref *BlockRef) error {
	return b.writeHandle.RmBlock(ctx, ref)
}

// PutBlockBatch forwards to the write handle if it supports batched writes.
func (b *StoreRW) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	if batcher, ok := b.writeHandle.(BatchPutStore); ok {
		return batcher.PutBlockBatch(ctx, entries)
	}
	for _, entry := range entries {
		if entry.Tombstone {
			if err := b.writeHandle.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		if _, _, err := b.writeHandle.PutBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: entry.Ref.Clone(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// BeginDeferFlush forwards to the write handle if it supports deferred flushing.
func (b *StoreRW) BeginDeferFlush() {
	if df, ok := b.writeHandle.(DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards to the write handle if it supports deferred flushing.
func (b *StoreRW) EndDeferFlush(ctx context.Context) error {
	if df, ok := b.writeHandle.(DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ StoreOps       = ((*StoreRW)(nil))
	_ BatchPutStore  = ((*StoreRW)(nil))
	_ DeferFlushable = ((*StoreRW)(nil))
)
