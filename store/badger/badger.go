package badger

import (
	"context"
	"io"
	"io/ioutil"
	"time"

	"github.com/aperturerobotics/hydra/store"
	"github.com/dgraph-io/badger"
)

// Store is a badger database key-value store.
type Store struct {
	db *badger.DB
}

// NewStore constructs a new key-value store from a badger db.
func NewStore(db *badger.DB) *Store {
	return &Store{db: db}
}

// Open opens a badger database store.
func Open(opts badger.Options) (*Store, error) {
	b, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return NewStore(b), nil
}

// GetDB returns the badger DB.
func (s *Store) GetDB() *badger.DB {
	return s.db
}

// Get looks up a key and returns the value.
// Returns value, if the key was found, and any error.
func (s *Store) Get(ctx context.Context, key []byte) (
	val io.ReadCloser,
	found bool,
	err error,
) {
	err = s.db.View(func(txn *badger.Txn) error {
		item, rerr := txn.Get(key)
		if rerr != nil {
			if rerr == badger.ErrKeyNotFound {
				// val == nil, found = false
				return nil
			}
			return rerr
		}

		found = true
		return item.Value(func(valb []byte) error {
			vb := &closableBuffer{}
			val = vb
			_, rerr := vb.Write(valb)
			return rerr
		})
	})
	return val, found, err
}

// Set sets a key to a value.
// If context is canceled, terminate the call.
func (s *Store) Set(
	ctx context.Context,
	key []byte,
	value io.Reader,
	ttl time.Duration,
) (err error) {
	valb, err := ioutil.ReadAll(value)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		if ttl == time.Duration(0) {
			return txn.Set(key, valb)
		} else {
			return txn.SetWithTTL(key, valb, ttl)
		}
	})
}

// _ is a type assertion
var _ store.KV = ((*Store)(nil))
