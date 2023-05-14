package world

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// RefCountTx wraps an engine transaction with a refcount reference.
type RefCountTx struct {
	WorldState
	tx  Tx
	ref refcount.RefLike
}

// NewRefCountTx constructs a new refcount engine tx.
func NewRefCountTx(tx Tx, ref refcount.RefLike) *RefCountTx {
	return &RefCountTx{WorldState: tx, tx: tx, ref: ref}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (r *RefCountTx) Commit(ctx context.Context) error {
	defer r.ref.Release()
	return r.tx.Commit(ctx)
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (r *RefCountTx) Discard() {
	defer r.ref.Release()
	r.tx.Discard()
}

// _ is a type assertion
var _ Tx = ((*RefCountTx)(nil))
