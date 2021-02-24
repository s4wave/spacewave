package block

import "errors"

var (
	// ErrBucketUnavailable is returned when Fetch is called against a nil bucket.
	ErrBucketUnavailable = errors.New("bucket is not set or is unavailable")
	// ErrUnexpectedType is returned if a type assertion failed.
	ErrUnexpectedType = errors.New("block: unexpected object type")
)
