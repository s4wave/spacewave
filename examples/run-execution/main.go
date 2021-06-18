package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"time"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_core "github.com/aperturerobotics/forge/core"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_controller "github.com/aperturerobotics/forge/execution/controller"
	execution_transaction "github.com/aperturerobotics/forge/execution/transaction"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := runExecutionDemo(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runExecutionDemo runs the Execution demo.
func runExecutionDemo(ctx context.Context, le *logrus.Entry) error {
	// read target path
	if len(os.Args) < 2 {
		return errors.New("usage: ./run-execution ./test-target.yaml")
	}

	targetPath := os.Args[1]
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}

	targetData, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return err
	}

	// unmarshal target from yaml into a container for later type resolution
	var tgt target_json.Target
	if err := tgt.UnmarshalYAML(targetData); err != nil {
		return err
	}

	// build storage, etc.
	verbose := false
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(verbose))
	if err != nil {
		return err
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	forge_core.AddFactories(tb.Bus, tb.StaticResolver)

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

	// create Target object
	// resolve from yaml -> protobuf types
	tgtp, err := tgt.ResolveProto(ctx, tb.Bus)
	if err != nil {
		return err
	}
	le.Infof("target resolved to %s", tgtp.GetExec().GetController().GetId())

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
