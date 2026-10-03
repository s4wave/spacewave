package task_tx_test

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestTxRetryRejectsLivePredecessor(t *testing.T) {
	// Prepare a Task and predecessor Pass in the execution custody fixture.
	f := newCustodyFixture(t)
	taskKey := "test/task/retry-live-predecessor"
	passKey := forge_task.NewPassKey(taskKey, 1)

	// Create the Task with the fixture Target.
	{
		createdObject, _, err := forge_task.CreateTaskWithTarget(f.ctx, f.tb.WorldState,
			f.peerID, taskKey, "retry-live-predecessor", f.target.CloneVT(), f.peerID, 1, nil, f.ts)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Install the fixture Target on the Task.
	updateTarget := task_tx.NewTxUpdateInputs(taskKey)
	updateTarget.TxUpdateInputs.UpdateTarget = true
	updateTarget.TxUpdateInputs.ResetInputs = true
	if _, _, err := f.tb.WorldState.ApplyWorldOp(f.ctx, updateTarget, f.peerID); err != nil {
		t.Fatal(err)
	}

	// Start the predecessor Pass and link it to the Task.
	f.createRunningPass(t, passKey, 1)
	if err := f.tb.WorldState.SetGraphQuad(f.ctx,
		forge_task.NewTaskToPassQuad(taskKey, passKey, 1)); err != nil {
		t.Fatal(err)
	}

	// Record a failed Task while the predecessor execution remains live.
	failed := forge_value.NewResultWithError(errors.New("predecessor failed"))
	if _, _, err := world.AccessWorldObject(f.ctx, f.tb.WorldState, taskKey, true,
		func(bcs *block.Cursor) error {
			// Read the Task before assigning its failed attempt Result.
			task, err := forge_task.UnmarshalTask(f.ctx, bcs)
			if err != nil {
				return err
			}

			// Save the completed Task with its predecessor Pass nonce.
			task.TaskState = forge_task.State_TaskState_COMPLETE
			task.PassNonce = 1
			task.Result = failed.Clone()
			bcs.SetBlock(task, true)
			return nil
		}); err != nil {
		t.Fatal(err)
	}

	// Record the Pass as failed without ending its live execution.
	if _, _, err := world.AccessWorldObject(f.ctx, f.tb.WorldState, passKey, true,
		func(bcs *block.Cursor) error {
			// Read the predecessor Pass before assigning its failed Result.
			pass, err := forge_pass.UnmarshalPass(f.ctx, bcs)
			if err != nil {
				return err
			}

			// Save the completed Pass with its failed Result.
			pass.PassState = forge_pass.State_PassState_COMPLETE
			pass.Result = failed.Clone()
			bcs.SetBlock(pass, true)
			return nil
		}); err != nil {
		t.Fatal(err)
	}

	// Verify the Task retry rejects the live predecessor execution.
	if _, _, err := f.tb.WorldState.ApplyWorldOp(f.ctx,
		task_tx.NewTxRetry(taskKey, 1, forge_target.NewValueSet()), f.peerID); err == nil {
		t.Fatal("retry accepted a predecessor with a live execution")
	}
}
