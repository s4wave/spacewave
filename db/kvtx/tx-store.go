package kvtx

import "context"

// TxStore implements the Store interface backed by a single Tx instance.
// This allows many transactions to be batched into one Tx.
//
// Discard of a write tx after changes were made returns an error.
// Discard followed by Commit returns ErrDiscarded
// Commit followed by Discard returns nil.
// If tx committed or discarded, operations return ErrDiscarded.
// It's not possible to roll-back changes as we are proxying to 1 txops object.
type TxStore struct {
	// tx is the underlying tx
	tx TxOps
}

// NewTxStore constructs a new tx store.
func NewTxStore(ops TxOps) *TxStore {
	return &TxStore{tx: ops}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (t *TxStore) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	return NewTxStoreTx(t.tx)
}

// _ is a type assertion
var _ Store = (*TxStore)(nil)
