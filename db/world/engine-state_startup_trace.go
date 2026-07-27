//go:build bldr_startup_trace && !tinygo

package world

import (
	"context"

	startuptrace "github.com/s4wave/spacewave/db/traceutil/startup"
)

func (e *engineWorldState) newOperationTransaction(ctx context.Context, write bool) (Tx, error) {
	taskType := "hydra/world-block/world-state/transaction/open"
	if startuptrace.GraphLookupScope(ctx) {
		taskType += "/eligibility-graph-node"
	}
	traceCtx, task := startuptrace.NewTask(ctx, taskType)
	defer task.End()
	return e.e.NewTransaction(traceCtx, write)
}

func discardOperationTransaction(ctx context.Context, tx Tx) {
	taskType := "hydra/world-block/world-state/transaction/discard"
	if startuptrace.GraphLookupScope(ctx) {
		taskType += "/eligibility-graph-node"
	}
	_, task := startuptrace.NewTask(ctx, taskType)
	defer task.End()
	tx.Discard()
}
