package world_block_engine_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
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

	err = eng.AccessWorldState(ctx, nil, func(bls *world.WorldAccess) error {

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

func TestWorldEngineControllerFallsBackWhenCoordinatorUnsupported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	kvtxVolume, ok := tb.Volume.(*common_kvtx.Volume)
	if !ok {
		t.Fatalf("testbed volume type = %T, want *common_kvtx.Volume", tb.Volume)
	}
	kvtxVolume.Coordinator = coord.NewUnsupportedCoordinator(
		coord.BackendKindRPC,
		coord.FallbackReasonUnsupported,
	)

	transformConf, err := block_transform.NewConfig(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	engineConf := world_block_engine.NewConfig(
		"test-world-engine-unsupported-coordinator",
		tb.Volume.GetID(),
		tb.BucketId,
		"test-world-engine-unsupported-coordinator-store",
		&bucket.ObjectRef{
			BucketId:      tb.BucketId,
			TransformConf: transformConf,
		},
		nil,
		false,
	)
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf)
	if err != nil {
		t.Fatalf("start world engine with unsupported coordinator: %v", err)
	}
	defer worldCtrlRef.Release()

	engine, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatalf("get world engine: %v", err)
	}
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new write transaction with unsupported coordinator: %v", err)
	}
	if _, err := tx.CreateObject(ctx, "unsupported-coordinator-fallback", nil); err != nil {
		tx.Discard()
		t.Fatalf("create object with unsupported coordinator: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit with unsupported coordinator: %v", err)
	}
}

func TestWorldEngineControllerCoordinatorHeadWatch(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	boltPath := filepath.Join(t.TempDir(), "world-head-watch.bolt")
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_bolt.Config{Path: boltPath}))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	volumeID := tb.Volume.GetID()
	objectStoreID := "test-world-engine-head-watch-store"
	bucketID := tb.BucketId
	transformConf, err := block_transform.NewConfig(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	initWorldRef := &bucket.ObjectRef{
		BucketId:      bucketID,
		TransformConf: transformConf,
	}

	startEngine := func(engineID string) (*world_block_engine.Controller, directive.Reference) {
		engineConf := world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			initWorldRef,
			nil,
			false,
		)
		worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf)
		if err != nil {
			t.Fatal(err.Error())
		}
		if _, err := worldCtrl.GetWorldEngine(ctx); err != nil {
			worldCtrlRef.Release()
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}

	writerCtrl, writerRef := startEngine("test-world-engine-head-watch-writer")
	defer writerRef.Release()
	readerCtrl, readerRef := startEngine("test-world-engine-head-watch-reader")
	defer readerRef.Release()

	writerEngine, err := writerCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	readerEngine, err := readerCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	externalLease, err := tb.Volume.WaitAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      volumeID,
		ObjectStoreID: objectStoreID,
		ParticipantID: "external-writer",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	blockedTx := make(chan world.Tx, 1)
	blockedErr := make(chan error, 1)
	go func() {
		tx, err := writerEngine.NewTransaction(ctx, true)
		if err != nil {
			blockedErr <- err
			return
		}
		blockedTx <- tx
	}()
	select {
	case err := <-blockedErr:
		t.Fatalf("writer transaction failed while waiting for lease: %v", err)
	case tx := <-blockedTx:
		tx.Discard()
		t.Fatal("writer transaction acquired while external coordinator lease was held")
	case <-time.After(50 * time.Millisecond):
	}
	if err := externalLease.Release(ctx); err != nil {
		t.Fatal(err.Error())
	}
	select {
	case err := <-blockedErr:
		t.Fatalf("writer transaction failed after lease release: %v", err)
	case tx := <-blockedTx:
		tx.Discard()
	case <-time.After(5 * time.Second):
		t.Fatal("writer transaction did not acquire after external lease release")
	}

	writeRawHead := func(ref *bucket.ObjectRef) {
		storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objectStoreID, volumeID, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer storeRef.Release()
		ktx, err := storeVal.GetObjectStore().NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer ktx.Discard()
		data, err := (&world_block_engine.HeadState{HeadRef: ref}).MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := ktx.Set(ctx, []byte("world-head"), data); err != nil {
			t.Fatal(err.Error())
		}
		if err := ktx.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
	}
	baseHead := writerEngine.(*world_block.Engine).GetRootRef()
	staleTx, err := writerEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := staleTx.CreateObject(ctx, "coordinator-stale-head-object", nil); err != nil {
		staleTx.Discard()
		t.Fatal(err.Error())
	}
	writeRawHead(&bucket.ObjectRef{BucketId: bucketID})
	if err := staleTx.Commit(ctx); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatalf("stale head commit error = %v, want ErrStaleGeneration", err)
	}
	writeRawHead(baseHead)

	watchScope := coord.Scope{
		VolumeID:      volumeID,
		ObjectStoreID: objectStoreID,
		ParticipantID: "watcher",
	}
	capability, err := tb.Volume.Capability(ctx, watchScope)
	if err != nil {
		t.Fatal(err.Error())
	}
	watch, err := tb.Volume.Watch(ctx, watchScope, capability.Generation)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer watch.Close()

	tx, err := writerEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := tx.CreateObject(ctx, "coordinator-head-watch-object", nil); err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	acceptedRoot := writerEngine.(*world_block.Engine).GetRootRef()
	if acceptedRoot.Clone() == nil {
		t.Fatalf("accepted root clone was nil: %#v", acceptedRoot)
	}
	publishedSnapshot, err := tb.Volume.Snapshot(ctx, watchScope)
	if err != nil {
		t.Fatal(err.Error())
	}
	if publishedSnapshot.Root == nil || !publishedSnapshot.Root.EqualsRef(acceptedRoot) {
		t.Fatalf("published coordinator root = %#v, want %#v", publishedSnapshot.Root, acceptedRoot)
	}

	foundPublish := false
	var seenEvents []coord.Event
	publishCtx, publishCancel := context.WithTimeout(ctx, 5*time.Second)
	defer publishCancel()
	for !foundPublish {
		select {
		case <-publishCtx.Done():
			t.Fatalf("coordinator watch did not observe accepted world root publication; events=%+v", seenEvents)
		case event, ok := <-watch.Events():
			if !ok {
				t.Fatal("coordinator watch closed before accepted world root publication")
			}
			seenEvents = append(seenEvents, event)
			foundPublish = event.RootChanged != nil &&
				event.RootChanged.EqualsRef(acceptedRoot) &&
				string(event.KeyPrefixChanged) == "world-head"
		}
	}

	firstWriterTx, err := writerEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := firstWriterTx.CreateObject(ctx, "coordinator-serialized-writer-a", nil); err != nil {
		firstWriterTx.Discard()
		t.Fatal(err.Error())
	}
	secondWriterTx := make(chan world.Tx, 1)
	secondWriterErr := make(chan error, 1)
	go func() {
		tx, err := readerEngine.NewTransaction(ctx, true)
		if err != nil {
			secondWriterErr <- err
			return
		}
		secondWriterTx <- tx
	}()
	select {
	case err := <-secondWriterErr:
		t.Fatalf("second writer failed while waiting for peer lease: %v", err)
	case tx := <-secondWriterTx:
		tx.Discard()
		t.Fatal("second writer acquired while first standalone writer held lease")
	case <-time.After(50 * time.Millisecond):
	}
	if err := firstWriterTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	var secondTx world.Tx
	select {
	case err := <-secondWriterErr:
		t.Fatalf("second writer failed after first writer commit: %v", err)
	case secondTx = <-secondWriterTx:
	case <-time.After(5 * time.Second):
		t.Fatal("second writer did not acquire after first writer commit")
	}
	if _, err := secondTx.CreateObject(ctx, "coordinator-serialized-writer-b", nil); err != nil {
		secondTx.Discard()
		t.Fatal(err.Error())
	}
	if err := secondTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		rtx, err := readerEngine.NewTransaction(waitCtx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		_, foundWatchObject, err := rtx.GetObject(waitCtx, "coordinator-head-watch-object")
		if err == nil && foundWatchObject {
			_, foundWriterA, err := rtx.GetObject(waitCtx, "coordinator-serialized-writer-a")
			if err == nil && foundWriterA {
				_, foundWriterB, err := rtx.GetObject(waitCtx, "coordinator-serialized-writer-b")
				if err == nil {
					foundWatchObject = foundWriterB
				}
			}
		}
		rtx.Discard()
		if err != nil {
			t.Fatal(err.Error())
		}
		if foundWatchObject {
			return
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("reader did not adopt durable world head from coordinator generation event")
		case <-time.After(10 * time.Millisecond):
		}
	}
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

	err = eng.AccessWorldState(ctx, nil, func(bls *world.WorldAccess) error {

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

	// Fence the engine so the first write's blocks drain from the deferred
	// single-writer buffer into the volume. The out-of-band modification below
	// reads the world root directly from the raw bucket, which only sees blocks
	// that have been made durable by Sync.
	if err := func() error {
		worldEng, _, worldEngRef, err := world.ExLookupWorldEngine(ctx, b, false, engineID, nil)
		if err != nil {
			return err
		}
		defer worldEngRef.Release()
		_, err = worldEng.Sync(ctx)
		return err
	}(); err != nil {
		t.Fatal(err.Error())
	}

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

		return worldEng.AccessWorldState(ctx, nil, func(rootBls *world.WorldAccess) error {
			worldObjRefFirstWrite = rootBls.Cursor().GetRef()
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
