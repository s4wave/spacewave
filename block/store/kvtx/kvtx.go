package block_store_kvtx

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/kvtx"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
)

// KVTxBlock is a block store on top of a kvtx.
type KVTxBlock struct {
	ctx      context.Context
	kvkey    *store_kvkey.KVKey
	store    kvtx.Store
	hashType hash.HashType
}

// NewKVTxBlock constructs a new block store on top of a kvtx store.
//
// hashType can be 0 to use a default value.
func NewKVTxBlock(ctx context.Context, kvkey *store_kvkey.KVKey, store kvtx.Store, hashType hash.HashType) *KVTxBlock {
	return &KVTxBlock{ctx: ctx, kvkey: kvkey, store: store, hashType: hashType}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (k *KVTxBlock) GetHashType() hash.HashType {
	return k.hashType
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTxBlock) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	if opts == nil {
		opts = &block.PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(k.hashType)

	ref, err = block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceBlockRef := opts.GetForceBlockRef(); !forceBlockRef.GetEmpty() {
		if !ref.EqualsRef(forceBlockRef) {
			return ref, false, block.ErrBlockRefMismatch
		}
	}

	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(ctx, true)
	if err != nil {
		return ref, false, err
	}
	defer tx.Discard()

	if exists, _ := tx.Exists(ctx, key); exists {
		return ref, true, nil
	}

	// many stores cannot handle empty values
	// add a blanket check here to be sure
	if len(data) == 0 {
		return ref, false, block.ErrEmptyBlock
	}

	if err := tx.Set(ctx, key, data); err != nil {
		return ref, false, err
	}

	return ref, false, tx.Commit(k.ctx)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (k *KVTxBlock) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()

	return tx.Get(ctx, key)
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any exceptional error.
func (k *KVTxBlock) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	return tx.Exists(ctx, key)
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KVTxBlock) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	rm, err := ref.MarshalKey()
	if err != nil {
		return err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if err := tx.Delete(ctx, key); err != nil {
		return err
	}

	return tx.Commit(k.ctx)
}

// _ is a type assertion
var _ block_store.Store = ((*KVTxBlock)(nil))
