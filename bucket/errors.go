package bucket

import "errors"

var (
	// ErrBucketIdEmpty is returned if the bucket id was empty.
	ErrBucketIdEmpty = errors.New("bucket id must be specified")
	// ErrBucketUnavailable is returned when a bucket was not found.
	ErrBucketUnavailable = errors.New("bucket is unavailable")
)
