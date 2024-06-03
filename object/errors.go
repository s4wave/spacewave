package object

import (
	"errors"
)

var (
	// ErrObjectStoreClosed is returned if the store is closed.
	ErrObjectStoreClosed = errors.New("object store is closed")
	// ErrEmptyObjectStoreId is returned if the object store id was empty.
	ErrEmptyObjectStoreId = errors.New("object store id is empty")
	// ErrEmptyObjectStoreKey is returned if the object store key was empty.
	ErrEmptyObjectStoreKey = errors.New("object store key is empty")
)
