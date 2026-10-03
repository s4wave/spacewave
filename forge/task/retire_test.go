package forge_task_test

import (
	"slices"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_world "github.com/s4wave/spacewave/forge/world"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
)

// TestRetireTaskPreservesDrainAndHistory retires a complete Task while its
// Execution is still draining, then detaches the Execution at completion.
func TestRetireTaskPreservesDrainAndHistory(t *testing.T) {
	// Create one assigned Task and its complete Pass with a pending Execution.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	sender := tb.Volume.GetPeerID()
	const taskKey, passKey, executionKey, siblingKey = "task/retire", "pass/retire", "execution/retire", "task/live"
	target := &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}
	for _, key := range []string{taskKey, siblingKey} {
		object, _, err := forge_task.CreateTaskWithTarget(ctx, tb.WorldState, sender, key, "test", target, sender, 1, nil, timestamp.Now())
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create an assigned Pass and its unfinished Execution.
	passObject, _, err := forge_pass.CreatePassWithTarget(ctx, tb.WorldState, sender, passKey, nil, target, 1, 1, sender.String(), nil, timestamp.Now())
	world.ReleaseObjectState(passObject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, sender, executionKey, sender, nil, target, nil, timestamp.Now()); err != nil {
		t.Fatal(err)
	}

	// Retain the Task and attempt graph while writing terminal result bodies.
	for _, link := range []world.GraphQuad{forge_task.NewTaskToPassQuad(taskKey, passKey, 1), forge_pass.NewPassToExecutionQuad(passKey, executionKey)} {
		if err := tb.WorldState.SetGraphQuad(ctx, link); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{taskKey, passKey} {
		if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, key, true, func(cursor *block.Cursor) error {
			// Store terminal bodies without changing their original peer assignment.
			if key == taskKey {
				body, err := forge_task.UnmarshalTask(ctx, cursor)
				if err != nil {
					return err
				}
				body.TaskState = forge_task.State_TaskState_COMPLETE
				body.PassNonce = 1
				body.Result = forge_value.NewResultWithError(errors.New("task failed"))
				cursor.SetBlock(body, true)
				return nil
			}
			body, err := forge_pass.UnmarshalPass(ctx, cursor)
			if err != nil {
				return err
			}
			body.PassState = forge_pass.State_PassState_COMPLETE
			body.Result = forge_value.NewResultWithError(errors.New("pass failed"))
			cursor.SetBlock(body, true)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Reject another sender before any retirement edges or assignments change.
	other, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := forge_task.RetireTask(ctx, tb.WorldState, taskKey, other.GetPeerID()); err == nil {
		t.Fatal("foreign sender retired assigned work")
	}
	keypairKey := identity_world.NewKeypairKey(sender.String())
	assigned, err := forge_world.ListKeypairObjects(ctx, tb.WorldState, keypairKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 4 {
		t.Fatalf("rejected retirement changed assignments: %v", assigned)
	}

	// Retire twice: only the pending Execution and sibling retain worker demand.
	for range 2 {
		if err := forge_task.RetireTask(ctx, tb.WorldState, taskKey, sender); err != nil {
			t.Fatal(err)
		}
	}
	assigned, err = forge_world.ListKeypairObjects(ctx, tb.WorldState, keypairKey)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(assigned, []string{executionKey, siblingKey}) {
		t.Fatalf("draining assignments = %v", assigned)
	}

	// Write the final Execution result before releasing its retirement demand.
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, executionKey, true, func(cursor *block.Cursor) error {
		// Preserve the Execution's assigned peer while storing its final result.
		body, err := forge_execution.UnmarshalExecution(ctx, cursor)
		if err != nil {
			return err
		}
		body.ExecutionState = forge_execution.State_ExecutionState_COMPLETE
		body.Result = forge_value.NewResultWithSuccess()
		cursor.SetBlock(body, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if retired, err := forge_task.ReconcileRetiredObject(ctx, tb.WorldState, executionKey); err != nil || !retired {
		t.Fatalf("completion retirement = %v, %v", retired, err)
	}
	assigned, err = forge_world.ListKeypairObjects(ctx, tb.WorldState, keypairKey)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(assigned, []string{siblingKey}) {
		t.Fatalf("settled assignments = %v", assigned)
	}

	// Historical results, peer authorization and Task-to-Execution edges survive.
	passes, err := forge_task.ListTaskPasses(ctx, tb.WorldState, taskKey)
	if err != nil || !slices.Equal(passes, []string{passKey}) {
		t.Fatalf("Pass history = %v, %v", passes, err)
	}
	executions, err := forge_pass.ListPassExecutions(ctx, tb.WorldState, passKey)
	if err != nil || !slices.Equal(executions, []string{executionKey}) {
		t.Fatalf("Execution history = %v, %v", executions, err)
	}
	body, err := world.LookupObjectBody[*forge_execution.Execution](ctx, tb.WorldState, executionKey, forge_execution.NewExecutionBlock)
	if err != nil {
		t.Fatal(err)
	}
	if !body.GetResult().GetSuccess() || body.GetPeerId() != sender.String() {
		t.Fatal("retirement changed historical result or peer")
	}
	if err := body.CheckPeerID(other.GetPeerID()); err == nil {
		t.Fatal("retirement opened historical Execution authorization")
	}

	// Reactivation restores only the Task, leaving its settled attempts retired.
	if _, err := task_tx.NewTxRetry(taskKey, 1, nil).ApplyWorldOp(ctx, tb.Logger, tb.WorldState, sender); err != nil {
		t.Fatal(err)
	}
	assigned, err = forge_world.ListKeypairObjects(ctx, tb.WorldState, keypairKey)
	if err != nil || !slices.Equal(assigned, []string{siblingKey, taskKey}) {
		t.Fatalf("reactivated assignments = %v, %v", assigned, err)
	}
}
