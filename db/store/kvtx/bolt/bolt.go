//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"os"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Store is a bolt database key-value store.
type Store struct {
	db     *bdb.DB
	bucket []byte
}

// NewStore constructs a new key-value store from a bolt db.
func NewStore(db *bdb.DB, bucket []byte) *Store {
	return &Store{db: db, bucket: bucket}
}

// Open opens a bolt database store.
func Open(path string, mode os.FileMode, options *bdb.Options, bucket []byte) (*Store, error) {
	if len(bucket) == 0 {
		return nil, errors.New("bucket len cannot be zero")
	}

	b, err := bdb.Open(path, mode, options)
	if err != nil {
		return nil, err
	}

	return NewStore(b, bucket), nil
}

// GetDB returns the bolt DB.
func (s *Store) GetDB() *bdb.DB {
	return s.db
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	txn, err := s.db.Begin(write)
	if err != nil {
		return nil, err
	}
	return NewTx(txn, s.bucket), nil
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
