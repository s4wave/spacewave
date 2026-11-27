package task_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxCompleteCached constructs the COMPLETE_CACHED transaction.
func NewTxCompleteCached(objKey, cachedTaskKey string) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType:           TxType_TxType_COMPLETE_CACHED,
		TxCompleteCached: &TxCompleteCached{CachedTaskKey: cachedTaskKey},
	}
}

// GetTxType returns the transaction's type.
func (t *TxCompleteCached) GetTxType() TxType {
	return TxType_TxType_COMPLETE_CACHED
}

// Validate checks the transaction cursorily.
func (t *TxCompleteCached) Validate() error {
	if t.GetCachedTaskKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cached_task_key")
	}
	return nil
}

// ExecuteTx completes the PENDING Task with the earlier Task's outputs. It
// recomputes the reuse key from the Task's own Target and inputs and requires
// the earlier Task to be recorded with that key, so a stale or wrong
// candidate is refused instead of completing the Task with foreign outputs.
func (t *TxCompleteCached) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	// Only a Task waiting to start can take an earlier result.
	cachedKey := t.GetCachedTaskKey()
	if cachedKey == objKey {
		return world.NewOpRefusedError(errors.New("task cannot reuse its own outputs"))
	}
	if state := root.GetTaskState(); state != forge_task.State_TaskState_PENDING {
		return world.NewOpRefusedError(errors.Wrapf(forge_value.ErrUnknownState, "%s", state.String()))
	}

	// Compute the Task's reuse key from its frozen Target and inputs.
	tgt, _, err := root.FollowTargetRef(ctx, bcs)
	if err != nil {
		return err
	}
	reuseKey, err := forge_task.NewReuseKey(tgt, root.GetValueSet().GetInputs())
	if err != nil {
		return err
	}
	if reuseKey == "" {
		return world.NewOpRefusedError(errors.New("task target does not reuse outputs"))
	}

	// Require a successful earlier Task recorded with the same key.
	recorded, err := forge_task.CheckTaskReuseKey(ctx, worldState, cachedKey, reuseKey)
	if err != nil {
		return err
	}
	if !recorded {
		return world.NewOpRefusedError(errors.Errorf("task %s has a different reuse key", cachedKey))
	}
	cached, err := forge_task.LookupTaskBody(ctx, worldState, cachedKey)
	if err != nil {
		return err
	}
	if cached.GetTaskState() != forge_task.State_TaskState_COMPLETE || !cached.GetResult().GetSuccess() {
		return world.NewOpRefusedError(errors.Errorf("task %s is not complete and successful", cachedKey))
	}

	// Link the Task to its source and record its key, so the result stays
	// findable after the earlier Task is removed.
	if err := forge_task.LinkTaskCached(ctx, worldState, objKey, cachedKey); err != nil {
		return err
	}
	if err := worldState.SetGraphQuad(ctx, forge_task.NewTaskToReuseKeyQuad(objKey, reuseKey)); err != nil {
		return err
	}

	// Complete the Task with the earlier outputs.
	if root.ValueSet == nil {
		root.ValueSet = &forge_target.ValueSet{}
	}
	root.ValueSet.Outputs = forge_value.ValueSlice(cached.GetValueSet().GetOutputs()).Clone()
	root.TaskState = forge_task.State_TaskState_COMPLETE
	root.Result = forge_value.NewResultWithSuccess()
	bcs.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = (*TxCompleteCached)(nil)
