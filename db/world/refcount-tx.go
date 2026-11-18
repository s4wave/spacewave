package world

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
)

// RefCountTx wraps an engine transaction with a refcount reference.
type RefCountTx struct {
	WorldState
	// tx is the wrapped transaction.
	tx Tx
	// ref is the engine reference the transaction holds until it ends.
	ref refcount.RefLike
}

// NewRefCountTx constructs a new refcount engine tx.
func NewRefCountTx(tx Tx, ref refcount.RefLike) *RefCountTx {
	return &RefCountTx{WorldState: tx, tx: tx, ref: ref}
}

// WaitObjectRev waits through the wrapped transaction.
func (r *RefCountTx) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return WaitObjectRev(ctx, r.WorldState, key, rev, ignoreNotFound)
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

// GetObjectRootRefsBatch returns object root refs through the wrapped
// transaction.
func (r *RefCountTx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error) {
	return GetObjectRootRefsBatch(ctx, r.tx, keys)
}

// _ is a type assertion
var (
	_ Tx                   = (*RefCountTx)(nil)
	_ ObjectRootRefBatcher = (*RefCountTx)(nil)
)
