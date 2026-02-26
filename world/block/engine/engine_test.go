package world_block_engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
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
	"github.com/aperturerobotics/util/ccontainer"
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

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := tb.BucketId

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
		engineConf := world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			initWorldRef,
			nodeStateTransformConf,
			true,
		)
		// engineConf.Verbose = true
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
	engTx, err := eng.NewTransaction(ctx, true)
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
		wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		_ = worldState
		// le.Infof("world state after test suite: %s", worldState.String())
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
	engTx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, found, err := engTx.GetObject(ctx, "test-object")
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

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := tb.BucketId

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
		false,
	)
	// engineConf.Verbose = true
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
	engTx, err := eng.NewTransaction(ctx, true)
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
		wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		// le.Infof("world state after test suite: %s", worldState.String())
		_ = worldState

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

// TestWorldEngineWatchReload tests watching for changes on a WorldEngine that fully reloads with a new version.
// This is a regression test.
func TestWorldEngineWatchReload(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Setup a cursor pointing to the volume and bucket.
	b, le, vol, bucketID := tb.Bus, tb.Logger, tb.Volume, tb.BucketId
	bls, objRef, err := bucket_lookup.BuildEmptyCursor(ctx, b, le, tb.StepFactorySet, bucketID, vol.GetID(), nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer bls.Release()

	// Build the initial world state.
	if err := func() error {
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(world_block.NewWorld(false), true)
		nroot, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}
		objRef.RootRef = nroot
		return nil
	}(); err != nil {
		t.Fatal(err.Error())
	}

	le.Infof("got world root ref after initial state: %v", objRef.MarshalB58())

	// Start a world engine controller with that state.
	engineID := "engine/test"
	initWorldEngConf := &world_block_engine.Config{
		EngineId:    engineID,
		BucketId:    bucketID,
		VolumeId:    vol.GetID(),
		InitHeadRef: objRef.Clone(),
	}
	initConfigSet := configset.ConfigSet{
		engineID: configset.NewControllerConfig(1, initWorldEngConf),
	}
	_, initConfigSetRef, err := b.AddDirective(configset.NewApplyConfigSet(initConfigSet), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer initConfigSetRef.Release()

	// Start a new routine which watches the world seqno.
	//
	// We expect the seqno to increase, first when we write to the world, second when we restart the controller with a different head ref.
	currSeqno := ccontainer.NewCContainer(uint64(0))
	errCh := make(chan error, 1)
	go func() {
		busEngine := world.NewBusEngine(ctx, b, engineID)
		ws := world.NewEngineWorldState(busEngine, true)

		for {
			seqno, err := ws.GetSeqno(ctx)
			if err != nil {
				errCh <- err
				return
			}

			le.Debugf("observed world seqno: %v", seqno)
			currSeqno.SetValue(seqno)
			_, err = ws.WaitSeqno(ctx, seqno+1)
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Write to the world via the controller.
	objKey := "test-object"
	if err := func() error {
		worldEng, _, worldEngRef, err := world.ExLookupWorldEngine(ctx, b, false, engineID, nil)
		if err != nil {
			return err
		}
		defer worldEngRef.Release()

		return world.ExecTransaction(ctx, worldEng, true, func(ctx context.Context, wtx world.WorldState) error {
			_, _, err := world.CreateWorldObject(ctx, wtx, objKey, func(bcs *block.Cursor) error {
				_, err := blob.BuildBlobWithBytes(ctx, []byte("Hello world"), bcs)
				return err
			})
			return err
		})
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// Expect the seqno to be > 0
	firstWriteSeqno, err := currSeqno.WaitValueWithValidator(ctx, func(v uint64) (bool, error) {
		return v > 0, nil
	}, errCh)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("got sequence number after first write: %v", firstWriteSeqno)

	// Now we will modify the world state without telling the controller,
	// Then apply a configset with a higher revision for that controller ID.
	// This will shut down the world engine controller and start a new one.
	// Hopefully the BusEngine above will retrieve this new engine handle.

	// Retrieve the current object ref from the world engine.
	var worldObjRefFirstWrite *bucket.ObjectRef
	if err := func() error {
		worldEng, _, worldEngRef, err := world.ExLookupWorldEngine(ctx, b, false, engineID, nil)
		if err != nil {
			return err
		}
		defer worldEngRef.Release()

		return worldEng.AccessWorldState(ctx, nil, func(rootBls *bucket_lookup.Cursor) error {
			worldObjRefFirstWrite = rootBls.GetRef()
			return nil
		})
	}(); err != nil {
		t.Fatal(err.Error())
	}
	if err := worldObjRefFirstWrite.Validate(); err != nil {
		t.Fatal(err.Error())
	}

	// Modify the world engine state
	objRef.RootRef = worldObjRefFirstWrite.RootRef.Clone()
	rootRefFirstWrite := objRef.CloneVT()
	le.Infof("got world root ref after first write: %v", rootRefFirstWrite.MarshalB58())

	// Access
	var rootRefSecondWrite *block.BlockRef
	if err := func() error {
		btx, bcs := bls.BuildTransactionAtRef(nil, worldObjRefFirstWrite.RootRef.Clone())
		blk, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			return err
		}

		wblk := blk.(*world_block.World)
		wblk.LastChange.Seqno = 100
		bcs.MarkDirty()

		nref, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}

		rootRefSecondWrite = nref
		return nil
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// Restart the world engine controller with updated state.
	updHeadRef := objRef.Clone()
	updHeadRef.RootRef = rootRefSecondWrite
	le.Infof("got world root ref after second write: %v", updHeadRef.MarshalB58())
	if updHeadRef.EqualVT(rootRefFirstWrite) {
		t.Fatal("expected refs to change")
	}

	updWorldEngConf := &world_block_engine.Config{
		EngineId:    engineID,
		BucketId:    bucketID,
		VolumeId:    vol.GetID(),
		InitHeadRef: updHeadRef,
	}
	updConfigSet := configset.ConfigSet{
		engineID: configset.NewControllerConfig(2, updWorldEngConf),
	}
	_, updConfigSetRef, err := b.AddDirective(configset.NewApplyConfigSet(updConfigSet), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer updConfigSetRef.Release()

	// Expect that the world seqno update will be observed.
	finalWriteSeqno, err := currSeqno.WaitValueWithValidator(ctx, func(v uint64) (bool, error) {
		return v >= 100, nil
	}, errCh)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("got sequence number after second write: %v", finalWriteSeqno)
}
