package store_kvtx_inmem

import (
	"context"
	"sync"

	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/aperturerobotics/hydra/kvtx"
)

// Store is a in-memory key-value store.
// Primarily intended for mock/testing.
// Uses uint64 crc64 keys.
// Casts keys to strings.
type Store struct {
	// mtx guards ct
	mtx sync.Mutex
	// ct is the ctrie backing the store
	ct *ctrie.Ctrie
	// writeMtx guards write transactions
	writeMtx sync.Mutex
}

// NewStore constructs a new key-value store from a badger db.
func NewStore() *Store {
	return &Store{
		ct: ctrie.New(nil),
	}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(write bool) (kvtx.Tx, error) {
	s.mtx.Lock()
	c := s.ct
	s.mtx.Unlock()
	if write {
		s.writeMtx.Lock()
		c = c.Snapshot()
	} else {
		c = c.ReadOnlySnapshot()
	}

	return newTx(s, write, c), nil
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
