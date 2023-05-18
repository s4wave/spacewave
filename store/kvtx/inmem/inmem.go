package store_kvtx_inmem

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/util/broadcast"
)

// Store is a in-memory key-value store.
//
// Uses a fast ctrie (concurrent trie) K/V map.
type Store struct {
	// m is the map containing the store
	// key is encoded with base58
	m map[uint64]valType

	// bcast is broadcast when below fields change
	bcast broadcast.Broadcast
	// mtx guards the below fields
	mtx sync.Mutex
	// nreaders is the number of active readers
	nreaders int
	// writing indicates there's a write tx active
	writing bool
	// writeWaiting indicates a write tx is waiting
	writeWaiting bool
}

// NewStore constructs a new key-value store.
func NewStore() *Store {
	return &Store{m: map[uint64]valType{}}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	for {
		var tx kvtx.Tx
		s.mtx.Lock()
		if write {
			if s.nreaders != 0 || s.writing {
				s.writeWaiting = true
			} else {
				s.writing = true
				s.writeWaiting = false
				tx = newTx(s, true)
			}
		} else {
			if !s.writing && !s.writeWaiting {
				s.nreaders++
				tx = newTx(s, false)
			}
		}
		var bcastCh <-chan struct{}
		if tx == nil {
			bcastCh = s.bcast.GetWaitCh()
		}
		s.mtx.Unlock()
		if tx != nil {
			return tx, nil
		}
		<-bcastCh
	}
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
