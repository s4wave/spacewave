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

// GetObjectBodiesBatch returns object bodies through the wrapped transaction.
func (r *RefCountTx) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error) {
	return GetObjectBodiesBatch(ctx, r.tx, keys)
}

// GetObjectBodiesBatchPage returns one budgeted page through the wrapped
// transaction.
func (r *RefCountTx) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, error) {
	return GetObjectBodiesBatchPage(ctx, r.tx, keys, byteBudget)
}

// GetObjectBodiesBatchPageWithSeqno returns one budgeted page and its World
// sequence number through the wrapped transaction.
func (r *RefCountTx) GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, uint64, error) {
	return GetObjectBodiesBatchPageWithSeqno(ctx, r.tx, keys, byteBudget)
}

// GetObjectRootRefsBatch returns object root refs through the wrapped
// transaction.
func (r *RefCountTx) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error) {
	return GetObjectRootRefsBatch(ctx, r.tx, keys)
}

// _ is a type assertion
var (
	_ Tx                         = (*RefCountTx)(nil)
	_ ObjectBodyBatcher          = (*RefCountTx)(nil)
	_ ObjectBodyPageBatcher      = (*RefCountTx)(nil)
	_ ObjectBodyPageSeqnoBatcher = (*RefCountTx)(nil)
	_ ObjectRootRefBatcher       = (*RefCountTx)(nil)
)
