//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/paralin/go-indexeddb"
)

// kvtxStore implements the underlying kvtx store.
type kvtxStore struct {
	// db is the database
	db *indexeddb.Database
}

func newKvtxStore(db *indexeddb.Database) *kvtxStore {
	return &kvtxStore{db: db}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *kvtxStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	mode := indexeddb.READONLY
	if write {
		mode = indexeddb.READWRITE
	}
	txn, err := indexeddb.NewDurableTransaction(s.db, []string{kvStoreObjectStore}, mode)
	if err != nil {
		return nil, err
	}
	return newKvtxTx(txn)
}

// _ is a type assertion
var _ kvtx.Store = ((*kvtxStore)(nil))
