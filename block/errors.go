package block

import "errors"

var (
	// ErrBucketUnavailable is returned when Fetch is called against a nil bucket.
	ErrBucketUnavailable = errors.New("bucket is not set or is unavailable")
)
