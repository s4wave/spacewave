package block_store

import (
	"github.com/aperturerobotics/hydra/cid"
)

// Store is a block store.
type Store interface {
	// PutBlock puts a block into the store.
	// Stores should check if the block already exists if possible.
	PutBlock(ref *cid.BlockRef, data []byte) (existed bool, err error)
	// GetBlock looks up a block in the store.
	// Returns data, found, and any exceptional error.
	GetBlock(ref *cid.BlockRef) ([]byte, bool, error)
	// RmBlock deletes a block from the store.
	// Should not return an error if the block did not exist.
	RmBlock(ref *cid.BlockRef) error
}
