package world_block_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// CommitBatches applies a long-running write as a series of transactions whose
// batches each stay under a byte budget, so no commit exceeds the size limit of
// the engine's operations.
//
// Each round runs apply on a fresh write transaction wrapped in a
// BudgetWorldState. When apply returns ErrBatchFull the round is committed and
// apply runs again on the next transaction; when it returns nil the final round
// is committed. Any other error discards the current round and returns, leaving
// the earlier rounds committed.
//
// apply must be resumable: it sees the earlier rounds and does only the work
// they did not finish, as a directory sync does. Each round applies at least one
// operation, so a resumable apply makes progress.
func CommitBatches(
	ctx context.Context,
	eng world.Engine,
	budget int,
	apply func(ctx context.Context, ws world.WorldState) error,
) error {
	for {
		batchFull, err := commitBatch(ctx, eng, budget, apply)
		if err != nil || !batchFull {
			return err
		}
	}
}

// commitBatch runs one round of CommitBatches and reports if apply stopped
// because the batch was full.
func commitBatch(
	ctx context.Context,
	eng world.Engine,
	budget int,
	apply func(ctx context.Context, ws world.WorldState) error,
) (bool, error) {
	// Open the round's transaction.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	// Apply until done or full; only a full batch continues in the next round.
	err = apply(ctx, NewBudgetWorldState(tx, budget))
	batchFull := errors.Is(err, ErrBatchFull)
	if err != nil && !batchFull {
		return false, err
	}
	return batchFull, tx.Commit(ctx)
}
