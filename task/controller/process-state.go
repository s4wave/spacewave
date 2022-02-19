package task_controller

import (
	"context"

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
		c.pushWatchPassState(nil)
	}

	// check if completed
	if currState == forge_task.State_TaskState_COMPLETE {
		le.Debug("task is marked as complete")
		return false, nil
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
		return true, err
	}

	// start the pass watcher if running
	if currState == forge_task.State_TaskState_RUNNING {
		// lookup the current pass
		passes, passKeys, err := forge_task.CollectTaskPasses(ctx, ws, objKey)
		if err != nil {
			c.pushWatchPassState(nil)
			return true, err
		}
		activePass, activePassIdx := forge_task.FindPassWithNonce(taskState.GetPassNonce(), passes)
		if activePass == nil {
			c.pushWatchPassState(nil)

			// active pass is nil, submit a tx to go back to pending
			txUpdate := task_tx.NewTxUpdatePassState(objKey)
			_, _, err = ws.ApplyWorldOp(txUpdate, c.peerID)
			return true, err
		}
		// watch the pass for completion
		c.pushWatchPassState(newPassState(passKeys[activePassIdx], activePass))
		return true, nil
	}

	// unknown state
	return true, errors.Wrapf(
		forge_value.ErrUnknownState,
		"%s", currState.String(),
	)
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState
