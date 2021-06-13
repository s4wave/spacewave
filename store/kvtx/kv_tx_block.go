package store_kvtx

import (
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
)

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
func (k *KVTx) PutBlock(data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return k.blk.PutBlock(data, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (k *KVTx) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	return k.blk.GetBlock(ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (k *KVTx) GetBlockExists(ref *block.BlockRef) (bool, error) {
	return k.blk.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (k *KVTx) RmBlock(ref *block.BlockRef) error {
	return k.blk.RmBlock(ref)
}

// _ is a type assertion
var _ block_store.Store = ((*KVTx)(nil))
