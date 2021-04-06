package store_kvtx

import (
	"time"

	"github.com/aperturerobotics/hydra/block"
	block_store "github.com/aperturerobotics/hydra/block/store"
)

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTx) PutBlock(data []byte, opts *block.PutOpts) (ref *block.BlockRef, exists bool, err error) {
	ref, err = block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
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

	if err := tx.Set(key, data, time.Duration(0)); err != nil {
		return ref, false, err
	}

	return ref, false, tx.Commit(k.ctx)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (k *KVTx) GetBlock(ref *block.BlockRef) ([]byte, bool, error) {
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
func (k *KVTx) GetBlockExists(ref *block.BlockRef) (bool, error) {
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
func (k *KVTx) RmBlock(ref *block.BlockRef) error {
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
var _ block_store.Store = ((*KVTx)(nil))
