package volume

import (
	"context"

	hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	block_store "github.com/aperturerobotics/hydra/block/store"
)

// VolumeBlockStore wraps a volume with a block store.
type VolumeBlockStore struct {
	Volume
	store block.StoreOps
}

// NewVolumeBlockStore constructs a new wrapper with a block store around a volume.
func NewVolumeBlockStore(vol Volume, blockStore block.StoreOps) *VolumeBlockStore {
	return &VolumeBlockStore{Volume: vol, store: blockStore}
}

// GetHashType returns the preferred hash type for the store.
func (v *VolumeBlockStore) GetHashType() hash.HashType {
	return v.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (v *VolumeBlockStore) GetSupportedFeatures() block.StoreFeature {
	return v.store.GetSupportedFeatures()
}

// PutBlock puts a block into the store.
func (v *VolumeBlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return v.store.PutBlock(ctx, data, opts)
}

// GetBlock gets a block with the given reference.
func (v *VolumeBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return v.store.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
func (v *VolumeBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return v.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the wrapped block store when supported.
func (v *VolumeBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return v.store.GetBlockExistsBatch(ctx, refs)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (v *VolumeBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return v.store.StatBlock(ctx, ref)
}

// RmBlock deletes a block from the bucket.
func (v *VolumeBlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return v.store.RmBlock(ctx, ref)
}

// PutBlockBatch forwards batched writes to the wrapped block store when supported.
func (v *VolumeBlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return v.store.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the wrapped block store when supported.
func (v *VolumeBlockStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return v.store.PutBlockBackground(ctx, data, opts)
}

// Flush forwards to the wrapped block store.
func (v *VolumeBlockStore) Flush(ctx context.Context) error {
	return v.store.Flush(ctx)
}

// BeginDeferFlush opens a defer-flush scope on the wrapped block store.
func (v *VolumeBlockStore) BeginDeferFlush() {
	v.store.BeginDeferFlush()
}

// EndDeferFlush closes a defer-flush scope on the wrapped block store.
func (v *VolumeBlockStore) EndDeferFlush(ctx context.Context) error {
	return v.store.EndDeferFlush(ctx)
}

// GetGCManagerHooks forwards WAL-backed GC manager hooks from the wrapped
// volume when available.
func (v *VolumeBlockStore) GetGCManagerHooks() (block_gc.ManagerHooks, bool) {
	provider, ok := v.Volume.(interface {
		GetGCManagerHooks() (block_gc.ManagerHooks, bool)
	})
	if !ok {
		return block_gc.ManagerHooks{}, false
	}
	return provider.GetGCManagerHooks()
}

// _ is a type assertion
var (
	_ Volume            = ((*VolumeBlockStore)(nil))
	_ block_store.Store = ((*VolumeBlockStore)(nil))
)
