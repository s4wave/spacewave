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
	// Retain the bucket tracker while its API handle is in use.
	ref, ht, _ := c.bucketHandles.AddKeyRef(bucketID)

	// Wait for the bucket tracker to publish its handle.
	h, err := ht.wait(ctx)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}

	return h, ref.Release, nil
}
