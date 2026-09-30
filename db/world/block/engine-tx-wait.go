package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
// The sequence belongs to this pending writer or immutable read snapshot.
func (e *EngineTx) GetSeqno(ctx context.Context) (uint64, error) {
	// Read the sequence through the transaction's retained state.
	var seqno uint64
	err := e.performOp(ctx, func(tx *Tx) error {
		var berr error
		seqno, berr = tx.GetSeqno(ctx)
		return berr
	})
	return seqno, err
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
// Writers wait on their pending transaction broadcasts. Read snapshots cannot
// advance beyond their retained sequence; Discard or cancellation ends the wait.
func (e *EngineTx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	// Wait on the same transaction state that supplies GetSeqno.
	var seqno uint64
	err := e.performOp(ctx, func(tx *Tx) error {
		var err error
		seqno, err = tx.WaitSeqno(ctx, value)
		return err
	})
	return seqno, err
}

// _ is a type assertion
var _ world.WorldWaitSeqno = (*EngineTx)(nil)
