package block

import (
	"context"

	hash "github.com/s4wave/spacewave/net/hash"
)

// StoreRW combines a read and write store together.
type StoreRW struct {
	readHandle          StoreOps
	writeHandle         StoreOps
	publicationReadOnly bool
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
	return &StoreRW{readHandle: readHandle, writeHandle: readHandle, publicationReadOnly: true}, release, nil
}

// EnsureDecodedBlockCacheFresh forwards decoded-cache freshness to the read handle.
func (b *StoreRW) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	freshener, ok := b.readHandle.(DecodedBlockCacheFreshener)
	if !ok {
		return nil
	}
	return freshener.EnsureDecodedBlockCacheFresh(ctx)
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

// Sync forwards the durability barrier to the write handle.
func (b *StoreRW) Sync(ctx context.Context) (bool, error) {
	return b.writeHandle.Sync(ctx)
}

// BeginDeferFlush forwards the GC defer-flush scope to the write handle.
func (b *StoreRW) BeginDeferFlush() {
	BeginDeferFlush(b.writeHandle)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the write handle.
func (b *StoreRW) EndDeferFlush(ctx context.Context) error {
	return EndDeferFlush(ctx, b.writeHandle)
}

// SupportsAtomicPublication follows the write domain, never a cross-volume lookup.
func (b *StoreRW) SupportsAtomicPublication() bool {
	if b.publicationReadOnly {
		return false
	}
	p, ok := b.writeHandle.(AtomicPublisher)
	return ok && p.SupportsAtomicPublication()
}

// AtomicPublicationVolumeID returns the write domain's shared publication namespace.
func (b *StoreRW) AtomicPublicationVolumeID() string {
	if !b.SupportsAtomicPublication() {
		return ""
	}
	return b.writeHandle.(AtomicPublisher).AtomicPublicationVolumeID()
}

// SubmitAtomic forwards admission to the write handle's publisher.
func (b *StoreRW) SubmitAtomic(ctx context.Context, p *AtomicPublication) (*PublicationReceipt, error) {
	if !b.SupportsAtomicPublication() {
		return nil, ErrAtomicPublicationUnsupported
	}
	return b.writeHandle.(AtomicPublisher).SubmitAtomic(ctx, p)
}

// PublishAtomic forwards the uncancelled durability wait to the write handle.
func (b *StoreRW) PublishAtomic(ctx context.Context, p *AtomicPublication) error {
	if !b.SupportsAtomicPublication() {
		return ErrAtomicPublicationUnsupported
	}
	return b.writeHandle.(AtomicPublisher).PublishAtomic(ctx, p)
}

// _ is a type assertion
var (
	_ StoreOps                   = (*StoreRW)(nil)
	_ DecodedBlockCacheFreshener = (*StoreRW)(nil)
	_ AtomicPublisher            = (*StoreRW)(nil)
)
