//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/hydra/kvtx"
)

// kvtxStore implements the underlying kvtx store.
type kvtxStore struct {
	// db is the database
	db *idb.Database
}

func newKvtxStore(db *idb.Database) *kvtxStore {
	return &kvtxStore{db: db}
}

// NewTransaction returns a new transaction against the store.
func (s *kvtxStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return newKvtxTx(s.db, write), nil
}

// _ is a type assertion
var _ kvtx.Store = ((*kvtxStore)(nil))
