package execution_controller

import (
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
)

func TestRestartMidClaimLeavesOneRunnableController(t *testing.T) {
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
	objKey := "test/execution/controller-claim"
	_, err = forge_execution.CreateExecutionWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		objKey,
		peerID,
		forge_target.NewValueSet(),
		target,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	ownerConfig := NewConfig(
		tb.EngineID,
		objKey,
		peerID,
		&forge_target.InputWorld{EngineId: tb.EngineID},
	)
	observerConfig := NewConfig(
		tb.EngineID,
		objKey,
		peerID,
		&forge_target.InputWorld{EngineId: tb.EngineID},
	)
	obj, err := world.MustGetObject(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := obj.ApplyObjectOp(
		ctx,
		execution_tx.NewTxStart(peerID, ownerConfig.GetClaimId()),
		peerID,
	); err != nil {
		t.Fatal(err)
	}

	process := func(ctrl *Controller) {
		t.Helper()
		rootRef, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wait, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, rootRef, rev)
		if err != nil {
			t.Fatal(err)
		}
		if !wait {
			t.Fatal("active execution controller stopped observing")
		}
	}

	owner := NewController(tb.Logger, tb.Bus, ownerConfig)
	process(owner)
	if owner.execRoutine.GetState() == nil {
		t.Fatal("claim owner did not construct a runnable execution")
	}
	owner.execRoutine.SetState(nil)

	observer := NewController(tb.Logger, tb.Bus, observerConfig)
	process(observer)
	if observer.execRoutine.GetState() != nil {
		t.Fatal("non-owner controller constructed a runnable execution")
	}

	restartedOwner := NewController(tb.Logger, tb.Bus, ownerConfig.CloneVT())
	process(restartedOwner)
	if restartedOwner.execRoutine.GetState() == nil {
		t.Fatal("restarted claim owner did not recover runnable execution")
	}
	if owner.execRoutine.GetState() != nil {
		t.Fatal("stopped owner remained runnable after restart")
	}
	if observer.execRoutine.GetState() != nil {
		t.Fatal("restart left more than one runnable controller")
	}
}
