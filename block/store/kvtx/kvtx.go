package block_store_kvtx

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/kvtx"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
)

// KVTxBlock is a block store on top of a kvtx.
type KVTxBlock struct {
	ctx   context.Context
	kvkey *store_kvkey.KVKey
	store kvtx.Store
}

// NewKVTxBlock constructs a new block store on top of a kvtx store.
func NewKVTxBlock(ctx context.Context, kvkey *store_kvkey.KVKey, store kvtx.Store) *KVTxBlock {
	return &KVTxBlock{ctx: ctx, kvkey: kvkey, store: store}
}

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTxBlock) PutBlock(data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
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
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return ref, false, err
	}
	defer tx.Discard()

	if exists, _ := tx.Exists(key); exists {
		return ref, true, nil
	}

	if err := tx.Set(key, data); err != nil {
		return ref, false, err
	}

	return ref, false, tx.Commit(k.ctx)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (k *KVTxBlock) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()

	return tx.Get(key)
}

// GetBlockExists checks if a block exists in the store.
// Returns found, and any exceptional error.
func (k *KVTxBlock) GetBlockExists(ref *block.BlockRef) (bool, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	return tx.Exists(key)
}

// RmBlock deletes a block from the store.
// Should not return an error if the block did not exist.
func (k *KVTxBlock) RmBlock(ref *block.BlockRef) error {
	rm, err := ref.MarshalKey()
	if err != nil {
		return err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if err := tx.Delete(key); err != nil {
		return err
	}

	return tx.Commit(k.ctx)
}

// _ is a type assertion
var _ block_store.Store = ((*KVTxBlock)(nil))
