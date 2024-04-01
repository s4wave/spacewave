package block_store

import "github.com/pkg/errors"

var (
	// ErrBlockStoreIDEmpty is returned if the block store id was empty.
	ErrBlockStoreIDEmpty = errors.New("block store id cannot be empty")
	// ErrBlockStoreNotFound is returned if the block store was not found.
	ErrBlockStoreNotFound = errors.New("block store not found")
	// ErrReadOnly is returned if the block store is not writable.
	ErrReadOnly = errors.New("block store is read-only")
)
