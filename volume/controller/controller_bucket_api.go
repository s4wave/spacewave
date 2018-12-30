package volume_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
)

// BuildBucketAPI builds an API handle for the bucket ID in the volume.
// If the bucket is not found, should monitor in case it is created.
// The handles are valid while ctx is valid.
func (c *Controller) BuildBucketAPI(
	ctx context.Context,
	bucketID string,
	cb func(b bucket.Bucket, added bool),
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var h *bucketHandle
		c.bucketMtx.Lock()
		h = c.bucketHandles[bucketID]
		c.bucketMtx.Unlock()
		if h == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case vb := <-c.volumeCh:
				c.volumeCh <- vb
				bc, err := vb.vol.GetLatestBucketConfig(bucketID)
				if err != nil {
					return err
				}
				h = newBucketHandle(vb.ctx, c, vb.vol, bc)
			}
		}
		c.bucketMtx.Lock()
		if nh, ok := c.bucketHandles[bucketID]; ok {
			if h.superceeds(nh) {
				nh.Flush()
				nh = nil
				c.bucketHandles[bucketID] = h
			} else {
				h.Flush()
				h = nh
			}
		} else {
			c.bucketHandles[bucketID] = h
		}
		defer h.startOperation().release()
		c.bucketMtx.Unlock()
		pt := h.bucketConf != nil
		if pt {
			cb(h, true)
			go func() {
				select {
				case <-ctx.Done():
				case <-h.ctx.Done():
					cb(h, false)
				}
			}()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.ctx.Done():
			if pt {
				cb(h, false)
			}
		}
	}
}
