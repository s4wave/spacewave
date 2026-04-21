package volume_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
)

// BuildBucketAPI builds an API handle for the bucket ID in the volume.
// The handles are valid while ctx is valid.
// Returns a release function.
func (c *Controller) BuildBucketAPI(
	ctx context.Context,
	bucketID string,
) (bucket.BucketHandle, func(), error) {
	ref, ht, _ := c.bucketHandles.AddKeyRef(bucketID)

	h, err := ht.handleCtr.WaitValue(ctx, nil)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}

	if h.err != nil {
		ref.Release()
		return nil, nil, err
	}

	return h, ref.Release, nil
}
