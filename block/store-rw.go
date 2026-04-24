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

// GetSupportedFeatures returns the native feature bitmask for the store.
func (b *StoreRW) GetSupportedFeatures() StoreFeature {
	var out StoreFeature
	if b.writeHandle != nil {
		features := b.writeHandle.GetSupportedFeatures()
		out |= features & StoreFeatureNativeBatchPut
		out |= features & StoreFeatureNativeBackgroundPut
		out |= features & StoreFeatureNativeFlush
		out |= features & StoreFeatureNativeDeferFlush
	}
	if b.readHandle != nil {
		out |= b.readHandle.GetSupportedFeatures() & StoreFeatureNativeBatchExists
	}
	return out
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

// GetBlockExistsBatch forwards batched existence probes to the read handle when supported.
func (b *StoreRW) GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return b.readHandle.GetBlockExistsBatch(ctx, refs)
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
	return b.writeHandle.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards to the write handle if it supports background writes.
func (b *StoreRW) PutBlockBackground(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return b.writeHandle.PutBlockBackground(ctx, data, opts)
}

// Flush forwards the durability boundary to the write handle.
func (b *StoreRW) Flush(ctx context.Context) error {
	return b.writeHandle.Flush(ctx)
}

// BeginDeferFlush forwards to the write handle if it supports deferred flushing.
func (b *StoreRW) BeginDeferFlush() {
	b.writeHandle.BeginDeferFlush()
}

// EndDeferFlush forwards to the write handle if it supports deferred flushing.
func (b *StoreRW) EndDeferFlush(ctx context.Context) error {
	return b.writeHandle.EndDeferFlush(ctx)
}

// _ is a type assertion
var (
	_ StoreOps = ((*StoreRW)(nil))
)
