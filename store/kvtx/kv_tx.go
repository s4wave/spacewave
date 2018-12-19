package kvtx

import (
	"context"

	hstore "github.com/aperturerobotics/hydra/store"
	"github.com/aperturerobotics/hydra/store/kvkey"
)

// KVTx wraps a key/value transaction store and implements the Hydra store.
type KVTx struct {
	ctx   context.Context
	kvkey *store_kvkey.KVKey
	store Store
}

// NewKVTx constructs a new KVTx store.
func NewKVTx(
	ctx context.Context,
	kvkey *store_kvkey.KVKey,
	store Store,
) hstore.Store {
	return &KVTx{ctx: ctx, kvkey: kvkey, store: store}
}

// _ is a type assertion
var _ hstore.Store = ((*KVTx)(nil))
