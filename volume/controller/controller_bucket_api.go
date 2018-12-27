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
	// TODO
	return nil
}
