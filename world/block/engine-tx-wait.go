package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
)

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
// Note: this contains the seqno of the tx if this is a transaction.
func (e *EngineTx) GetSeqno() (uint64, error) {
	var seqno uint64
	err := e.performOp(func(tx *Tx) error {
		var berr error
		seqno, berr = tx.GetSeqno()
		return berr
	})
	return seqno, err
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
// Note: this waits for the engine seqno, not the pending tx seqno.
func (e *EngineTx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.engine.WaitSeqno(ctx, value)
}

// _ is a type assertion
var _ world.WorldWait = ((*EngineTx)(nil))
