package block_store_kvfile

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
)

// KvfileBlock is a read-only block store on top of a kvfile.
type KvfileBlock struct {
	ctx   context.Context
	kvkey *store_kvkey.KVKey
	store *kvfile.Reader
}

// NewKvfileBlock constructs a new block store on top of a kvtx store.
//
// hashType can be 0 to use a default value.
func NewKvfileBlock(ctx context.Context, kvkey *store_kvkey.KVKey, store *kvfile.Reader) *KvfileBlock {
	return &KvfileBlock{ctx: ctx, kvkey: kvkey, store: store}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (k *KvfileBlock) GetHashType() hash.HashType {
	return 0
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KvfileBlock) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	return nil, false, block_store.ErrReadOnlyStore
}

// GetBlock looks up a block in the store.
// Returns data, found, and any unexpected error.
func (k *KvfileBlock) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	return k.store.Get(key)
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any unexpected error.
func (k *KvfileBlock) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)

	return k.store.Exists(key)
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KvfileBlock) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return block_store.ErrReadOnlyStore
}

// _ is a type assertion
var _ block_store.Store = ((*KvfileBlock)(nil))
