package kvtx

import (
	"context"
	"sync/atomic"
)

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
	// wrote is set once a transaction of the store writes to tx.
	wrote atomic.Bool
}

// NewTxStore constructs a new tx store.
func NewTxStore(ops TxOps) *TxStore {
	return &TxStore{tx: ops}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (t *TxStore) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	// Build the transaction and record its writes on the store.
	tx, err := NewTxStoreTx(t.tx)
	if err != nil {
		return nil, err
	}
	tx.wrote = &t.wrote

	// Batch writes when the underlying operations support batches.
	if batch, ok := t.tx.(WriteBatchTxOps); write && ok {
		return &writeBatchTxStoreTx{TxStoreTx: tx, batch: batch}, nil
	}
	return tx, nil
}

// Wrote reports whether a transaction of the store wrote to the underlying
// transaction, so the caller can end an unchanged one without committing it.
func (t *TxStore) Wrote() bool {
	return t.wrote.Load()
}

// writeBatchTxStoreTx advertises batch writes only when the underlying
// operation supports them. It keeps the virtual transaction's lifecycle check;
// never bypass it by unwrapping.
type writeBatchTxStoreTx struct {
	*TxStoreTx
	// batch is the underlying transaction's write-batch capability.
	batch WriteBatchTxOps
}

// ApplyWriteBatch validates the batch keys and forwards the batch to the
// underlying transaction while this virtual transaction is still live.
func (t *writeBatchTxStoreTx) ApplyWriteBatch(ctx context.Context, entries []WriteBatchEntry) error {
	// Validate batch keys before checking the virtual transaction lifecycle.
	for _, entry := range entries {
		if len(entry.Key) == 0 {
			return ErrEmptyKey
		}
	}

	// Forward the write batch while the virtual transaction remains live.
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	if t.discarded {
		return ErrDiscarded
	}
	t.markWrote()
	return t.batch.ApplyWriteBatch(ctx, entries)
}

// _ is a type assertion
var _ Store = (*TxStore)(nil)
