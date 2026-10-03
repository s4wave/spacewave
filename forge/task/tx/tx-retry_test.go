package task_tx

import (
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

func TestTxRetryValidateRejectsOutputs(t *testing.T) {
	tx := &TxRetry{NextInputs: &forge_target.ValueSet{
		Outputs: forge_value.ValueSlice{forge_value.NewValue("output")},
	}}
	if err := tx.Validate(); err == nil {
		t.Fatal("retry accepted output values")
	}
}

func TestTxRetryValidateAcceptsNamedInputs(t *testing.T) {
	tx := &TxRetry{NextInputs: &forge_target.ValueSet{
		Inputs: forge_value.ValueSlice{forge_value.NewValueWithWorldObjectSnapshot(
			"continuation",
			&forge_value.WorldObjectSnapshot{Key: "session"},
		)},
	}}
	if err := tx.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSameInputsUsesValueContent(t *testing.T) {
	// Verify separately allocated input values compare by their content.
	left := forge_value.ValueSlice{forge_value.NewValue("continuation")}
	right := forge_value.ValueSlice{forge_value.NewValue("continuation")}
	if !sameInputs(left, right) {
		t.Fatal("equal named inputs did not compare equal")
	}

	// Verify a changed input name makes the value sets differ.
	right[0].Name = "other"
	if sameInputs(left, right) {
		t.Fatal("different named inputs compared equal")
	}
}

func TestTxRetryClearsTerminalTaskResultAndRetainsAttemptHistory(t *testing.T) {
	// Open a World testbed for the retry and its retained attempt history.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Prepare the Task and predecessor Pass with a disabled Target.
	sender := tb.Volume.GetPeerID()
	taskKey := "test/task/retry-result"
	passKey := forge_task.NewPassKey(taskKey, 1)
	target := &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}
	ts := timestamppb.Now()

	// Create the Task for the failed attempt.
	{
		createdObject, _, err := forge_task.CreateTaskWithTarget(ctx, tb.WorldState, sender,
			taskKey, "retry-result", target, "", 1, nil, ts)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Install the Target on the Task before starting its Pass.
	updateTarget := NewTxUpdateInputs(taskKey)
	updateTarget.TxUpdateInputs.UpdateTarget = true
	updateTarget.TxUpdateInputs.ResetInputs = true
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, updateTarget, sender); err != nil {
		t.Fatal(err)
	}

	// Create the predecessor Pass with the Task Target.
	{
		createdObject2, _, err := forge_pass.CreatePassWithTarget(ctx, tb.WorldState, sender,
			passKey, forge_target.NewValueSet(), target.CloneVT(), 1, 1, "", nil, ts)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Link the predecessor Pass to the Task and start its execution.
	if err := tb.WorldState.SetGraphQuad(ctx,
		forge_task.NewTaskToPassQuad(taskKey, passKey, 1)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, pass_tx.NewTxStart(
		passKey, []*pass_tx.ExecSpec{{PeerId: sender.String()}}, true), sender); err != nil {
		t.Fatal(err)
	}

	// Open the predecessor execution for its claim and failure transitions.
	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, sender.String())
	executionObject, err := world.MustGetObject(ctx, tb.WorldState, executionKey)
	defer world.ReleaseObjectState(executionObject)
	if err != nil {
		t.Fatal(err)
	}

	// Claim the predecessor execution for this attempt.
	const claimID = "retry-history"
	if _, _, err := executionObject.ApplyObjectOp(ctx,
		execution_tx.NewTxStart(sender, time.Now().Add(time.Hour), claimID), sender); err != nil {
		t.Fatal(err)
	}

	// Complete the execution with a failure and update the Pass readback.
	failed := forge_value.NewResultWithError(errRetryFixture)
	if _, _, err := executionObject.ApplyObjectOp(ctx, execution_tx.NewTxComplete(
		failed.Clone(), &forge_execution.Claim{ClaimId: claimID, Epoch: 1}), sender); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx,
		pass_tx.NewTxUpdateExecStates(passKey), sender); err != nil {
		t.Fatal(err)
	}

	// Record the failed Task with predecessor outputs to discard on retry.
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, taskKey, true,
		func(bcs *block.Cursor) error {
			// Read the Task before recording the terminal attempt.
			task, err := forge_task.UnmarshalTask(ctx, bcs)
			if err != nil {
				return err
			}

			// Save the failed Task with its predecessor Result and outputs.
			task.TaskState = forge_task.State_TaskState_COMPLETE
			task.PassNonce = 1
			task.Result = failed.Clone()
			task.ValueSet.Outputs = forge_value.ValueSlice{
				forge_value.NewValueWithWorldObjectSnapshot("old-output",
					&forge_value.WorldObjectSnapshot{Key: "old"}),
			}
			bcs.SetBlock(task, true)
			return nil
		}); err != nil {
		t.Fatal(err)
	}

	// Record the predecessor Pass as completed with the execution failure.
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, passKey, true,
		func(bcs *block.Cursor) error {
			// Read the Pass before recording the terminal attempt.
			pass, err := forge_pass.UnmarshalPass(ctx, bcs)
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

	// Retry the failed Task with new continuation inputs.
	nextInputs := &forge_target.ValueSet{Inputs: forge_value.ValueSlice{
		forge_value.NewValueWithWorldObjectSnapshot("continuation",
			&forge_value.WorldObjectSnapshot{Key: "session"}),
	}}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx,
		NewTxRetry(taskKey, 1, nextInputs), sender); err != nil {
		t.Fatalf("retry failed task: %v", err)
	}

	// Read the retried Task to inspect its persisted state.
	task, objectState, err := forge_task.LookupTask(ctx, tb.WorldState, taskKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the retried Task is pending with only the new inputs.
	if err := task.Validate(); err != nil {
		t.Fatalf("retried task is invalid: %v", err)
	}
	if task.GetTaskState() != forge_task.State_TaskState_PENDING {
		t.Fatalf("task state = %s, want PENDING", task.GetTaskState())
	}
	if !task.GetResult().IsEmpty() {
		t.Fatal("terminal task result was retained on pending task")
	}
	if len(task.GetValueSet().GetOutputs()) != 0 {
		t.Fatal("predecessor outputs were copied to retry inputs")
	}
	if !sameInputs(task.GetValueSet().GetInputs(), nextInputs.GetInputs()) {
		t.Fatal("retry inputs were not installed")
	}

	// Verify replaying the same retry leaves the Task valid.
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx,
		NewTxRetry(taskKey, 1, nextInputs), sender); err != nil {
		t.Fatalf("same retry replay: %v", err)
	}

	// Verify replaying the retry with different inputs is rejected.
	differentInputs := &forge_target.ValueSet{Inputs: forge_value.ValueSlice{
		forge_value.NewValueWithWorldObjectSnapshot("other",
			&forge_value.WorldObjectSnapshot{Key: "session-2"}),
	}}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx,
		NewTxRetry(taskKey, 1, differentInputs), sender); err == nil {
		t.Fatal("retry accepted different inputs after idempotent replay")
	}

	// Verify the retry preserves the predecessor Pass and its Result.
	passes, _, passKeys, err := forge_task.CollectTaskPasses(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || passKeys[0] != passKey {
		t.Fatalf("pass history = %v, want predecessor %q", passKeys, passKey)
	}
	if !passes[0].GetResult().Equals(failed) {
		t.Fatal("predecessor result history changed")
	}

	// Verify the retry preserves the predecessor execution Result.
	execution, objectState2, err := forge_execution.LookupExecution(ctx, tb.WorldState, executionKey)
	world.ReleaseObjectState(objectState2)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.GetResult().Equals(failed) {
		t.Fatal("execution result history changed")
	}

	// Start the successor Pass from the pending Task.
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, NewTxStart(taskKey, true), sender); err != nil {
		t.Fatalf("start successor pass: %v", err)
	}

	// Verify the Task is running its second Pass.
	var objectState3 world.ObjectState
	task, objectState3, err = forge_task.LookupTask(ctx, tb.WorldState, taskKey)
	world.ReleaseObjectState(objectState3)
	if err != nil {
		t.Fatal(err)
	}
	if task.GetTaskState() != forge_task.State_TaskState_RUNNING || task.GetPassNonce() != 2 {
		t.Fatalf("successor task = state %s pass %d, want RUNNING pass 2", task.GetTaskState(), task.GetPassNonce())
	}

	// Verify the successor adds a Pass without replacing attempt history.
	passes, _, passKeys, err = forge_task.CollectTaskPasses(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 2 {
		t.Fatalf("pass history length = %d, want 2 (%v)", len(passes), passKeys)
	}

	// Verify the successor Pass starts without predecessor outputs.
	secondPass, _, _, err := forge_task.LookupTaskPass(ctx, tb.WorldState, taskKey, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPass.GetValueSet().GetOutputs()) != 0 {
		t.Fatal("successor pass inherited predecessor outputs")
	}
}

func TestTxRetryRequiresTerminalFailedTask(t *testing.T) {
	tx := &TxRetry{ExpectedPassNonce: 1}
	for _, state := range []forge_task.State{
		forge_task.State_TaskState_RUNNING,
		forge_task.State_TaskState_CHECKING,
	} {
		root := &forge_task.Task{TaskState: state}
		err := tx.ExecuteTx(t.Context(), nil, peer.ID(""), "task", nil, root)
		if err == nil {
			t.Fatalf("retry accepted root state %s", state)
		}
	}
	root := &forge_task.Task{
		TaskState: forge_task.State_TaskState_COMPLETE,
		Result:    forge_value.NewResultWithSuccess(),
	}
	if err := tx.ExecuteTx(t.Context(), nil, peer.ID(""), "task", nil, root); err == nil {
		t.Fatal("retry accepted successful complete task")
	}
}

var errRetryFixture = errors.New("retry fixture failed")
