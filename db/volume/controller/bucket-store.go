package volume_controller

import (
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// NewBucketStore returns the bucket handle of conf's bucket on v without a
// running controller, for a caller that opens a Volume directly, such as a
// command that opens a World file. Its roots, pins and stages are the
// bucket's on v, as a controller's handle's are, so a World opened through it
// and one opened through a controller hold one head.
func NewBucketStore(v volume.Volume, conf *bucket.Config) bucket.Bucket {
	return &bucketHandle{
		t:          &bucketHandleTracker{bucketID: conf.GetId()},
		v:          v,
		bucketConf: conf,
	}
}
