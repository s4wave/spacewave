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
	// ErrVolumeClosed is returned by a write to a Volume that closed.
	ErrVolumeClosed = errors.New("volume is closed")
	// ErrReadOnlyUpgrade is returned by a read-only open of a volume whose
	// storage needs a conversion or a format upgrade, which only a writable
	// open makes.
	ErrReadOnlyUpgrade = errors.New("volume needs a format upgrade, which a read-only open cannot write")
	// ErrFormatUpgradeRefused is returned by a writable open of a volume
	// whose data is at an older format when its configuration refuses the
	// upgrade, as the spacewave development CLI does unless the operator
	// opts in.
	ErrFormatUpgradeRefused = errors.New("volume needs a format upgrade, which this build makes only with --upgrade-format or SPACEWAVE_UPGRADE_FORMAT=1")
)
