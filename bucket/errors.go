package bucket

import "errors"

var (
	// ErrBucketIDEmpty is returned if the bucket id was empty.
	ErrBucketIDEmpty = errors.New("bucket id cannot be empty")
	// ErrRevEmpty is returned if the revision was empty.
	ErrRevEmpty = errors.New("bucket rev cannot be empty")
	// ErrBucketUnknown is returned when the bucket was not found.
	ErrBucketUnknown = errors.New("bucket not found")
)
