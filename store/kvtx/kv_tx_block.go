package store_kvtx

import (
	"time"

	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/cid"
)

// PutBlock puts a block into the store.
// Stores should check if the block already exists if possible.
func (k *KVTx) PutBlock(ref *cid.BlockRef, data []byte) (exists bool, err error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	key := k.kvkey.GetBlockKey(rm)
	tx, err := k.store.NewTransaction(true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	if exists, _ := tx.Exists(key); exists {
		return true, nil
	}

	if err := tx.Set(key, data, time.Duration(0)); err != nil {
		return false, err
	}

	return false, tx.Commit(k.ctx)
}

// GetBlock looks up a block in the store.
// Returns data, found, and any exceptional error.
func (k *KVTx) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
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
func (k *KVTx) GetBlockExists(ref *cid.BlockRef) (bool, error) {
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
func (k *KVTx) RmBlock(ref *cid.BlockRef) error {
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
