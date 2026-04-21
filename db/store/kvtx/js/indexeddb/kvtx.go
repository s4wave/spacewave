//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/s4wave/spacewave/db/kvtx"
)

// kvtxStore implements the underlying kvtx store.
type kvtxStore struct {
	// db is the database
	db *idb.Database
	// objectStoreName is the object store to use
	objectStoreName string
}

func newKvtxStore(db *idb.Database, objectStoreName string) *kvtxStore {
	return &kvtxStore{db: db, objectStoreName: objectStoreName}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *kvtxStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return newKvtxTx(s.db, write, s.objectStoreName)
}

// _ is a type assertion
var _ kvtx.Store = ((*kvtxStore)(nil))
