package volume

import "errors"

var (
	// ErrVolumeIDEmpty is returned if the volume id was empty.
	ErrVolumeIDEmpty = errors.New("volume id cannot be empty")
	// ErrBucketIDEmpty is returned if the bucket id was empty.
	ErrBucketIDEmpty = errors.New("bucket id cannot be empty")
	// ErrReconcilerQueuesDisabled is returned if reconciler queues are disabled for the volume.
	ErrReconcilerQueuesDisabled = errors.New("reconciler queues are disabled")
)
