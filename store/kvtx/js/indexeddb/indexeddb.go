// +build js

package store_kvtx_indexeddb

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_txcache "github.com/aperturerobotics/hydra/kvtx/txcache"
	"github.com/paralin/go-indexeddb"
)

// Note that commit() doesn't normally have to be called — a transaction
// will automatically commit when all outstanding requests have been
// satisfied and no new requests have been made. commit() can be used to
// start the commit process without waiting for events from outstanding
// requests to be dispatched.
//
// Lots of code expects to be able to Discard() and cancel the transaction.
//
// this is wrapped with kvtx_txcache to fix this.

// dbSchemaVersion is the schema version.
// increment whenever changing the schema.
const dbSchemaVersion = 1

var (
	// kvStoreObjectStore is the key/value flat namespace store.
	kvStoreObjectStore = "kvstore"
)

// Store is a indexeddb key-value store.
type Store struct {
	kvtx.Store
	// db is the database
	db *indexeddb.Database
}

// NewStore constructs a new key-value store from a IndexedDB reference.
func NewStore(db *indexeddb.Database) *Store {
	st := newKvtxStore(db)
	return &Store{
		Store: kvtx_txcache.NewStore(st),
		db:    db,
	}
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
		return nil, errors.New("indexeddb not available")
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

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
