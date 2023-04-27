package block_store

import "github.com/pkg/errors"

var (
	// ErrBlockStoreIDEmpty is returned if the block store id was empty.
	ErrBlockStoreIDEmpty = errors.New("block store id cannot be empty")
)
