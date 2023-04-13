package bucket

import "errors"

var (
	// ErrBucketIDEmpty is returned if the bucket id was empty.
	ErrBucketIDEmpty = errors.New("bucket id cannot be empty")
	// ErrBucketUnknown is returned when the bucket was not found.
	ErrBucketUnknown = errors.New("bucket not found")
)
