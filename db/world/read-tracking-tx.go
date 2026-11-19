package world

import "context"

// readTrackingTx reads through a tracked state and ends the original transaction.
type readTrackingTx struct {
	// readSetWorldState records reads and preserves its tracked batch methods.
	*readSetWorldState
	// tx supplies Commit and Discard without changing their lifetimes.
	tx Tx
}

// Commit commits the underlying transaction.
func (t *readTrackingTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// CommitAdmit commits the underlying transaction once its engine admits it.
func (t *readTrackingTx) CommitAdmit(ctx context.Context) (Admission, error) {
	return CommitAdmit(ctx, t.tx)
}

// Discard discards the underlying transaction.
func (t *readTrackingTx) Discard() {
	t.tx.Discard()
}

// _ checks the tracked transaction's contract.
var (
	_ Tx                         = (*readTrackingTx)(nil)
	_ ObjectRootRefBatcher       = (*readTrackingTx)(nil)
	_ ObjectBodyBatcher          = (*readTrackingTx)(nil)
	_ ObjectBodyPageBatcher      = (*readTrackingTx)(nil)
	_ ObjectBodyPageSeqnoBatcher = (*readTrackingTx)(nil)
	_ AdmitCommitter             = (*readTrackingTx)(nil)
)
