package kvtx_txcache

import "github.com/aperturerobotics/hydra/kvtx"

// TxStore wraps a single Transaction to create a read/write store.
// Buffers changes in memory so that Discard() and Commit() work correctly.
// Commit() on a write transaction will forward writes to the TxOps.
type TxStore struct {
	tx    kvtx.Tx
	write bool
}

// NewTxStore constructs a new transaction store.
// write indicates if writes to the tx are allowed
func NewTxStore(tx kvtx.Tx, write bool) *TxStore {
	return &TxStore{
		tx:    tx,
		write: write,
	}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (t *TxStore) NewTransaction(write bool) (kvtx.Tx, error) {
	if write && !t.write {
		return nil, kvtx.ErrNotWrite
	}

	return NewTxWithCbs(
		t.tx,
		write,
		nil,
		func() (kvtx.Tx, error) {
			return t.tx, nil
		},
		false,
	)
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
