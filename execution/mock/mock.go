package execution_mock

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_controller "github.com/aperturerobotics/forge/execution/controller"
	execution_transaction "github.com/aperturerobotics/forge/execution/transaction"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	hydracore "github.com/aperturerobotics/hydra/core"
	hydra_all "github.com/aperturerobotics/hydra/core/all"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

// RunTargetInTestbed runs a target in an ephemeral testbed.
func RunTargetInTestbed(
	ctx context.Context,
	le *logrus.Entry,
	tgt *target_json.Target,
	addFactories func(b bus.Bus, sr *static.Resolver),
	testbedOpts ...testbed.Option,
) error {
	// build storage, etc.
	tb, err := testbed.NewTestbed(ctx, le, testbedOpts...)
	if err != nil {
		return err
	}
	defer tb.Release()

	b := tb.Bus
	sr := tb.StaticResolver
	hydracore.AddFactories(b, sr)
	hydra_all.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(execution_controller.NewFactory(b))
	if addFactories != nil {
		addFactories(b, sr)
	}

	// create Target object
	// resolve from yaml -> protobuf types
	tgtp, err := tgt.ResolveProto(ctx, tb.Bus)
	if err != nil {
		return err
	}
	le.Infof("target resolved to %s", tgtp.GetExec().GetController().GetId())

	// construct & mount world controller
	engineID := "forge-1"
	volumeID := tb.Volume.GetID()
	bucketID := testbed.BucketId
	objectStoreID := "forge"
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			nil,
		),
	)
	if err != nil {
		return err
	}
	defer worldCtrlRef.Release()

	wh, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return err
	}
	defer wh.Release()

	// create cursor to manage world objects
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		return err
	}
	defer cursor.Release()

	// write the Target to a block
	btx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(tgtp, true)
	var tgtRef *block.BlockRef
	tgtRef, bcs, err = btx.Write(true)
	if err != nil {
		return err
	}

	// use a wrapper to automatically create / commit txs
	worldState := world.NewEngineWorldState(wh.GetContext(), wh, true)

	// create test-object
	targetObjectID := "targets/1"
	targetObjState, err := worldState.CreateObject(targetObjectID, &bucket.ObjectRef{
		RootRef: tgtRef,
	})
	if err != nil {
		return err
	}

	targetObjState, err = world.MustGetObject(worldState, targetObjectID)
	if err != nil {
		return err
	}
	rootRef, _, err := targetObjState.GetRootRef()
	if err != nil {
		return err
	}

	btx, bcs = cursor.BuildTransactionAtRef(nil, rootRef.GetRootRef())
	tgtb, err := bcs.Unmarshal(forge_target.NewTargetBlock)
	if err != nil {
		return err
	}
	tgtp = tgtb.(*forge_target.Target)

	le.Infof("successfully stored and read back target from world: %s", tgtp.String())

	// create the Execution object in the world
	// TODO: use the execution_creator package + execution Spec
	executionObjectID := "execution/1"

	// construct execution controller & attach to Execution object
	peerID := tb.Volume.GetPeerID()
	execCtrlCfg := execution_controller.NewConfig(
		engineID,
		executionObjectID,
		peerID,
	)
	execCtrlCfg.AllowNonExecController = true
	execCtrl, execCtrlRef, err := execution_controller.StartControllerWithConfig(
		ctx,
		tb.Bus,
		execCtrlCfg,
	)
	if err != nil {
		return err
	}
	defer execCtrlRef.Release()
	_ = execCtrl

	// add object type handlers to bus
	opc := world.NewOperationController(
		"test-world-engine-ops",
		engineID, "",
		nil,
		[]world.ApplyObjectOpFunc{
			// execution object: apply a transaction
			execution_transaction.ApplyObjectOp,
		},
	)
	go tb.Bus.ExecuteController(ctx, opc)
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// write the initial execstate to a block
	btx, bcs = cursor.BuildTransaction(nil)
	bcs.SetBlock(&forge_execution.Execution{
		ExecutionState: forge_execution.State_ExecutionState_PENDING,
		PeerId:         peerID.Pretty(),
		TargetRef:      tgtRef,
	}, true)
	var execRef *block.BlockRef
	execRef, bcs, err = btx.Write(true)
	if err != nil {
		return err
	}

	// create execution object (note: this can be done after starting the controller)
	_, err = worldState.CreateObject(executionObjectID, &bucket.ObjectRef{RootRef: execRef})
	if err != nil {
		return err
	}

	// wait for execution to complete
	res, err := forge_execution.WaitExecutionComplete(
		ctx,
		le.WithField("control-loop", "run-execution-wait-complete"),
		wh,
		executionObjectID,
	)
	if err != nil {
		return err
	}
	if errStr := res.FailError; len(errStr) != 0 {
		return errors.New(errStr)
	}

	// success
	return nil
}
