package volume

import (
	"context"

	hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
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

// RmBlock deletes a block from the bucket.
func (v *VolumeBlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return v.store.RmBlock(ctx, ref)
}

// _ is a type assertion
var (
	_ Volume            = ((*VolumeBlockStore)(nil))
	_ block_store.Store = ((*VolumeBlockStore)(nil))
)
