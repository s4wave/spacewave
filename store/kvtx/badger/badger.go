package store_kvtx_badger

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	bdb "github.com/dgraph-io/badger/v2"
)

// Store is a badger database key-value store.
type Store struct {
	db       *bdb.DB
	writeMtx sync.Mutex
}

// NewStore constructs a new key-value store from a badger db.
func NewStore(db *bdb.DB) *Store {
	return &Store{db: db}
}

// Open opens a badger database store.
func Open(opts bdb.Options) (*Store, error) {
	b, err := bdb.Open(opts)
	if err != nil {
		return nil, err
	}

	return NewStore(b), nil
}

// GetDB returns the badger DB.
func (s *Store) GetDB() *bdb.DB {
	return s.db
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
//
// TODO: Badger allows concurrent writes but returns ErrConflict.
// Our application code is not ErrConflict aware, and in many cases
// expects a single holder for a write transaction at a time.
func (s *Store) NewTransaction(write bool) (kvtx.Tx, error) {
	if write {
		s.writeMtx.Lock()
	}
	txn := s.db.NewTransaction(write)
	return s.newTx(txn, write), nil
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	// Execute the value log gc.
	// TODO: customize this with more badger opts
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	again:
		if err := s.db.RunValueLogGC(0.7); err == nil {
			goto again
		}
	}
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
