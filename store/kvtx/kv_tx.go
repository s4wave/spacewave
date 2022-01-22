package store_kvtx

import (
	"context"

	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
	hstore "github.com/aperturerobotics/hydra/store"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
)

// KVTx wraps a key/value transaction store and implements the Hydra store.
type KVTx struct {
	ctx   context.Context
	kvkey *store_kvkey.KVKey
	blk   *block_store_kvtx.KVTxBlock
	conf  *Config
	// store may also be a Store
	store   kvtx.Store
	storeID string
}

// NewKVTx constructs a new KVTx store.
//
// store can optionally be a store_kvtx.Store with execute func.
func NewKVTx(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *Config,
) hstore.Store {
	return &KVTx{
		ctx:   ctx,
		conf:  conf,
		kvkey: kvkey,
		blk: block_store_kvtx.NewKVTxBlock(
			ctx,
			kvkey,
			store,
		),
		store:   store,
		storeID: storeID,
	}
}

// GetStoreID returns the store id.
func (k *KVTx) GetStoreID() string {
	return k.storeID
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (k *KVTx) Execute(ctx context.Context) error {
	if kstore, kstoreOk := k.store.(Store); kstoreOk {
		return kstore.Execute(ctx)
	}
	return nil
}

// _ is a type assertion
var _ hstore.Store = ((*KVTx)(nil))
