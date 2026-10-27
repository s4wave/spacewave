package volume

import "errors"

var (
	// ErrVolumeIDEmpty is returned if the volume id was empty.
	ErrVolumeIDEmpty = errors.New("volume id cannot be empty")
	// ErrBucketNotInVolume is returned if the volume does not contain the bucket.
	ErrBucketNotInVolume = errors.New("bucket does not exist in volume")
	// ErrObjectStoreUnavailable is returned when the object store is not available.
	ErrObjectStoreUnavailable = errors.New("object store is unavailable")
	// ErrReserve is returned when a volume refuses a new write because the
	// disk free to it is below its reserve. Reads, collection and deletes
	// continue, and writes resume once collection frees space.
	ErrReserve = errors.New("volume is below its disk reserve")
)
