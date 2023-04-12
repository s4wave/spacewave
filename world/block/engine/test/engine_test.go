package world_block_engine_testing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/directive"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// TestWorldEngineController tests constructing the engine controller, looking up
// the engine on the bus, & running some basic queries.
func TestWorldEngineController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := testbed.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test: engine_test.go", []byte(objectStoreID), encKey)
	le.Infof("using encryption key: %s", b58.Encode(encKey))

	nodeStateBucketID := bucketID
	nodeStateTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// initWorldRef is only used if the world has not been previously inited.
	initWorldRef := &bucket.ObjectRef{
		BucketId:      nodeStateBucketID,
		TransformConf: nodeStateTransformConf,
	}

	// initialize world engine
	startEngine := func() (*world_block_engine.Controller, directive.Reference) {
		worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
			ctx,
			tb.Bus,
			world_block_engine.NewConfig(
				engineID,
				volumeID, bucketID,
				objectStoreID,
				initWorldRef,
				nodeStateTransformConf,
			),
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}

	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()

	// provide object op handlers to bus
	opc := world.NewLookupOpController("test-world-engine-ops", engineID, world_mock.LookupMockOp)
	go func() {
		_ = tb.Bus.ExecuteController(ctx, opc)
	}()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	err = world_mock.TestWorldEngine(ctx, le, busEngine)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		le.Infof("world state after test suite: %s", worldState.String())
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// re-mount the world and make sure it still works.
	worldCtrlRef.Release()
	<-time.After(time.Second * 1)

	worldCtrl, worldCtrlRef = startEngine()
	defer worldCtrlRef.Release()
	<-time.After(time.Millisecond * 100)

	eng, err = worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// second test pass
	engTx, err = eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, found, err := engTx.GetObject("test-object")
	if !found && err == nil {
		err = errors.New("object not found after remounting")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// success
}

// TestWorldEngineController_DisableChangelog tests constructing the engine
// controller with the changelog disabled.
func TestWorldEngineController_DisableChangelog(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := testbed.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test: engine_test.go", []byte(objectStoreID), encKey)
	le.Infof("using encryption key: %s", b58.Encode(encKey))

	nodeStateBucketID := bucketID
	nodeStateTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// initWorldRef is only used if the world has not been previously inited.
	initWorldRef := &bucket.ObjectRef{
		BucketId:      nodeStateBucketID,
		TransformConf: nodeStateTransformConf,
	}

	// initialize world engine
	engineConf := world_block_engine.NewConfig(
		engineID,
		volumeID, bucketID,
		objectStoreID,
		initWorldRef,
		nodeStateTransformConf,
	)
	engineConf.DisableChangelog = true
	startEngine := func() (*world_block_engine.Controller, directive.Reference) {
		worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
			ctx,
			tb.Bus,
			engineConf,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}

	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	err = world_mock.TestWorldEngine(ctx, le, busEngine)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		le.Infof("world state after test suite: %s", worldState.String())

		// check if any field other than seqno is set
		lastChange := worldState.GetLastChange().CloneVT()
		lastChange.Seqno = 0
		if lastChange.SizeVT() != 0 || !worldState.GetLastChangeDisable() {
			return errors.New("changelog was not disabled correctly")
		}

		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	worldCtrlRef.Release()
}
