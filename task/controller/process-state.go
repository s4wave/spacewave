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
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		taskState, berr = forge_task.UnmarshalTask(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	// signal to the controller to stop watching for pass state if not running
	currState := taskState.GetTaskState()
	if currState != forge_task.State_TaskState_RUNNING {
		c.syncWatchPassStates(nil)
	}

	// check if completed
	if currState == forge_task.State_TaskState_COMPLETE {
		le.Debug("task is marked as complete")
		return true, nil
	}

	// check if peer id matches
	if c.peerIDStr != taskState.GetPeerId() {
		le.Warnf("task peer id %q does not match ours %q", taskState.GetPeerId(), c.peerIDStr)
		return true, nil
	}

	// start the task if pending
	if currState == forge_task.State_TaskState_PENDING {
		// pre-resolve and check the target and inputs
		tgt, tgtKey, err := forge_task.LookupTaskTarget(ctx, ws, objKey)
		if err != nil {
			return true, errors.Wrap(err, "target")
		}
		if tgt == nil {
			le.Debug("waiting for target to be set")
			return true, nil
		}
		_ = tgtKey

		// TODO pre-resolve and check target + inputs
		// this is also done in the tx, but we should check before sending
		var valueSet *forge_target.ValueSet
		_ = valueSet

		txStart := task_tx.NewTxStart(objKey, c.conf.GetAssignSelf())
		_, _, err = ws.ApplyWorldOp(txStart, c.peerID)
		return true, errors.Wrap(err, "start task")
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
			txUpdate := task_tx.NewTxUpdatePassState(objKey)
			_, _, err = ws.ApplyWorldOp(txUpdate, c.peerID)
			return true, errors.Wrap(err, "update pass state")
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
		var passOutputs []*forge_value.Value
		passOutputs, err = forge_pass.ComputeOutputsWithStates(outputs, taskPass.GetExecStates(), int(taskState.GetReplicas()))
		if err != nil {
			err = errors.Wrapf(err, "pass[%d]: compute outputs", passNonce)
		}
		// verify the outputs match what the pass has
		if err == nil && !forge_value.CompareValueSet(passOutputs, taskState.GetValueSet().GetOutputs()) {
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
