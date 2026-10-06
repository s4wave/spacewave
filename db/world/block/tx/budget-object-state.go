package world_block_tx

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// budgetObjectState counts the operations applied to an object against the
// budget of the BudgetWorldState that returned it.
type budgetObjectState struct {
	// ObjectState is the wrapped object.
	world.ObjectState
	// w is the budgeted world state.
	w *BudgetWorldState
}

// newBudgetObjectState wraps an object of the world state.
func newBudgetObjectState(w *BudgetWorldState, obj world.ObjectState) *budgetObjectState {
	return &budgetObjectState{ObjectState: obj, w: w}
}

// Release releases the wrapped object handle.
func (o *budgetObjectState) Release() {
	world.ReleaseObjectState(o.ObjectState)
}

// ApplyObjectOp applies an object operation if it fits in the budget.
func (o *budgetObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	// Size the batch entry the transaction will record for the op.
	entry, err := NewTxApplyObjectOp(op.GetOperationTypeId(), op, o.GetKey(), opSender)
	if err != nil {
		return 0, false, err
	}
	return applyBudgeted(o.w, entry.SizeVT(), func() (uint64, bool, error) {
		return o.ObjectState.ApplyObjectOp(ctx, op, opSender)
	})
}

// _ is a type assertion
var _ world.ObjectState = (*budgetObjectState)(nil)
