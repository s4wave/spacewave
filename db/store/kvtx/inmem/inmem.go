package store_kvtx_inmem

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/kvtx"
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
	// writeWaiting is the number of write tx waiting to acquire.
	// Readers are held while this is non-zero so waiting writers are not
	// starved; each waiter releases its own registration on acquire or cancel.
	writeWaiting int
}

// NewStore constructs a new key-value store.
func NewStore() *Store {
	return &Store{tree: btree.NewBTreeG[*valType](valTypeLess)}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	var tx kvtx.Tx
	// waiting indicates this call registered as a waiting writer and still
	// owns a writeWaiting increment it must release on any early exit.
	var waiting bool
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		if write {
			if s.nreaders != 0 || s.writing {
				s.writeWaiting++
				waiting = true
				waitCh = getWaitCh()
			} else {
				s.writing = true
				tx = newTx(s, true)
			}
		} else if !s.writing && s.writeWaiting == 0 {
			s.nreaders++
			tx = newTx(s, false)
		} else {
			waitCh = getWaitCh()
		}
	})

	if tx != nil {
		return tx, nil
	}

	for {
		select {
		case <-ctx.Done():
			// A cancelled write waiter must release its admission block on
			// readers. writeWaiting gates reader admission, so leaking this
			// registration permanently starves every subsequent reader on the
			// shared store (the tx is nil, so the caller's Discard never runs).
			if waiting {
				s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					s.writeWaiting--
					broadcast()
				})
			}
			return nil, context.Canceled
		case <-waitCh:
		}

		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if write {
				if s.nreaders == 0 && !s.writing {
					s.writeWaiting--
					waiting = false
					s.writing = true
					tx = newTx(s, true)
				} else {
					waitCh = getWaitCh()
				}
			} else if !s.writing && s.writeWaiting == 0 {
				s.nreaders++
				tx = newTx(s, false)
			} else {
				waitCh = getWaitCh()
			}
		})

		if tx != nil {
			return tx, nil
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
