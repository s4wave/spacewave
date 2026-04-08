package store_kvtx

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
	hstore "github.com/aperturerobotics/hydra/store"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
)

// KVTx wraps a key/value transaction store and implements the Hydra store.
type KVTx struct {
	kvkey *store_kvkey.KVKey
	blk   block.StoreOps
	conf  *Config
	// store may also be a store_kvtx.Store
	store kvtx.Store
}

// NewKVTx constructs a new KVTx store.
//
// store can optionally be a store_kvtx.Store with execute func.
func NewKVTx(
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *Config,
) hstore.Store {
	return &KVTx{
		conf:  conf,
		kvkey: kvkey,
		blk: block_store_kvtx.NewKVTxBlock(
			kvkey,
			store,
			conf.GetHashType(),
			!conf.GetDisableHashGet(),
		),
		store: store,
	}
}

// NewKVTxWithBlockStore constructs a KVTx with a custom block store.
//
// blk is used for block operations instead of creating a KVTxBlock.
// store can optionally be a store_kvtx.Store with execute func.
func NewKVTxWithBlockStore(
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	blk block.StoreOps,
	conf *Config,
) hstore.Store {
	return &KVTx{
		conf:  conf,
		kvkey: kvkey,
		blk:   blk,
		store: store,
	}
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
