package execution_mock

import (
	"errors"
	"time"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_controller "github.com/aperturerobotics/forge/execution/controller"
	execution_transaction "github.com/aperturerobotics/forge/execution/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	hydracore "github.com/aperturerobotics/hydra/core"
	hydra_all "github.com/aperturerobotics/hydra/core/all"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
)

// RunTargetOpts are optional options for RunTargetInTestbed.
type RunTargetOpts struct {
	// PreHook is called just before running tests.
	PreHook func(state world.WorldState) error
	// PostHook is called just after running tests.
	PostHook func(state world.WorldState, exec *forge_execution.Execution) error
	// EngineId overrides the engine id.
	// Default is "forge-run-mock-target
	EngineId string
	// ObjectStoreId overrides the object store id.
	ObjectStoreId string
	// VolumeId overrides the volume id.
	VolumeId string
	// BucketId overrides the bucket id.
	BucketId string
}

// RunTargetInTestbed runs a target in an ephemeral testbed.
func RunTargetInTestbed(
	tb *testbed.Testbed,
	tgt *target_json.Target,
	valueSet *forge_target.ValueSet,
	opts *RunTargetOpts,
) (*forge_execution.Execution, error) {
	ctx := tb.Context
	le := tb.Logger
	b := tb.Bus
	sr := tb.StaticResolver
	hydracore.AddFactories(b, sr)
	hydra_all.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(execution_controller.NewFactory(b))

	// create Target object
	// resolve from yaml -> protobuf types
	tgtp, err := tgt.ResolveProto(ctx, tb.Bus)
	if err != nil {
		return nil, err
	}
	le.Infof("target resolved to %s", tgtp.GetExec().GetController().GetId())

	// construct & mount world controller
	engineID := "forge-run-mock-target"
	if opts != nil && opts.EngineId != "" {
		engineID = opts.EngineId
	}
	volumeID := tb.Volume.GetID()
	if opts != nil && opts.VolumeId != "" {
		volumeID = opts.VolumeId
	}
	bucketID := testbed.BucketId
	if opts != nil && opts.BucketId != "" {
		bucketID = opts.BucketId
	}
	objectStoreID := "forge-run-mock-target"
	if opts != nil && opts.ObjectStoreId != "" {
		objectStoreID = opts.ObjectStoreId
	}
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
		return nil, err
	}
	defer worldCtrlRef.Release()

	wh, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	defer wh.Release()

	// create cursor to manage world objects
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		return nil, err
	}
	defer cursor.Release()

	// write the Target to a block
	btx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(tgtp, true)
	var tgtRef *block.BlockRef
	tgtRef, bcs, err = btx.Write(true)
	if err != nil {
		return nil, err
	}

	// use a wrapper to automatically create / commit txs
	worldState := world.NewEngineWorldState(wh.GetContext(), wh, true)
	if opts != nil && opts.PreHook != nil {
		if err := opts.PreHook(worldState); err != nil {
			return nil, err
		}
	}

	// create test-object
	targetObjectID := "targets/1"
	targetObjState, err := worldState.CreateObject(targetObjectID, &bucket.ObjectRef{
		RootRef: tgtRef,
	})
	if err != nil {
		return nil, err
	}

	targetObjState, err = world.MustGetObject(worldState, targetObjectID)
	if err != nil {
		return nil, err
	}
	rootRef, _, err := targetObjState.GetRootRef()
	if err != nil {
		return nil, err
	}

	btx, bcs = cursor.BuildTransactionAtRef(nil, rootRef.GetRootRef())
	tgtb, err := bcs.Unmarshal(forge_target.NewTargetBlock)
	if err != nil {
		return nil, err
	}
	tgtp = tgtb.(*forge_target.Target)

	// create the Execution object in the world
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
		return nil, err
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
		ValueSet:       valueSet,
	}, true)
	var execRef *block.BlockRef
	execRef, bcs, err = btx.Write(true)
	if err != nil {
		return nil, err
	}

	// create execution object (note: this can be done after starting the controller)
	_, err = worldState.CreateObject(executionObjectID, &bucket.ObjectRef{RootRef: execRef})
	if err != nil {
		return nil, err
	}

	// wait for execution to complete
	finalState, err := forge_execution.WaitExecutionComplete(
		ctx,
		le.WithField("control-loop", "run-execution-wait-complete"),
		wh,
		executionObjectID,
	)
	if err != nil {
		return nil, err
	}

	res := finalState.GetResult()
	if errStr := res.FailError; len(errStr) != 0 {
		return finalState, errors.New(errStr)
	}
	// success
	if opts != nil && opts.PostHook != nil {
		if err := opts.PostHook(worldState, finalState); err != nil {
			return nil, err
		}
	}
	return finalState, nil
}
