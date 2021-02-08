package store_kvtx_inmem

import (
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/tidwall/btree"
)

// Store is a in-memory key-value store.
//
// Uses a K/V map.
type Store struct {
	// tree is the btree containing the store
	tree *btree.BTreeG[*valType]

	// bcast guards below fields
	bcast broadcast.Broadcast
	// nreaders is the number of active readers
	nreaders int
	// writing indicates there's a write tx active
	writing bool
	// writeWaiting indicates a write tx is waiting
	writeWaiting bool
}

// NewStore constructs a new key-value store.
func NewStore() *Store {
	return &Store{tree: btree.NewBTreeG[*valType](valTypeLess)}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	for {
		var tx kvtx.Tx
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
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
			if tx == nil {
				waitCh = getWaitCh()
			}
		})

		if tx != nil {
			return tx, nil
		}

		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
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
