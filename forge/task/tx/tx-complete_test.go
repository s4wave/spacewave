package task_tx_test

import (
	"errors"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// buildTaskWithPass stands up a Task linked to a single Pass, forcing both into
// the given states so a completion transaction can be applied against them.
func buildTaskWithPass(
	t *testing.T,
	tb *world_testbed.Testbed,
	taskKey string,
	taskState forge_task.State,
	taskResult *forge_value.Result,
	passResult *forge_value.Result,
) peer.ID {
	t.Helper()
	ctx := t.Context()
	sender := tb.Volume.GetPeerID()
	target := &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}
	passKey := forge_task.NewPassKey(taskKey, 1)
	ts := timestamp.Now()
	if _, _, err := forge_task.CreateTaskWithTarget(
		ctx,
		tb.WorldState,
		sender,
		taskKey,
		"tx-complete",
		target,
		"",
		1,
		ts,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_pass.CreatePassWithTarget(
		ctx,
		tb.WorldState,
		sender,
		passKey,
		forge_target.NewValueSet(),
		target.CloneVT(),
		1,
		1,
		"",
		ts,
	); err != nil {
		t.Fatal(err)
	}
	if err := tb.WorldState.SetGraphQuad(
		ctx,
		forge_task.NewTaskToPassQuad(taskKey, passKey, 1),
	); err != nil {
		t.Fatal(err)
	}
	targetUpdate := task_tx.NewTxUpdateInputs(taskKey)
	targetUpdate.TxUpdateInputs.UpdateTarget = true
	targetUpdate.TxUpdateInputs.ResetInputs = true
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, targetUpdate, sender); err != nil {
		t.Fatalf("update task target: %v", err)
	}
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, taskKey, true, func(bcs *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}
		task.TaskState = taskState
		task.Result = taskResult
		task.PassNonce = 1
		bcs.SetBlock(task, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, passKey, true, func(bcs *block.Cursor) error {
		pass, err := forge_pass.UnmarshalPass(ctx, bcs)
		if err != nil {
			return err
		}
		pass.PassState = forge_pass.State_PassState_COMPLETE
		pass.Result = passResult
		bcs.SetBlock(pass, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return sender
}

func TestTxCompleteConvertsFailedPassToFailedTask(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	taskKey := "test/task/complete-failed-pass"
	sender := buildTaskWithPass(
		t, tb, taskKey,
		forge_task.State_TaskState_CHECKING,
		nil,
		forge_value.NewResultWithError(errors.New("pass failed")),
	)

	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		task_tx.NewTxComplete(taskKey, forge_value.NewResultWithSuccess()),
		sender,
	); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	task, _, err := forge_task.LookupTask(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if task.GetTaskState() != forge_task.State_TaskState_COMPLETE {
		t.Fatalf("task state = %s, want COMPLETE", task.GetTaskState())
	}
	if task.GetResult().GetSuccess() {
		t.Fatal("failed pass was recorded as successful task completion")
	}
	if task.GetResult().GetFailError() == "" {
		t.Fatal("failed pass completion did not preserve a failure error")
	}
}

func TestTxCompleteRejectsSuccessOutsideChecking(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	taskKey := "test/task/complete-twice"
	sender := buildTaskWithPass(
		t, tb, taskKey,
		forge_task.State_TaskState_COMPLETE,
		forge_value.NewResultWithSuccess(),
		forge_value.NewResultWithSuccess(),
	)

	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		task_tx.NewTxComplete(taskKey, forge_value.NewResultWithSuccess()),
		sender,
	); err == nil {
		t.Fatal("completing an already complete task was accepted")
	}

	task, _, err := forge_task.LookupTask(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if !task.GetResult().GetSuccess() {
		t.Fatalf("recorded success was overwritten: %s", task.GetResult().GetFailError())
	}
}
