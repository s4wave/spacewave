//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"context"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_txcache "github.com/aperturerobotics/hydra/kvtx/txcache"
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

// Store is a indexeddb key-value store.
type Store struct {
	kvtx.Store
	// db is the database
	db *idb.Database
	// objectStoreName is the name of the object store to use
	objectStoreName string
}

// NewStore constructs a new key-value store from a IndexedDB reference.
func NewStore(db *idb.Database, objectStoreName string) *Store {
	st := newKvtxStore(db, objectStoreName)
	return &Store{
		Store:           kvtx_txcache.NewStore(st),
		db:              db,
		objectStoreName: objectStoreName,
	}
}

// Open opens an IndexedDB database, creating the schema if it doesn't exist.
//
// The object store name will be used for the kvtx functions.
func Open(ctx context.Context, name, objectStoreName string) (*Store, error) {
	openRequest, err := idb.Global().Open(ctx, name, dbSchemaVersion, func(db *idb.Database, oldVersion, newVersion uint) error {
		db.CreateObjectStore(objectStoreName, idb.ObjectStoreOptions{})
		return nil
	})
	if err != nil {
		return nil, err
	}

	db, err := openRequest.Await(ctx)
	if err != nil {
		return nil, err
	}

	return NewStore(db, objectStoreName), nil
}

// GetDB returns the IndexedDB database
func (s *Store) GetDB() *idb.Database {
	return s.db
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// Close closes the store db.
func (s *Store) Close() error {
	return s.db.Close()
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
