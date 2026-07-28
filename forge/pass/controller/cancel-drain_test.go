package pass_controller

import (
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
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestProcessStateReplaysCancelAndWaitsForDrain(t *testing.T) {
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

	peerID := tb.Volume.GetPeerID()
	claimID := "pass-controller-test"
	passKey := "test/pass/controller-cancel-drain"
	_, _, err = forge_pass.CreatePassWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		passKey,
		forge_target.NewValueSet(),
		target.CloneVT(),
		1,
		1,
		peerID.String(),
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: peerID.String()}}, true),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, peerID.String())
	executionObject, err := world.MustGetObject(ctx, tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executionObject.ApplyObjectOp(
		ctx,
		execution_tx.NewTxStart(peerID, claimID),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	cancelResult := forge_value.NewResultWithCanceled(errors.New("controller cancellation"))
	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		pass_tx.NewTxCancel(passKey, cancelResult),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	passObject, err := world.MustGetObject(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := NewController(tb.Logger, tb.Bus, NewConfig(tb.EngineID, passKey, peerID, true))
	t.Cleanup(func() {
		if err := ctrl.Close(); err != nil {
			t.Error(err)
		}
	})

	process := func() {
		t.Helper()
		rootRef, rev, err := passObject.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wait, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, passObject, rootRef, rev)
		if err != nil {
			t.Fatal(err)
		}
		if !wait {
			t.Fatal("canceling Pass controller stopped before drain completed")
		}
	}

	process()
	execution, _, err := forge_execution.LookupExecution(ctx, tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want CANCELING", state)
	}
	pass, _, err := forge_pass.LookupPass(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := pass.GetPassState(); state != forge_pass.State_PassState_CANCELING {
		t.Fatalf("pass state = %s, want CANCELING while execution drains", state)
	}

	if _, _, err := executionObject.ApplyObjectOp(
		ctx,
		execution_tx.NewTxComplete(
			forge_value.NewResultWithCanceled(errors.New("drained")),
			&forge_execution.Claim{ClaimId: claimID, Epoch: 1},
		),
		peerID,
	); err != nil {
		t.Fatal(err)
	}
	process()
	pass, _, err = forge_pass.LookupPass(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if state := pass.GetPassState(); state != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("pass state = %s, want COMPLETE after drain", state)
	}
	if !pass.GetResult().GetCanceled() {
		t.Fatal("completed pass lost its canceled result")
	}
}
