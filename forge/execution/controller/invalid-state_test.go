package execution_controller

import (
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
)

func TestInvalidExecutionWaitsForChange(t *testing.T) {
	// Start a World testbed holding a pending Execution for the mock target.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	// Resolve the mock target and identity used by the pending Execution.
	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}
	peerID := tb.Volume.GetPeerID()
	objKey := "test/execution/controller-invalid"
	_, err = forge_execution.CreateExecutionWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		objKey,
		peerID,
		forge_target.NewValueSet(),
		target,
		nil,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Store a claim without a lease, which fails Execution validation.
	_, _, err = world.AccessWorldObject(ctx, tb.WorldState, objKey, true, func(bcs *block.Cursor) error {
		// Load the stored Execution before writing an invalid claim.
		exState, err := forge_execution.UnmarshalExecution(ctx, bcs)
		if err != nil {
			return err
		}
		exState.ExecutionState = forge_execution.State_ExecutionState_RUNNING
		exState.Claim = &forge_execution.Claim{ClaimId: "stale", Epoch: 1}
		bcs.SetBlock(exState, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reconcile the invalid Execution.
	obj, err := world.MustGetObject(ctx, tb.WorldState, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	rootRef, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conf := NewConfig(tb.EngineID, objKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
	conf.AllowNonExecController = true
	ctrl := NewController(tb.Logger, tb.Bus, conf)

	// Require a wait for the next write rather than an error the loop retries.
	wait, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, rootRef, rev)
	if err != nil {
		t.Fatalf("process invalid execution: %v", err)
	}
	if !wait {
		t.Fatal("controller stopped observing the invalid execution")
	}
	if ctrl.execRoutine.GetState() != nil {
		t.Fatal("controller started the invalid execution")
	}
}
