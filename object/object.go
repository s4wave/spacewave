package object

import (
	"errors"
)

var (
	// ErrObjectStoreClosed is returned if the store is closed.
	ErrObjectStoreClosed = errors.New("object store is closed")
)

// ObjectStore implements a key/value object store.
// Calls may return ErrObjectStoreClosed or context.Canceled if the store is closed.
type ObjectStore interface {
	// Get gets an object by key.
	GetObject(key string) (val []byte, found bool, err error)
	// Set sets an object by key.
	SetObject(key string, val []byte) error
	// DeleteObject deletes an object by key.
	DeleteObject(key string) error
	// ListKeys lists keys with a given prefix.
	ListKeys(prefix string) ([]string, error)
}
