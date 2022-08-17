package task_controller

import (
	"context"

	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	task_tx "github.com/aperturerobotics/forge/task/tx"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/pkg/errors"
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
		taskState, berr = forge_task.UnmarshalTask(bcs)
		if berr == nil {
			taskTarget, _, berr = taskState.FollowTargetRef(bcs)
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

	// check if completed
	// TODO: add an option to enable restarting COMPLETE tasks if inputs change.
	if currState == forge_task.State_TaskState_COMPLETE {
		le.Debug("task is marked as complete")
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

	// compute any changes in the inputs as well.
	defWorld := forge_target.NewInputValueWorld(nil, ws)
	inputMap, unsetInputs, inputMapRel, err := forge_target.ResolveInputMap(ctx, c.bus, defWorld, tgt, nil)
	if err != nil {
		return true, err
	}

	// build the value set
	inputValueSet := inputMap.BuildValueSet()

	// compare the value set with the stored inputs
	var inputSet forge_value.ValueSlice = inputValueSet.GetInputs()
	var oldInputSet forge_value.ValueSlice = taskState.GetValueSet().GetInputs()
	addedInputs, removedInputs, changedInputs := oldInputSet.Compare(inputSet)
	inputsDirty := len(addedInputs)+len(removedInputs)+len(changedInputs) != 0

	// release the map, we don't need it anymore.
	inputMapRel()

	// if the target or any inputs changed, transmit a transaction to update.
	if targetDirty || inputsDirty {
		txUpdateInputs := task_tx.NewTxUpdateInputs(objKey)
		txInner := txUpdateInputs.TxUpdateInputs
		txInner.ResetInputs = len(inputSet) == 0
		txInner.UpdateTarget = targetDirty
		txInner.ValueSet = forge_target.NewValueSet()
		for _, input := range addedInputs {
			txInner.ValueSet.Inputs = append(txInner.ValueSet.Inputs, input)
		}
		for _, input := range changedInputs {
			txInner.ValueSet.Inputs = append(txInner.ValueSet.Inputs, input)
		}
		for _, input := range removedInputs {
			txInner.ValueSet.Inputs = append(txInner.ValueSet.Inputs, &forge_value.Value{
				Name:      input.GetName(),
				ValueType: 0,
			})
		}
		txInner.ValueSet.SortValues()
		_, _, err = ws.ApplyWorldOp(txUpdateInputs, c.peerID)
		if err != nil {
			return true, errors.Wrap(err, "update inputs")
		}
		return true, nil
	}

	// update the list of Input world objects to watch.
	c.syncWatchInputObjects(tgt.GetInputs())

	// if any unset inputs: exit here.
	if len(unsetInputs) != 0 {
		unsetInputNames := forge_target.GetInputsNames(unsetInputs)
		le.Debugf("waiting for %d unset inputs: %s", len(unsetInputNames), unsetInputNames)
		return true, nil
	}

	// start the task if pending
	if currState == forge_task.State_TaskState_PENDING {
		txStart := task_tx.NewTxStart(objKey, c.conf.GetAssignSelf())
		_, _, err = ws.ApplyWorldOp(txStart, c.peerID)
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
			_, _, err = ws.ApplyWorldOp(txUpdate, c.peerID)
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

// processCheckTaskResult processes the task in the CHECKING state.
// in the future, additional checks may be added here.
func (c *Controller) processCheckTaskResult(ctx context.Context, ws world.WorldState, taskState *forge_task.Task) error {
	passNonce := taskState.GetPassNonce()

	// look up the completed pass
	taskPass, taskPassTgt, _, err := forge_task.LookupTaskPass(ctx, ws, c.objKey, passNonce)
	if err == nil {
		if taskPass == nil {
			err = errors.Wrap(world.ErrObjectNotFound, "task pass")
		} else {
			err = taskPass.Validate(false)
		}
	}

	// check that the pass completed successfully
	passResult := taskPass.GetResult()
	if err == nil && !passResult.GetSuccess() {
		passResult.FillFailError()
		err = errors.New(passResult.GetFailError())
		err = errors.Wrap(err, "pass failed")
	}

	// look up the outputs
	outputs := taskPassTgt.GetOutputs()
	if err == nil && len(outputs) != 0 {
		// compute the outputs from the exec states
		var passOutputs forge_value.ValueSlice
		passOutputs, err = forge_pass.ComputeOutputsWithStates(outputs, taskPass.GetExecStates(), int(taskState.GetReplicas()))
		if err != nil {
			err = errors.Wrapf(err, "pass[%d]: compute outputs", passNonce)
		}
		// verify the outputs match what the pass has
		if err == nil && !passOutputs.Equals(taskState.GetValueSet().GetOutputs()) {
			err = errors.Wrapf(err, "pass[%d]: outputs mismatch re-computed values", passNonce)
		}
	}
	if err != nil {
		c.le.WithError(err).Warn("marking task as failed w/ error")
		tx := task_tx.NewTxComplete(c.objKey, forge_value.NewResultWithError(err))
		_, _, err = ws.ApplyWorldOp(tx, c.peerID)
		return err
	}

	c.le.Info("marking task as complete")
	tx := task_tx.NewTxComplete(c.objKey, forge_value.NewResultWithSuccess())
	_, _, err = ws.ApplyWorldOp(tx, c.peerID)
	return err
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState
