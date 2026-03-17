package block

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
)

// StoreOps can read/write blocks.
type StoreOps interface {
	// GetHashType returns the preferred hash type for the store.
	// This should return as fast as possible (called frequently).
	// If 0 is returned, uses a default defined by Hydra.
	GetHashType() hash.HashType
	// PutBlock puts a block into the store.
	// The ref should not be modified after return.
	// The second return value can optionally indicate if the block already existed.
	// If the hash type is unset, use the type from GetHashType().
	PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error)
	// GetBlock gets a block with the given reference.
	// The ref should not be modified or retained by GetBlock.
	// Returns data, found, error.
	// Returns nil, false, nil if not found.
	// Note: the block may not be in the specified bucket.
	GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error)
	// GetBlockExists checks if a block exists with a cid reference.
	// The ref should not be modified or retained by GetBlock.
	// Note: the block may not be in the specified bucket.
	GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error)
	// RmBlock deletes a block from the bucket.
	// Does not return an error if the block was not present.
	// In some cases, will return before confirming delete.
	RmBlock(ctx context.Context, ref *BlockRef) error
	// StatBlock returns metadata about a block without reading its data.
	// Returns nil, nil if the block does not exist.
	StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error)
}

// BlockStat contains metadata about a stored block.
type BlockStat struct {
	// Ref is the block reference.
	Ref *BlockRef
	// Size is the block data size in bytes.
	// May be -1 if the size is unknown.
	Size int64
}

// PutBlock marshals & puts a block into a bucket.
func PutBlock(ctx context.Context, bk StoreOps, b Block) (*BlockRef, bool, error) {
	dat, err := b.MarshalBlock()
	if err != nil {
		return nil, false, err
	}
	return bk.PutBlock(ctx, dat, nil)
}
