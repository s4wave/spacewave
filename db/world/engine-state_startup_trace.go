//go:build bldr_startup_trace && !tinygo

package world

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
)

func (e *engineWorldState) newOperationTransaction(ctx context.Context, write bool) (Tx, error) {
	traceCtx, task := startuptrace.NewTask(ctx, "hydra/world-block/world-state/transaction/open")
	defer task.End()
	return e.e.NewTransaction(traceCtx, write)
}

func discardOperationTransaction(ctx context.Context, tx Tx) {
	_, task := startuptrace.NewTask(ctx, "hydra/world-block/world-state/transaction/discard")
	defer task.End()
	tx.Discard()
}
