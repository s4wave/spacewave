package bucket

import "errors"

var (
	// ErrBucketIdEmpty is returned if the bucket id was empty.
	ErrBucketIdEmpty = errors.New("bucket id must be specified")
)
