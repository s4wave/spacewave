package volume

import "errors"

var (
	// ErrVolumeIDEmpty is returned if the volume id was empty.
	ErrVolumeIDEmpty = errors.New("volume id cannot be empty")
	// ErrBucketNotInVolume is returned if the volume does not contain the bucket.
	ErrBucketNotInVolume = errors.New("bucket does not exist in volume")
	// ErrObjectStoreUnavailable is returned when the object store is not available.
	ErrObjectStoreUnavailable = errors.New("object store is unavailable")
)
