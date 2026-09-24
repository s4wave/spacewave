package block_store_s3

import "github.com/pkg/errors"

var (
	// ErrNotFound is returned when an object does not exist on the server.
	ErrNotFound = errors.New("not found")
	// ErrBucketNotFound is returned when the bucket does not exist.
	ErrBucketNotFound = errors.New("bucket not found")
)
