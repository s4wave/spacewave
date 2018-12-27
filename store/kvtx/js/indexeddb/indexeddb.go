//+build js

package kvtx_indexeddb

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/store/kvtx"
	"github.com/paralin/go-indexeddb"
)

// dbSchemaVersion is the schema version.
// increment whenever changing the schema.
const dbSchemaVersion = 1

var (
	// kvStoreObjectStore is the key/value flat namespace store.
	kvStoreObjectStore = "kvstore"
)

// Store is a indexeddb key-value store.
type Store struct {
	// db is the database
	db *indexeddb.Database
}

// NewStore constructs a new key-value store from a IndexedDB reference.
func NewStore(db *indexeddb.Database) *Store {
	return &Store{db: db}
}

// schemaUpgrader is the upgrader function.
func schemaUpgrader(d *indexeddb.DatabaseUpdate, oldVersion int, newVersion int) error {
	if !d.ContainsObjectStore(kvStoreObjectStore) {
		if err := d.CreateObjectStore(kvStoreObjectStore, nil); err != nil {
			return err
		}
	}

	return nil
}

// Open opens a IndexedDB database, upgrading the schema.
func Open(ctx context.Context, name string) (*Store, error) {
	gidb := indexeddb.GlobalIndexedDB()
	if gidb == nil {
		return nil, errors.New("indexed db not available")
	}

	d, err := gidb.Open(ctx, name, dbSchemaVersion, schemaUpgrader)
	if err != nil {
		return nil, err
	}

	return NewStore(d), nil
}

// GetDB returns the IndexedDB database
func (s *Store) GetDB() *indexeddb.Database {
	return s.db
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(write bool) (kvtx.Tx, error) {
	mode := indexeddb.READONLY
	if write {
		mode = indexeddb.READWRITE
	}
	txn, err := s.db.Transaction([]string{kvStoreObjectStore}, mode)
	if err != nil {
		return nil, err
	}
	return NewTx(txn)
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
