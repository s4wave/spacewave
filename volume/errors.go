package volume

import "errors"

var (
	// ErrVolumeIDEmpty is returned if the volume id was empty.
	ErrVolumeIDEmpty = errors.New("volume id cannot be empty")
	// ErrBucketIDEmpty is returned if the bucket id was empty.
	ErrBucketIDEmpty = errors.New("bucket id cannot be empty")
)
