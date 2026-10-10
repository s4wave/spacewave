package volume_scoped

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// blockStore serves the blocks of a scoped volume view.
//
// A block hash is the capability to read the block, so reads and writes pass
// through. The volume's collector owns deletion, since content addressing
// shares blocks between owners: the view refuses RmBlock and tombstones.
type blockStore struct {
	// ops is the block store of the underlying volume.
	ops block.StoreOps
}

// GetHashType returns the preferred hash type of the underlying store.
func (b *blockStore) GetHashType() hash.HashType {
	return b.ops.GetHashType()
}

// GetSupportedFeatures returns the feature bitmask of the underlying store.
func (b *blockStore) GetSupportedFeatures() block.StoreFeature {
	return b.ops.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the underlying store and wraps it
// in the same refusals.
func (b *blockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	ops, release, err := b.ops.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &blockStore{ops: ops}, release, nil
}

// PutBlock puts a block into the underlying store.
func (b *blockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.ops.PutBlock(ctx, data, opts)
}

// PutBlockBatch puts a batch of blocks into the underlying store. It refuses a
// batch that tombstones a block.
func (b *blockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Refuse deletions before any entry is written.
	for _, entry := range entries {
		if entry.Tombstone {
			return nil, errors.Wrap(ErrRefused, "tombstone block")
		}
	}

	return b.ops.PutBlockBatch(ctx, entries)
}

// GetBlock gets a block from the underlying store.
func (b *blockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.ops.GetBlock(ctx, ref)
}

// GetStoredBlock gets a block and its outgoing references from the underlying
// store.
func (b *blockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return b.ops.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks if the underlying store holds the block.
func (b *blockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.ops.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks if the underlying store holds each block.
func (b *blockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.ops.GetBlockExistsBatch(ctx, refs)
}

// RmBlock refuses to delete a block.
func (b *blockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return errors.Wrap(ErrRefused, "remove block")
}

// StatBlock returns metadata about a block in the underlying store.
func (b *blockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.ops.StatBlock(ctx, ref)
}

// Sync drains the buffered writes of the underlying store.
func (b *blockStore) Sync(ctx context.Context) (bool, error) {
	return b.ops.Sync(ctx)
}

// _ is a type assertion
var _ block.StoreOps = (*blockStore)(nil)
