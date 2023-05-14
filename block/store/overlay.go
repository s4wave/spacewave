package block_store

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
)

// writebackTimeout is the maximum time we can take to write back a block.
const writebackTimeout = time.Minute

// Overlay layers an upper block store over a lower store.
type Overlay struct {
	lower, upper Store
	mode         BlockStoreMode
}

// NewOverlay constructs a new overlay store.
func NewOverlay(lower, upper Store, mode BlockStoreMode) *Overlay {
	return &Overlay{lower: lower, upper: upper, mode: mode}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (o *Overlay) GetHashType() hash.HashType {
	if v := o.upper.GetHashType(); v != 0 {
		return v
	}
	return o.lower.GetHashType()
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
// If the hash type is unset, use the type from GetHashType().
func (o *Overlay) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	cacheMode := func(lower, upper Store) (*block.BlockRef, bool, error) {
		ref, existed, err := upper.PutBlock(ctx, data, opts)
		if err != nil {
			return nil, false, err
		}
		lowerOpts := opts.CloneVT()
		lowerOpts.ForceBlockRef = ref
		_, lowerExisted, err := lower.PutBlock(ctx, data, lowerOpts)
		if err != nil {
			return nil, false, err
		}
		return ref, existed && lowerExisted, nil
	}
	switch o.mode {
	default:
		fallthrough
	case BlockStoreMode_BlockStoreMode_DIRECT:
		return o.upper.PutBlock(ctx, data, opts)
	case BlockStoreMode_BlockStoreMode_CACHE:
		return cacheMode(o.lower, o.upper)
	case BlockStoreMode_BlockStoreMode_CACHE_LOWER:
		return cacheMode(o.upper, o.lower)
	}
}

// GetBlock gets a block with the given reference.
// The ref should not be modified or retained by GetBlock.
// Returns data, found, error.
// Returns nil, false, nil if not found.
// Note: the block may not be in the specified bucket.
func (o *Overlay) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	cacheMode := func(lower, upper Store) ([]byte, bool, error) {
		data, found, err := o.upper.GetBlock(ctx, ref)
		if err != nil || found {
			return data, found, err
		}
		data, found, err = o.lower.GetBlock(ctx, ref)
		if err != nil || !found {
			return data, found, err
		}
		putOpts := &block.PutOpts{
			ForceBlockRef: ref.Clone(),
		}
		go func() {
			// writeback
			writebackCtx, writebackCtxCancel := context.WithTimeout(context.Background(), writebackTimeout)
			_, _, _ = o.upper.PutBlock(writebackCtx, data, putOpts)
			writebackCtxCancel()
		}()
		return data, true, nil
	}
	switch o.mode {
	default:
		fallthrough
	case BlockStoreMode_BlockStoreMode_DIRECT:
		return o.upper.GetBlock(ctx, ref)
	case BlockStoreMode_BlockStoreMode_CACHE:
		return cacheMode(o.lower, o.upper)
	case BlockStoreMode_BlockStoreMode_CACHE_LOWER:
		return cacheMode(o.upper, o.lower)
	}
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (o *Overlay) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	cacheMode := func(lower, upper Store) (bool, error) {
		found, err := o.upper.GetBlockExists(ctx, ref)
		if err != nil || found {
			return found, err
		}
		return o.lower.GetBlockExists(ctx, ref)
	}
	switch o.mode {
	default:
		fallthrough
	case BlockStoreMode_BlockStoreMode_DIRECT:
		return o.upper.GetBlockExists(ctx, ref)
	case BlockStoreMode_BlockStoreMode_CACHE:
		return cacheMode(o.lower, o.upper)
	case BlockStoreMode_BlockStoreMode_CACHE_LOWER:
		return cacheMode(o.upper, o.lower)
	}
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (o *Overlay) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := o.upper.RmBlock(ctx, ref); err != nil {
		return err
	}
	return o.lower.RmBlock(ctx, ref)
}

// _ is a type assertion
var _ Store = ((*Overlay)(nil))
