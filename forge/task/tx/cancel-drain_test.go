package task_tx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// custodyFixture owns the World and target used to check cancellation custody.
type custodyFixture struct {
	// ctx bounds fixture operations to the test lifetime.
	ctx context.Context
	// tb supplies the fixture's World and controller factories.
	tb *world_testbed.Testbed
	// peerID identifies the fixture's executor.
	peerID peer.ID
	// claimID identifies the fixture's Execution claimant.
	claimID string
	// target is the resolved target used by fixture Passes.
	target *forge_target.Target
	// ts is the fixture's initial state timestamp.
	ts *timestamp.Timestamp
}

// newCustodyFixture creates the World and target for cancellation tests.
func newCustodyFixture(t *testing.T) *custodyFixture {
	// Start the world testbed and register supporting factories.
	t.Helper()
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register supporting factories on the static resolver.
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	// Resolve the mock target for pass creation.
	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}
	return &custodyFixture{
		ctx:     ctx,
		tb:      tb,
		peerID:  tb.Volume.GetPeerID(),
		claimID: "task-custody-test",
		target:  target,
		ts:      timestamp.Now(),
	}
}

// createRunningPass creates a Pass and starts its claimed Execution.
func (f *custodyFixture) createRunningPass(t *testing.T, passKey string, nonce uint64) string {
	// Create the running pass on the world state.
	t.Helper()
	createdObject, _, err := forge_pass.CreatePassWithTarget(
		f.ctx,
		f.tb.WorldState,
		f.peerID,
		passKey,
		forge_target.NewValueSet(),
		f.target.CloneVT(),
		nonce,
		1,
		f.peerID.String(),
		nil,
		f.ts,
	)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}
	return f.startPassExecution(t, passKey)
}

// startPassExecution starts and claims the fixture peer's Pass Execution.
func (f *custodyFixture) startPassExecution(t *testing.T, passKey string) string {
	// Start the pass with a local execution spec.
	t.Helper()
	_, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: f.peerID.String()}}, true),
		f.peerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Start the execution transaction on the execution object.
	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, f.peerID.String())
	executionObject, err := world.MustGetObject(f.ctx, f.tb.WorldState, executionKey)
	defer world.ReleaseObjectState(executionObject)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxStart(f.peerID, time.Now().Add(time.Hour), f.claimID),
		f.peerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return executionKey
}

// cancelPass records a durable cancellation request for the Pass.
func (f *custodyFixture) cancelPass(t *testing.T, passKey string) *forge_value.Result {
	// Apply the cancel transaction with a canceled result.
	t.Helper()
	result := forge_value.NewResultWithCanceled(errors.New("test cancellation"))
	_, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxCancel(passKey, result),
		f.peerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// cancelExecution cancels an Execution and lends its handle to the caller.
func (f *custodyFixture) cancelExecution(t *testing.T, executionKey string) world.ObjectState {
	// Cancel the execution object and return it for assertions.
	t.Helper()
	executionObject, err := world.MustGetObject(f.ctx, f.tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = executionObject.ApplyObjectOp(f.ctx, execution_tx.NewTxCancel(), f.peerID)
	if err != nil {
		world.ReleaseObjectState(executionObject)
		t.Fatal(err)
	}
	return executionObject
}

// TestPassCancelWaitsForExecutionDrain retains Pass custody until its target drains.
func TestPassCancelWaitsForExecutionDrain(t *testing.T) {
	// Create a running pass and cancel it while the execution drains.
	f := newCustodyFixture(t)
	passKey := "test/pass/cancel-drain"
	executionKey := f.createRunningPass(t, passKey, 1)
	cancelResult := f.cancelPass(t, passKey)

	// Assert completing the pass fails while the execution is running.
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxComplete(passKey, cancelResult),
		f.peerID,
	); err == nil {
		t.Fatal("pass completed while its execution was running")
	}

	// Cancel the execution and update the pass exec states.
	executionObject := f.cancelExecution(t, executionKey)
	defer world.ReleaseObjectState(executionObject)
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxUpdateExecStates(passKey),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	pass, _, err := forge_pass.LookupPass(f.ctx, f.tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := pass.GetPassState(); state != forge_pass.State_PassState_CANCELING {
		t.Fatalf("pass state = %s, want CANCELING", state)
	}

	// Assert a canceling execution rejects a successful terminal result.
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(
			forge_value.NewResultWithSuccess(),
			&forge_execution.Claim{ClaimId: f.claimID, Epoch: 1},
		),
		f.peerID,
	); err == nil {
		t.Fatal("canceling execution accepted a successful terminal result")
	}
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(
			forge_value.NewResultWithCanceled(errors.New("drained")),
			&forge_execution.Claim{ClaimId: f.claimID, Epoch: 1},
		),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxUpdateExecStates(passKey),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the drained pass completed with its canceled result.
	pass, _, err = forge_pass.LookupPass(f.ctx, f.tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := pass.GetPassState(); state != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("pass state = %s, want COMPLETE", state)
	}
	if !pass.GetResult().GetCanceled() {
		t.Fatal("completed pass did not preserve its canceled result")
	}
}

// TestTaskStartDoesNotCreateSuccessorOverLivePass fences a live predecessor.
func TestTaskStartDoesNotCreateSuccessorOverLivePass(t *testing.T) {
	// Create a task whose start must fence on a live predecessor.
	f := newCustodyFixture(t)
	taskKey := "test/task/successor-fence"
	createdObject, _, err := forge_task.CreateTaskWithTarget(
		f.ctx,
		f.tb.WorldState,
		f.peerID,
		taskKey,
		"successor-fence",
		&forge_target.Target{Exec: &forge_target.Exec{Disable: true}},
		f.peerID,
		1,
		nil,
		f.ts,
	)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Configure the task target and reset its inputs.
	updateTarget := task_tx.NewTxUpdateInputs(taskKey)
	updateTarget.TxUpdateInputs.UpdateTarget = true
	updateTarget.TxUpdateInputs.ResetInputs = true
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		updateTarget,
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Create a running predecessor pass and link it to the task.
	passKey := forge_task.NewPassKey(taskKey, 1)
	f.createRunningPass(t, passKey, 1)
	if err := f.tb.WorldState.SetGraphQuad(
		f.ctx,
		forge_task.NewTaskToPassQuad(taskKey, passKey, 1),
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		task_tx.NewTxStart(taskKey, true),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the task stays pending with only the draining predecessor.
	task, objectState, err := forge_task.LookupTask(f.ctx, f.tb.WorldState, taskKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}
	if state := task.GetTaskState(); state != forge_task.State_TaskState_PENDING {
		t.Fatalf("task state = %s, want PENDING while predecessor drains", state)
	}
	passes, _, passKeys, err := forge_task.CollectTaskPasses(f.ctx, f.tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || len(passKeys) != 1 || passKeys[0] != passKey {
		t.Fatalf("passes = %v, want only predecessor %q", passKeys, passKey)
	}
	if state := passes[0].GetPassState(); state != forge_pass.State_PassState_CANCELING {
		t.Fatalf("predecessor state = %s, want CANCELING", state)
	}
}

// TestTaskInputChangeRestartsOnlyAfterDrain retains the old Pass until settlement.
func TestTaskInputChangeRestartsOnlyAfterDrain(t *testing.T) {
	// Create a task whose input change waits for the pass to drain.
	f := newCustodyFixture(t)
	taskKey := "test/task/input-change-drain"
	createdObject, _, err := forge_task.CreateTaskWithTarget(
		f.ctx,
		f.tb.WorldState,
		f.peerID,
		taskKey,
		"input-change-drain",
		f.target.CloneVT(),
		f.peerID,
		1,
		nil,
		f.ts,
	)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Configure the target, reset inputs, and start the task.
	updateTarget := task_tx.NewTxUpdateInputs(taskKey)
	updateTarget.TxUpdateInputs.UpdateTarget = true
	updateTarget.TxUpdateInputs.ResetInputs = true
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		updateTarget,
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		task_tx.NewTxStart(taskKey, true),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Start a pass execution and request an input reset.
	firstPassKey := forge_task.NewPassKey(taskKey, 1)
	executionKey := f.startPassExecution(t, firstPassKey)
	updateInputs := task_tx.NewTxUpdateInputs(taskKey)
	updateInputs.TxUpdateInputs.ResetInputs = true
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		updateInputs,
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the task stays running on the draining predecessor.
	task, objectState, err := forge_task.LookupTask(f.ctx, f.tb.WorldState, taskKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}
	if state := task.GetTaskState(); state != forge_task.State_TaskState_RUNNING {
		t.Fatalf("task state = %s, want RUNNING while pass drains", state)
	}
	passes, _, passKeys, err := forge_task.CollectTaskPasses(f.ctx, f.tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(passes) != 1 || len(passKeys) != 1 || passKeys[0] != firstPassKey {
		t.Fatalf("passes = %v, want only draining predecessor %q", passKeys, firstPassKey)
	}
	if state := passes[0].GetPassState(); state != forge_pass.State_PassState_CANCELING {
		t.Fatalf("predecessor state = %s, want CANCELING", state)
	}

	// Drain the execution and advance the pass and task states.
	executionObject := f.cancelExecution(t, executionKey)
	defer world.ReleaseObjectState(executionObject)
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(
			forge_value.NewResultWithCanceled(errors.New("drained")),
			&forge_execution.Claim{ClaimId: f.claimID, Epoch: 1},
		),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxUpdateExecStates(firstPassKey),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		task_tx.NewTxUpdateWithPassState(taskKey),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		task_tx.NewTxStart(taskKey, true),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the successor pass started after the drain.
	var objectState2 world.ObjectState
	task, objectState2, err = forge_task.LookupTask(f.ctx, f.tb.WorldState, taskKey)
	world.ReleaseObjectState(objectState2)
	if err != nil {
		t.Fatal(err)
	}
	if state := task.GetTaskState(); state != forge_task.State_TaskState_RUNNING {
		t.Fatalf("task state = %s, want RUNNING after successor start", state)
	}
	if nonce := task.GetPassNonce(); nonce != 2 {
		t.Fatalf("pass nonce = %d, want 2", nonce)
	}
	_, _, passKeys, err = forge_task.CollectTaskPasses(f.ctx, f.tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}

	// Assert both predecessor and successor passes are present.
	secondPassKey := forge_task.NewPassKey(taskKey, 2)
	if len(passKeys) != 2 {
		t.Fatalf("passes = %v, want predecessor and successor", passKeys)
	}
	foundFirst, foundSecond := false, false
	for _, passKey := range passKeys {
		foundFirst = foundFirst || passKey == firstPassKey
		foundSecond = foundSecond || passKey == secondPassKey
	}
	if !foundFirst || !foundSecond {
		t.Fatalf("passes = %v, want %q and %q", passKeys, firstPassKey, secondPassKey)
	}
}

// TestCreateExecSpecsPreservesCancelingExecution preserves durable cancellation.
func TestCreateExecSpecsPreservesCancelingExecution(t *testing.T) {
	// Cancel a running execution and recreate its exec specs.
	f := newCustodyFixture(t)
	passKey := "test/pass/preserve-canceling"
	executionKey := f.createRunningPass(t, passKey, 1)
	world.ReleaseObjectState(f.cancelExecution(t, executionKey))

	// Recreate the exec specs for the canceled execution.
	createTx := pass_tx.NewTxCreateExecSpecs(passKey)
	createTx.TxCreateExecSpecs.ExecSpecs = []*pass_tx.ExecSpec{{
		PeerId: f.peerID.String(),
	}}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		createTx,
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the recreated execution preserved its canceling state.
	execution, objectState, err := forge_execution.LookupExecution(
		f.ctx,
		f.tb.WorldState,
		executionKey,
	)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want preserved CANCELING", state)
	}
}

// TestCancelReplayRecoversAfterRestart settles replayed cancellation after drain.
func TestCancelReplayRecoversAfterRestart(t *testing.T) {
	// Replay cancellation requests after a simulated restart.
	f := newCustodyFixture(t)
	passKey := "test/pass/restart-cancel"
	executionKey := f.createRunningPass(t, passKey, 1)
	f.cancelPass(t, passKey)
	executionObject := f.cancelExecution(t, executionKey)
	defer world.ReleaseObjectState(executionObject)

	// A restarted reconciler replays both durable cancellation requests.
	f.cancelPass(t, passKey)
	world.ReleaseObjectState(f.cancelExecution(t, executionKey))

	// Assert the replayed requests left the execution canceling.
	execution, objectState, err := forge_execution.LookupExecution(f.ctx, f.tb.WorldState, executionKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want CANCELING", state)
	}
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(
			forge_value.NewResultWithCanceled(errors.New("drained after restart")),
			&forge_execution.Claim{ClaimId: f.claimID, Epoch: 1},
		),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxUpdateExecStates(passKey),
		f.peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Assert the drained pass completed with its canceled result.
	pass, _, err := forge_pass.LookupPass(f.ctx, f.tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := pass.GetPassState(); state != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("pass state = %s, want COMPLETE", state)
	}
	if !pass.GetResult().GetCanceled() {
		t.Fatal("restart recovery lost the canceled result")
	}
}
