package task_controller

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// unmarshal Task state + build read cursor
	var taskState *forge_task.Task
	var taskTarget *forge_target.Target
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		taskState, berr = forge_task.UnmarshalTask(ctx, bcs)
		if berr == nil {
			taskTarget, _, berr = taskState.FollowTargetRef(ctx, bcs)
		}
		return berr
	})
	if err != nil {
		return false, err
	}

	// Task is not running: signal to the controller to stop watching pass states
	currState := taskState.GetTaskState()
	if currState != forge_task.State_TaskState_RUNNING {
		c.syncWatchPassStates(nil)
	}
	// Explicit cancellation remains stopped until the operator retries it.
	if currState == forge_task.State_TaskState_COMPLETE && taskState.GetResult().GetCanceled() {
		c.syncWatchInputObjects(nil, false)
		return true, nil
	}

	// check if peer id matches
	if c.peerIDStr != taskState.GetPeerId() {
		le.Warnf("task peer id %q does not match ours %q", taskState.GetPeerId(), c.peerIDStr)
		return true, nil
	}

	// lookup the latest version of the task target
	tgt, _, err := forge_task.LookupTaskTarget(ctx, ws, objKey)
	if err != nil {
		return true, errors.Wrap(err, "target")
	}
	if tgt == nil {
		le.Debug("waiting for target to exist")
		return true, nil
	}

	// compare (note: existingTgt and tgt both might be nil)
	targetDirty := !taskTarget.EqualVT(tgt)

	// Seed target resolution with stored inputs so scheduler-supplied names
	// survive reconciliation; target resolvers may overwrite their own names.
	storedInputs, err := forge_value.ValueSlice(taskState.GetValueSet().GetInputs()).
		BuildValueMap(true, true)
	if err != nil {
		return true, errors.Wrap(err, "stored inputs")
	}
	defWorld := forge_target.NewInputValueWorld(nil, ws)
	inputMap, unsetInputs, inputMapRel, err := forge_target.ResolveInputMap(ctx, c.bus, defWorld, tgt, storedInputs)
	if err != nil {
		return true, errors.Wrap(err, "resolve inputs")
	}

	// build the value set
	inputValueSet := inputMap.BuildValueSet()

	// compare the value set with the stored inputs
	var inputSet forge_value.ValueSlice = inputValueSet.GetInputs()
	var oldInputSet forge_value.ValueSlice = taskState.GetValueSet().GetInputs()
	addedInputs, removedInputs, changedInputs := oldInputSet.Compare(inputSet)

	// release the map, we don't need it anymore.
	inputMapRel()

	// determine if the inputs are dirty and need to trigger a update
	// if the Task is not PENDING and !input.WatchChanges, ignore the change.
	inputsDirty := taskInputsDirty(
		currState,
		taskTarget.GetInputs(),
		addedInputs,
		removedInputs,
		changedInputs,
	)

	// if the target or any inputs changed, transmit a transaction to update.
	if targetDirty || inputsDirty {
		txUpdateInputs := task_tx.NewTxUpdateInputs(objKey)
		txInner := txUpdateInputs.TxUpdateInputs
		txInner.ResetInputs = len(inputSet) == 0
		txInner.UpdateTarget = targetDirty
		txInner.ValueSet = buildUpdateInputValueSet(
			inputSet,
			addedInputs,
			changedInputs,
			removedInputs,
		)
		_, _, err = ws.ApplyWorldOp(ctx, txUpdateInputs, c.peerID)
		if err != nil {
			return true, errors.Wrap(err, "update inputs")
		}
		return true, nil
	}

	// update the list of Input world objects to watch.
	c.syncWatchInputObjects(tgt.GetInputs(), len(unsetInputs) != 0)

	// if any unset inputs: exit here.
	if len(unsetInputs) != 0 {
		unsetInputNames := forge_target.GetInputsNames(unsetInputs)
		le.Debugf("waiting for %d unset inputs: %s", len(unsetInputNames), unsetInputNames)
		return true, nil
	}

	// A completed task is current until its target or watched inputs change.
	if currState == forge_task.State_TaskState_COMPLETE {
		le.Debug("task is marked as complete")
		return true, nil
	}

	// start the task if pending
	if currState == forge_task.State_TaskState_PENDING {
		txStart := task_tx.NewTxStart(objKey, c.conf.GetAssignSelf())
		_, _, err = ws.ApplyWorldOp(ctx, txStart, c.peerID)
		if err != nil {
			return true, errors.Wrap(err, "start task")
		}
		return true, nil
	}

	// start the pass watcher if running
	if currState == forge_task.State_TaskState_RUNNING {
		// lookup the current pass
		passes, _, passKeys, err := forge_task.CollectTaskPasses(ctx, ws, objKey)
		if err != nil {
			c.syncWatchPassStates(nil)
			return true, errors.Wrap(err, "collect task passes")
		}
		activePass, activePassIdx := forge_task.FindPassWithNonce(taskState.GetPassNonce(), passes)
		if activePass == nil {
			c.syncWatchPassStates(nil)

			// active pass is nil, submit a tx to go back to pending
			txUpdate := task_tx.NewTxUpdateWithPassState(objKey)
			_, _, err = ws.ApplyWorldOp(ctx, txUpdate, c.peerID)
			return true, errors.Wrap(err, "update with pass state")
		}
		// watch the pass for completion
		passState := newPassState(passKeys[activePassIdx], activePass)
		c.syncWatchPassStates(passState)
		return true, nil
	}

	// check the output of the most recent pass & submit it with tx-complete
	if currState == forge_task.State_TaskState_CHECKING {
		c.le.Debug("processing CHECKING state")
		return true, c.processCheckTaskResult(ctx, ws, taskState)
	}

	// unknown state
	return true, errors.Wrapf(
		forge_value.ErrUnknownState,
		"%s", currState.String(),
	)
}

// taskInputsDirty applies the target's restart policy to resolved input changes.
func taskInputsDirty(
	taskState forge_task.State,
	targetInputs []*forge_target.Input,
	addedInputs, removedInputs, changedInputs forge_value.ValueSlice,
) bool {
	if taskState == forge_task.State_TaskState_PENDING || taskState == forge_task.State_TaskState_RETRY {
		return len(addedInputs)+len(removedInputs)+len(changedInputs) != 0
	}
	for _, taskInput := range targetInputs {
		if !taskInput.GetWatchChanges() {
			continue
		}
		for _, changes := range []forge_value.ValueSlice{addedInputs, removedInputs, changedInputs} {
			for _, changedInput := range changes {
				if changedInput.GetName() == taskInput.GetName() {
					return true
				}
			}
		}
	}
	return false
}

// buildUpdateInputValueSet builds the input delta accepted by TxUpdateInputs.
func buildUpdateInputValueSet(
	inputSet, addedInputs, changedInputs, removedInputs forge_value.ValueSlice,
) *forge_target.ValueSet {
	valueSet := forge_target.NewValueSet()
	valueSet.Inputs = append(valueSet.Inputs, addedInputs...)
	valueSet.Inputs = append(valueSet.Inputs, changedInputs...)
	for _, input := range removedInputs {
		valueSet.Inputs = append(valueSet.Inputs, &forge_value.Value{
			Name:      input.GetName(),
			ValueType: 0,
		})
	}
	if len(valueSet.Inputs) == 0 && len(inputSet) != 0 {
		valueSet.Inputs = slices.Clone(inputSet)
	}
	valueSet.SortValues()
	return valueSet
}

// processCheckTaskResult processes the task in the CHECKING state.
// in the future, additional checks may be added here.
func (c *Controller) processCheckTaskResult(ctx context.Context, ws world.WorldState, taskState *forge_task.Task) error {
	c.le.Debug("submitting CHECKING completion")
	tx := task_tx.NewTxComplete(c.objKey, forge_value.NewResultWithSuccess())
	_, _, err := ws.ApplyWorldOp(ctx, tx, c.peerID)
	return err
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*Controller)(nil).ProcessState
