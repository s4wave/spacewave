package block

import (
	"context"

	hash "github.com/s4wave/spacewave/net/hash"
)

// StoreRW combines a read and write store together.
type StoreRW struct {
	// StoreOps is the write handle. It answers the writes and the root calls,
	// so a Previous read from GetRoot is the reference SetRoots compares
	// against.
	StoreOps
	readHandle StoreOps
}

// NewStoreRW constructs a new Store handle using a read handle and an optional
// write handle. If the write handle is not nil, the write (put and delete)
// calls will go to it. Otherwise, all calls are sent to the read handle.
func NewStoreRW(readHandle, writeHandle StoreOps) StoreOps {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &StoreRW{StoreOps: writeHandle, readHandle: readHandle}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *StoreRW) GetHashType() hash.HashType {
	if b.StoreOps != nil {
		return b.StoreOps.GetHashType()
	}
	if b.readHandle != nil {
		return b.readHandle.GetHashType()
	}
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (b *StoreRW) GetSupportedFeatures() StoreFeature {
	var out StoreFeature
	if b.StoreOps != nil {
		features := b.StoreOps.GetSupportedFeatures()
		out |= features & StoreFeatureNativeBatchPut
	}
	if b.readHandle != nil {
		out |= b.readHandle.GetSupportedFeatures() & StoreFeatureNativeBatchExists
	}
	return out
}

// BeginReadOperation opens a read scope on the read handle.
func (b *StoreRW) BeginReadOperation(ctx context.Context) (StoreOps, func(), error) {
	readHandle, release, err := b.readHandle.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &StoreRW{StoreOps: readHandle, readHandle: readHandle}, release, nil
}

// EnsureDecodedBlockCacheFresh forwards decoded-cache freshness to the read handle.
func (b *StoreRW) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	freshener, ok := b.readHandle.(DecodedBlockCacheFreshener)
	if !ok {
		return nil
	}
	return freshener.EnsureDecodedBlockCacheFresh(ctx)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *StoreRW) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	return b.readHandle.GetBlock(ctx, ref)
}

// GetStoredBlock gets a block and its references from the read handle.
func (b *StoreRW) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	return b.readHandle.GetStoredBlock(ctx, ref)
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

// HoldCompleteRoots forwards a stage hold of proved roots to the write handle, never the read handle.
func (b *StoreRW) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return HoldCompleteRoots(ctx, b.StoreOps, refs)
}

// _ is a type assertion
var (
	_ StoreOps                   = (*StoreRW)(nil)
	_ DecodedBlockCacheFreshener = (*StoreRW)(nil)
	_ RootProofs                 = (*StoreRW)(nil)
)
