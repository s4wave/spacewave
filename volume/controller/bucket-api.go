package volume_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/volume"
)

// attachedBucketHandle is an attached bucket handle satisfying a directive.
type attachedBucketHandle struct {
	*bucketHandle
	closeOnce sync.Once
	ctx       context.Context
	ctxCancel context.CancelFunc
}

// newAttachedBucketHandle builds a new attached bucket handle.
func newAttachedBucketHandle(ctx context.Context, bh *bucketHandle) *attachedBucketHandle {
	h := &attachedBucketHandle{bucketHandle: bh}
	h.ctx, h.ctxCancel = context.WithCancel(ctx)
	return h
}

// GetContext returns the bucket handle context.
func (h *attachedBucketHandle) GetContext() context.Context {
	return h.bucketHandle.ctx
}

// BuildBucketAPI builds an API handle for the bucket ID in the volume.
// The handles are valid while ctx is valid.
func (c *Controller) BuildBucketAPI(
	ctx context.Context,
	bucketID string,
) (volume.BucketHandle, error) {
	var h *bucketHandle
	c.bucketMtx.Lock()
	h = c.bucketHandles[bucketID]
	if h != nil {
		select {
		case <-h.ctx.Done():
			h = nil
			delete(c.bucketHandles, bucketID)
		default:
		}
	}
	c.bucketMtx.Unlock()

	if h == nil {
		vol, err := c.GetVolume(ctx)
		if err != nil {
			return nil, err
		}

		bc, err := vol.GetBucketConfig(bucketID)
		if err != nil {
			return nil, err
		}
		h = newBucketHandle(ctx, c, vol, bc)

		c.bucketMtx.Lock()
		if nh, ok := c.bucketHandles[bucketID]; ok {
			if h.superceeds(nh) {
				nh.ctxCancel()
				nh = nil
				c.bucketHandles[bucketID] = h
			} else {
				h.ctxCancel()
				h = nh
			}
		} else {
			c.bucketHandles[bucketID] = h
		}
		c.bucketMtx.Unlock()
	}

	return newAttachedBucketHandle(ctx, h), nil
}

// Close closes the bucket handle.
// May be called many times.
// Does not block.
func (h *attachedBucketHandle) Close() {
	h.closeOnce.Do(func() {
		h.ctxCancel()
	})
}

// _ is a type assertion
var _ volume.BucketHandle = ((*attachedBucketHandle)(nil))
