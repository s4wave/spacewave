//go:build !tinygo

package block_store_s3

import (
	"context"
	"io"
)

// ObjectStore is the object storage a PackStore keeps its packfiles in: a
// flat namespace of immutable objects named by bucket and key.
//
// A written object is visible to readers only once complete. GetObject and
// GetObjectRange return an error matching ErrNotFound for a missing object.
// Deleting a missing object succeeds. *Client implements it over the S3 API.
type ObjectStore interface {
	// PutObject writes the object, replacing any object at the key.
	PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error
	// GetObject opens the object's body. The caller must Close it.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// GetObjectRange reads length bytes from offset off. The result is
	// shorter than length when the object ends first.
	GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error)
	// ListObjects calls fn with the key and size of every object whose key
	// starts with prefix.
	ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error
	// DeleteObject removes the object.
	DeleteObject(ctx context.Context, bucket, key string) error
}

// ObjectMover is an ObjectStore that moves an object to another key of the
// same bucket without passing its bytes through the client, as a pack sweep
// moves a pack to the trash and back.
type ObjectMover interface {
	ObjectStore
	// MoveObject moves the object at from to to. A move whose source is gone
	// and whose destination exists counts as done, so a repeated move
	// succeeds; with neither, it returns ErrNotFound.
	MoveObject(ctx context.Context, bucket, from, to string) error
}

// _ is a type assertion
var (
	_ ObjectStore = (*Client)(nil)
	_ ObjectMover = (*Client)(nil)
)
