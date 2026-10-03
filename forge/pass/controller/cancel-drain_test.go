package pass_controller

import (
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
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestProcessStateReplaysCancelAndWaitsForDrain(t *testing.T) {
	// Start a World testbed for Pass cancellation and execution drain.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register target dependencies and resolve the mock execution target.
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))
	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}

	// Create a Pass assigned to the testbed peer.
	peerID := tb.Volume.GetPeerID()
	claimID := "pass-controller-test"
	passKey := "test/pass/controller-cancel-drain"
	var createdObject world.ObjectState
	createdObject, _, err = forge_pass.CreatePassWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		passKey,
		forge_target.NewValueSet(),
		target.CloneVT(),
		1,
		1,
		peerID.String(),
		nil,
		timestamp.Now(),
	)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Start the Pass and create its peer execution.
	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: peerID.String()}}, true),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Acquire the Pass execution and retain it for drain completion.
	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, peerID.String())
	executionObject, err := world.MustGetObject(ctx, tb.WorldState, executionKey)
	defer world.ReleaseObjectState(executionObject)
	if err != nil {
		t.Fatal(err)
	}

	// Start the execution with a claim before canceling the Pass.
	if _, _, err := executionObject.ApplyObjectOp(
		ctx,
		execution_tx.NewTxStart(peerID, time.Now().Add(time.Hour), claimID),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Cancel the Pass while its claimed execution is still running.
	cancelResult := forge_value.NewResultWithCanceled(errors.New("controller cancellation"))
	if _, _, err := tb.WorldState.ApplyWorldOp(
		ctx,
		pass_tx.NewTxCancel(passKey, cancelResult),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Acquire the Pass object for controller reconciliation.
	passObject, err := world.MustGetObject(ctx, tb.WorldState, passKey)
	defer world.ReleaseObjectState(passObject)
	if err != nil {
		t.Fatal(err)
	}

	// Construct the Pass controller and register its cleanup.
	ctrl := NewController(tb.Logger, tb.Bus, NewConfig(tb.EngineID, passKey, peerID, true))
	t.Cleanup(func() {
		if err := ctrl.Close(); err != nil {
			t.Error(err)
		}
	})

	// Prepare a reconciliation action against the current Pass root.
	process := func() {
		// Read the current Pass root and revision for reconciliation.
		t.Helper()
		rootRef, rev, err := passObject.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Reconcile the Pass against its current persisted state.
		wait, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, passObject, rootRef, rev)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the controller keeps watching while the Pass drains.
		if !wait {
			t.Fatal("canceling Pass controller stopped before drain completed")
		}
	}

	// Reconcile the canceled Pass and read its execution state.
	process()
	execution, objectState, err := forge_execution.LookupExecution(ctx, tb.WorldState, executionKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the execution remains canceling until its claim drains.
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("execution state = %s, want CANCELING", state)
	}

	// Read the Pass state after cancellation replay.
	pass, _, err := forge_pass.LookupPass(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Pass remains canceling while its execution drains.
	if state := pass.GetPassState(); state != forge_pass.State_PassState_CANCELING {
		t.Fatalf("pass state = %s, want CANCELING while execution drains", state)
	}

	// Complete the execution with the canceled claim result.
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

	// Reconcile the Pass after its execution finishes draining.
	process()

	// Read the Pass state after drain reconciliation.
	pass, _, err = forge_pass.LookupPass(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Pass completes with its canceled result.
	if state := pass.GetPassState(); state != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("pass state = %s, want COMPLETE after drain", state)
	}
	if !pass.GetResult().GetCanceled() {
		t.Fatal("completed pass lost its canceled result")
	}
}
