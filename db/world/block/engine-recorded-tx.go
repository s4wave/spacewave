package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// recordedTx is a write transaction of an engine that records operations: it
// applies every World operation through a world_block_tx recorder over the
// engine transaction, and its commit proposes the recorded batch as the
// Candidate's Operation.
type recordedTx struct {
	// WorldState records each operation and applies it to etx.
	*world_block_tx.WorldState
	// etx is the engine transaction the operations apply to.
	etx *EngineTx
}

// newRecordedTx wraps etx in a recorder.
func newRecordedTx(ctx context.Context, etx *EngineTx) (*recordedTx, error) {
	rec, err := world_block_tx.NewWorldState(ctx, etx, true)
	if err != nil {
		etx.Discard()
		return nil, err
	}
	return &recordedTx{WorldState: rec, etx: etx}, nil
}

// Commit closes the recorder, encodes its batch with the engine's bucket ID
// cleared from its object roots, so a replaying member resolves them in its
// own bucket, and commits the engine transaction with it as the Operation and
// with every root it names as a payload, which the commit stores with it.
func (t *recordedTx) Commit(ctx context.Context) error {
	// Close the recorder.
	if err := t.WorldState.Commit(ctx); err != nil {
		t.etx.Discard()
		return err
	}

	// Encode the batch the transaction applied.
	batch := t.GetTxBatch()
	if len(batch.GetTxs()) != 0 {
		batch.ClearBucketID(t.etx.baseHeadRef.GetBucketId())
		op, err := world_block_tx.NewTxBatch(batch)
		if err == nil {
			t.etx.operation, err = op.MarshalVT()
			t.etx.recordedRoots = t.PayloadRoots()
		}
		if err != nil {
			t.etx.Discard()
			return err
		}
	}
	return t.etx.Commit(ctx)
}

// Discard drops the recorded operations and the engine transaction.
func (t *recordedTx) Discard() {
	t.WorldState.Discard()
	t.etx.Discard()
}

// _ is a type assertion
var _ world.Tx = (*recordedTx)(nil)
