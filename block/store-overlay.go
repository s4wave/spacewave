package block

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
)

// StoreOverlay layers an upper block store over a lower store.
//
// ctx is used for writeback requests
type StoreOverlay struct {
	ctx              context.Context
	lower, upper     StoreOps
	mode             OverlayMode
	writebackTimeout time.Duration
	writebackPutOpts *PutOpts
}

// NewOverlay constructs a new overlay store.
//
// ctx is used for writeback requests
func NewOverlay(
	ctx context.Context,
	lower,
	upper StoreOps,
	mode OverlayMode,
	writebackTimeout time.Duration,
	writebackPutOpts *PutOpts,
) *StoreOverlay {
	return &StoreOverlay{
		ctx:   ctx,
		lower: lower,
		upper: upper,
		mode:  mode,

		writebackTimeout: writebackTimeout,
		writebackPutOpts: writebackPutOpts,
	}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (o *StoreOverlay) GetHashType() hash.HashType {
	if v := o.upper.GetHashType(); v != 0 {
		return v
	}
	return o.lower.GetHashType()
}

// GetBlock gets a block with the given reference.
// The ref should not be modified or retained by GetBlock.
// Returns data, found, error.
// Returns nil, false, nil if not found.
// Note: the block may not be in the specified bucket.
func (o *StoreOverlay) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	cacheMode := func(s1, s2 StoreOps, writeBack StoreOps) ([]byte, bool, error) {
		// Try to get the block from the first store (s1)
		data, found, err := s1.GetBlock(ctx, ref)
		if err != nil || found {
			return data, found, err
		}

		// If not found in s1, try to get it from the second store (s2)
		data, found, err = s2.GetBlock(ctx, ref)
		if err != nil || !found {
			return data, found, err
		}

		// If found in s2 and writeback is enabled, write the block back to s1
		if writeBack != nil && o.ctx.Err() == nil {
			var writebackCtx context.Context
			var writebackCtxCancel context.CancelFunc
			if o.writebackTimeout > 0 {
				writebackCtx, writebackCtxCancel = context.WithTimeout(o.ctx, o.writebackTimeout)
			} else {
				writebackCtx, writebackCtxCancel = context.WithCancel(o.ctx)
			}

			go func() {
				// Prepare writeback options
				putOpts := o.writebackPutOpts.CloneVT()
				if putOpts == nil {
					putOpts = &PutOpts{}
				}
				putOpts.ForceBlockRef = ref.Clone()

				// Perform writeback asynchronously
				_, _, _ = writeBack.PutBlock(writebackCtx, data, putOpts)
				writebackCtxCancel()
			}()
		}
		return data, true, nil
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// reads go to the upper store only.
		return o.upper.GetBlock(ctx, ref)
	case OverlayMode_LOWER_ONLY:
		// reads go to the lower store only.
		return o.lower.GetBlock(ctx, ref)
	case OverlayMode_UPPER_CACHE:
		// reads go to the upper store first, then the lower store.
		// reads from lower are written back to upper.
		return cacheMode(o.upper, o.lower, o.upper)
	case OverlayMode_LOWER_CACHE:
		// reads go to the lower store first, then the upper store.
		// reads from upper are written back to lower.
		return cacheMode(o.lower, o.upper, o.lower)
	case OverlayMode_UPPER_READ_CACHE:
		// reads go to the upper store first, then the lower store.
		// reads from lower are not written back to upper.
		return cacheMode(o.upper, o.lower, nil)
	case OverlayMode_LOWER_READ_CACHE:
		// reads go to the lower store first, then the upper store.
		// reads from upper are not written back to lower.
		return cacheMode(o.lower, o.upper, nil)
	case OverlayMode_UPPER_WRITE_CACHE:
		// reads go to the upper store first, then the lower store.
		// reads from lower are not written back to upper.
		return cacheMode(o.upper, o.lower, nil)
	case OverlayMode_LOWER_WRITE_CACHE:
		// reads go to the lower store first, then the upper store.
		// reads from upper are not written back to lower.
		return cacheMode(o.lower, o.upper, nil)
	}
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (o *StoreOverlay) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	cacheMode := func(primary, secondary StoreOps) (bool, error) {
		found, err := primary.GetBlockExists(ctx, ref)
		if err != nil || found {
			return found, err
		}
		return secondary.GetBlockExists(ctx, ref)
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// reads go to the upper store only.
		return o.upper.GetBlockExists(ctx, ref)
	case OverlayMode_LOWER_ONLY:
		// reads go to the lower store only.
		return o.lower.GetBlockExists(ctx, ref)
	case OverlayMode_UPPER_CACHE:
		// reads go to the upper store first, then the lower store.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_CACHE:
		// reads go to the lower store first, then the upper store.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_UPPER_READ_CACHE:
		// reads go to the upper store first, then the lower store.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_READ_CACHE:
		// reads go to the lower store first, then the upper store.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_WRITE_CACHE:
		// reads go to the lower store first, then the upper store.
		return cacheMode(o.lower, o.upper)
	}
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (o *StoreOverlay) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	cacheMode := func(primary, secondary StoreOps) (*BlockStat, error) {
		stat, err := primary.StatBlock(ctx, ref)
		if err != nil || stat != nil {
			return stat, err
		}
		return secondary.StatBlock(ctx, ref)
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		return o.upper.StatBlock(ctx, ref)
	case OverlayMode_LOWER_ONLY:
		return o.lower.StatBlock(ctx, ref)
	case OverlayMode_UPPER_CACHE:
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_CACHE:
		return cacheMode(o.lower, o.upper)
	case OverlayMode_UPPER_READ_CACHE:
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_READ_CACHE:
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_WRITE_CACHE:
		return cacheMode(o.lower, o.upper)
	}
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
// If the hash type is unset, use the type from GetHashType().
func (o *StoreOverlay) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	cacheMode := func(s1, s2 StoreOps) (*BlockRef, bool, error) {
		ref, existed, err := s1.PutBlock(ctx, data, opts)
		if err != nil {
			return nil, false, err
		}
		lowerOpts := opts.CloneVT()
		lowerOpts.ForceBlockRef = ref
		_, lowerExisted, err := s2.PutBlock(ctx, data, lowerOpts)
		if err != nil {
			return nil, false, err
		}
		return ref, existed && lowerExisted, nil
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// writes go to the upper store only.
		return o.upper.PutBlock(ctx, data, opts)
	case OverlayMode_LOWER_ONLY:
		// writes go to the lower store only.
		return o.lower.PutBlock(ctx, data, opts)
	case OverlayMode_UPPER_CACHE:
		// writes go to both stores.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_CACHE:
		// writes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_UPPER_READ_CACHE:
		// writes go to the lower store only.
		return o.lower.PutBlock(ctx, data, opts)
	case OverlayMode_LOWER_READ_CACHE:
		// writes go to the upper store only.
		return o.upper.PutBlock(ctx, data, opts)
	case OverlayMode_UPPER_WRITE_CACHE:
		// writes go to the upper store only.
		return o.upper.PutBlock(ctx, data, opts)
	case OverlayMode_LOWER_WRITE_CACHE:
		// writes go to the lower store only.
		return o.lower.PutBlock(ctx, data, opts)
	}
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (o *StoreOverlay) RmBlock(ctx context.Context, ref *BlockRef) error {
	cacheMode := func(primary, secondary StoreOps) error {
		uerr := primary.RmBlock(ctx, ref)
		lerr := secondary.RmBlock(ctx, ref)
		if uerr != nil {
			return uerr
		}
		return lerr
	}

	switch o.mode {
	default:
		fallthrough
	case OverlayMode_UPPER_ONLY:
		// removes go to the upper store only.
		return o.upper.RmBlock(ctx, ref)
	case OverlayMode_LOWER_ONLY:
		// removes go to the lower store only.
		return o.lower.RmBlock(ctx, ref)
	case OverlayMode_UPPER_CACHE:
		// removes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_CACHE:
		// removes go to both stores.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_UPPER_READ_CACHE:
		// removes go to both stores.
		return cacheMode(o.lower, o.upper)
	case OverlayMode_LOWER_READ_CACHE:
		// removes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_UPPER_WRITE_CACHE:
		// removes go to both stores.
		return cacheMode(o.upper, o.lower)
	case OverlayMode_LOWER_WRITE_CACHE:
		// removes go to both stores.
		return cacheMode(o.lower, o.upper)
	}
}

// _ is a type assertion
var _ StoreOps = ((*StoreOverlay)(nil))
