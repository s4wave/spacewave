package world_block_tx

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// ErrBatchFull is returned by a BudgetWorldState for an operation that does not
// fit in the remaining byte budget. The operation is not applied.
var ErrBatchFull = errors.New("transaction batch is full")

// BudgetWorldState limits the serialized size of the operations applied through
// it, so the transaction batch they form stays under a byte budget.
//
// The first operation is always accepted, even if it alone exceeds the budget,
// so every batch makes progress. Only applied operations count: they carry the
// caller-sized payloads, while other batch entries have a small fixed size.
type BudgetWorldState struct {
	// WorldState is the wrapped transaction.
	world.WorldState
	// budget is the maximum total entry size in bytes.
	budget int

	// mtx guards used and serializes the check with the apply that follows it.
	mtx sync.Mutex
	// used is the total entry size of the operations applied so far.
	used int
}

// NewBudgetWorldState wraps a write transaction with a byte budget.
func NewBudgetWorldState(ws world.WorldState, budget int) *BudgetWorldState {
	return &BudgetWorldState{WorldState: ws, budget: budget}
}

// ApplyWorldOp applies a world-level operation if it fits in the budget.
func (w *BudgetWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	// Size the batch entry the transaction will record for the op.
	entry, err := NewTxApplyWorldOp(op, opSender)
	if err != nil {
		return 0, false, err
	}
	return applyBudgeted(w, entry.SizeVT(), func() (uint64, bool, error) {
		return w.WorldState.ApplyWorldOp(ctx, op, opSender)
	})
}

// GetObject looks up an object whose operations count against the budget.
func (w *BudgetWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := w.WorldState.GetObject(ctx, key)
	if err != nil || !found {
		world.ReleaseObjectState(obj)
		return nil, false, err
	}
	return &budgetObjectState{ObjectState: obj, w: w}, true, nil
}

// CreateObject creates an object whose operations count against the budget.
func (w *BudgetWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	obj, err := w.WorldState.CreateObject(ctx, key, rootRef)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	return &budgetObjectState{ObjectState: obj, w: w}, nil
}

// RenameObject renames an object, whose operations count against the budget.
func (w *BudgetWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	obj, err := w.WorldState.RenameObject(ctx, oldKey, newKey, descendants)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	return &budgetObjectState{ObjectState: obj, w: w}, nil
}

// applyBudgeted runs apply if an entry of size bytes fits in the budget and
// counts the entry once apply succeeds. It returns ErrBatchFull otherwise.
func applyBudgeted(w *BudgetWorldState, size int, apply func() (uint64, bool, error)) (uint64, bool, error) {
	// Hold the lock so the check and the apply that follows it are one step.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.used != 0 && w.used+size > w.budget {
		return 0, false, ErrBatchFull
	}

	// Count the entry once the transaction has recorded it.
	seqno, sysErr, err := apply()
	if err == nil {
		w.used += size
	}
	return seqno, sysErr, err
}

// _ is a type assertion
var _ world.WorldState = (*BudgetWorldState)(nil)
