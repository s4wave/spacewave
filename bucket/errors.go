package bucket

import "errors"

var (
	// ErrBucketIDEmpty is returned if the bucket id was empty.
	ErrBucketIDEmpty = errors.New("bucket id cannot be empty")
	// ErrStoreIDEmpty is returned if the bucket store id was empty.
	ErrStoreIDEmpty = errors.New("bucket store id cannot be empty")
	// ErrRevEmpty is returned if the revision was empty.
	ErrRevEmpty = errors.New("bucket rev cannot be empty")
	// ErrBucketNotFound is returned when the bucket was not found.
	ErrBucketNotFound = errors.New("bucket not found")
)
