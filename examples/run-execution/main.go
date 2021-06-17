package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	core_all "github.com/aperturerobotics/hydra/core/all"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/hydra/world/block/engine"
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
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		return err
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	core_all.AddFactories(tb.Bus, tb.StaticResolver)

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

	// construct execution controller & attach to Execution object

	// success
	return nil
}
