package volume

import (
	"context"

	hash "github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
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
	if batcher, ok := v.store.(block.BatchExistsStore); ok {
		return batcher.GetBlockExistsBatch(ctx, refs)
	}

	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := v.store.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
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
	batcher, ok := v.store.(block.BatchPutStore)
	if !ok {
		for _, entry := range entries {
			if entry.Tombstone {
				if err := v.store.RmBlock(ctx, entry.Ref); err != nil {
					return err
				}
				continue
			}
			if _, _, err := v.store.PutBlock(ctx, entry.Data, &block.PutOpts{
				ForceBlockRef: entry.Ref.Clone(),
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return batcher.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the wrapped block store when supported.
func (v *VolumeBlockStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	bg, ok := v.store.(block.BackgroundPutStore)
	if !ok {
		return v.store.PutBlock(ctx, data, opts)
	}
	return bg.PutBlockBackground(ctx, data, opts)
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
	_ Volume                   = ((*VolumeBlockStore)(nil))
	_ block_store.Store        = ((*VolumeBlockStore)(nil))
	_ block.BatchExistsStore   = ((*VolumeBlockStore)(nil))
	_ block.BatchPutStore      = ((*VolumeBlockStore)(nil))
	_ block.BackgroundPutStore = ((*VolumeBlockStore)(nil))
)
