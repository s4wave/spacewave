package task_tx_test

import (
	"context"
	"errors"
	"testing"

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

type custodyFixture struct {
	ctx    context.Context
	tb     *world_testbed.Testbed
	peerID peer.ID
	target *forge_target.Target
	ts     *timestamp.Timestamp
}

func newCustodyFixture(t *testing.T) *custodyFixture {
	t.Helper()

	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}
	return &custodyFixture{
		ctx:    ctx,
		tb:     tb,
		peerID: tb.Volume.GetPeerID(),
		target: target,
		ts:     timestamp.Now(),
	}
}

func (f *custodyFixture) createRunningPass(t *testing.T, passKey string, nonce uint64) string {
	t.Helper()

	_, _, err := forge_pass.CreatePassWithTarget(
		f.ctx,
		f.tb.WorldState,
		f.peerID,
		passKey,
		forge_target.NewValueSet(),
		f.target.CloneVT(),
		nonce,
		1,
		f.peerID.String(),
		f.ts,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: f.peerID.String()}}, true),
		f.peerID,
	)
	if err != nil {
		t.Fatal(err)
	}

	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, f.peerID.String())
	executionObject, err := world.MustGetObject(f.ctx, f.tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxStart(f.peerID),
		f.peerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return executionKey
}

func (f *custodyFixture) cancelPass(t *testing.T, passKey string) *forge_value.Result {
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

func (f *custodyFixture) cancelExecution(t *testing.T, executionKey string) world.ObjectState {
	t.Helper()

	executionObject, err := world.MustGetObject(f.ctx, f.tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = executionObject.ApplyObjectOp(f.ctx, execution_tx.NewTxCancel(), f.peerID)
	if err != nil {
		t.Fatal(err)
	}
	return executionObject
}

func TestPassCancelWaitsForExecutionDrain(t *testing.T) {
	f := newCustodyFixture(t)
	passKey := "test/pass/cancel-drain"
	executionKey := f.createRunningPass(t, passKey, 1)
	cancelResult := f.cancelPass(t, passKey)

	if _, _, err := f.tb.WorldState.ApplyWorldOp(
		f.ctx,
		pass_tx.NewTxComplete(passKey, cancelResult),
		f.peerID,
	); err == nil {
		t.Fatal("pass completed while its execution was running")
	}

	executionObject := f.cancelExecution(t, executionKey)
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

	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(forge_value.NewResultWithSuccess()),
		f.peerID,
	); err == nil {
		t.Fatal("canceling execution accepted a successful terminal result")
	}
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(forge_value.NewResultWithCanceled(errors.New("drained"))),
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

func TestTaskStartDoesNotCreateSuccessorOverLivePass(t *testing.T) {
	f := newCustodyFixture(t)
	taskKey := "test/task/successor-fence"
	_, _, err := forge_task.CreateTaskWithTarget(
		f.ctx,
		f.tb.WorldState,
		f.peerID,
		taskKey,
		"successor-fence",
		&forge_target.Target{Exec: &forge_target.Exec{Disable: true}},
		f.peerID,
		1,
		f.ts,
	)
	if err != nil {
		t.Fatal(err)
	}

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

	task, _, err := forge_task.LookupTask(f.ctx, f.tb.WorldState, taskKey)
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

func TestCreateExecSpecsPreservesCancelingExecution(t *testing.T) {
	f := newCustodyFixture(t)
	passKey := "test/pass/preserve-canceling"
	executionKey := f.createRunningPass(t, passKey, 1)
	f.cancelExecution(t, executionKey)

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

	execution, _, err := forge_execution.LookupExecution(
		f.ctx,
		f.tb.WorldState,
		executionKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want preserved CANCELING", state)
	}
}

func TestCancelReplayRecoversAfterRestart(t *testing.T) {
	f := newCustodyFixture(t)
	passKey := "test/pass/restart-cancel"
	executionKey := f.createRunningPass(t, passKey, 1)
	f.cancelPass(t, passKey)
	executionObject := f.cancelExecution(t, executionKey)

	// A restarted reconciler replays both durable cancellation requests.
	f.cancelPass(t, passKey)
	f.cancelExecution(t, executionKey)

	execution, _, err := forge_execution.LookupExecution(f.ctx, f.tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want CANCELING", state)
	}
	if _, _, err := executionObject.ApplyObjectOp(
		f.ctx,
		execution_tx.NewTxComplete(forge_value.NewResultWithCanceled(errors.New("drained after restart"))),
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
