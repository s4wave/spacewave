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
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *kvtxStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	mode := idb.TransactionReadOnly
	if write {
		mode = idb.TransactionReadWrite
	}
	txn, err := s.db.Transaction(mode, kvStoreObjectStore)
	if err != nil {
		return nil, err
	}
	return newKvtxTx(txn)
}

// _ is a type assertion
var _ kvtx.Store = ((*kvtxStore)(nil))
