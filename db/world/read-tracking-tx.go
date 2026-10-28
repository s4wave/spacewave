package world

import "context"

// TrackTx returns tx recording the reads made through it in reads. Commit and
// Discard end tx.
func TrackTx(tx Tx, reads *ReadSet) Tx {
	return &readTrackingTx{
		readSetWorldState: &readSetWorldState{WorldState: tx, reads: reads},
		tx:                tx,
	}
}

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
)
