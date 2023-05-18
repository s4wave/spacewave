package kvtx_txcache

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Store wraps a kvtx store with a txcache wrapper.
type Store struct {
	store kvtx.Store
}

// NewStore constructs a new txcache store.
func NewStore(store kvtx.Store) *Store {
	return &Store{store: store}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	t, err := NewTx(ctx, s, write)
	return t, err
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
