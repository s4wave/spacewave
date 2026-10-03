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

// newEngineTestbed starts a storage testbed with the World engine factory. It
// returns a function that starts the test World engine, which keeps a
// changelog when enableChangelog is set.
func newEngineTestbed(
	t *testing.T,
	enableChangelog bool,
) (context.Context, *logrus.Entry, *testbed.Testbed, func() (*world_block_engine.Controller, directive.Reference)) {
	// Report failures at the caller and configure debug logging.
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed with the World engine factory.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Derive the state encryption key and build the state transform.
	objectStoreID := "test-world-engine-store"
	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test: engine_test.go", []byte(objectStoreID), encKey)
	le.Infof("using encryption key: %s", b58.Encode(encKey))
	stateTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Configure the engine. initWorldRef is used only when the World has not
	// been initialized before.
	initWorldRef := &bucket.ObjectRef{
		BucketId:      tb.BucketId,
		TransformConf: stateTransformConf,
	}
	engineConf := world_block_engine.NewConfig(
		"test-world-engine",
		tb.Volume.GetID(), tb.BucketId,
		objectStoreID,
		initWorldRef,
		stateTransformConf,
		enableChangelog,
	)
	startEngine := func() (*world_block_engine.Controller, directive.Reference) {
		worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf)
		if err != nil {
			t.Fatal(err.Error())
		}
		return worldCtrl, worldCtrlRef
	}
	return ctx, le, tb, startEngine
}

// TestWorldEngineController tests constructing the engine controller, looking up
// the engine on the bus, & running some basic queries.
func TestWorldEngineController(t *testing.T) {
	// Start the engine with a changelog.
	ctx, le, tb, startEngine := newEngineTestbed(t, true)
	engineID := "test-world-engine"
	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()

	// Provide the mock object op handlers to the bus.
	opc := world.NewLookupOpController("test-world-engine-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relOpc()

	// Open and discard a write transaction on the engine.
	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// Run the World engine suite against the engine the bus resolves.
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	if err := world_mock.TestWorldEngine(ctx, le, busEngine); err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	// Check that the stored World state decodes.
	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		_, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Remount the World.
	worldCtrlRef.Release()
	worldCtrl, worldCtrlRef = startEngine()
	defer worldCtrlRef.Release()
	eng, err = worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check that the suite's object survived the remount.
	engTx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer engTx.Discard()
	objectState, found, err := engTx.GetObject(ctx, "test-object")
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("object not found after remounting")
	}
}

func TestWorldEngineControllerUsesDeferredDurabilityWithoutGenerations(t *testing.T) {
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
	kvtxVolume.Coordinator = generationlessCoordinator{Coordinator: kvtxVolume.Coordinator}

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
	{
		createdObject, err := tx.CreateObject(ctx, "unsupported-coordinator-fallback", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			tx.Discard()
			t.Fatalf("create object with unsupported coordinator: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit with unsupported coordinator: %v", err)
	}
	fenced, err := engine.Sync(ctx)
	if err != nil {
		t.Fatalf("sync deferred write: %v", err)
	}
	if !fenced {
		t.Fatal("generationless coordinator did not select deferred durability")
	}
}

// generationlessCoordinator reports a coordinator that supports leases but not
// generations.
type generationlessCoordinator struct {
	coord.Coordinator
}

// Capability returns the wrapped capability with generations disabled.
func (c generationlessCoordinator) Capability(
	ctx context.Context,
	scope coord.Scope,
) (*coord.Capability, error) {
	// Read the wrapped capability and clear its generation support.
	capability, err := c.Coordinator.Capability(ctx, scope)
	if err != nil {
		return nil, err
	}
	capability.Supported = true
	capability.Generations = false
	return capability, nil
}

// TestWorldEngineControllerCoordinatorHeadWatch checks that engines sharing an
// object store serialize writers through the coordinator lease, reject stale
// heads, publish accepted roots and adopt each other's durable heads.
func TestWorldEngineControllerCoordinatorHeadWatch(t *testing.T) {
	// Configure debug logging.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start a Bolt-backed testbed with the World engine factory.
	boltPath := filepath.Join(t.TempDir(), "world-head-watch.bolt")
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_bolt.Config{Path: boltPath}))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Start a writer and a reader engine on the same object store.
	volumeID := tb.Volume.GetID()
	objectStoreID := "test-world-engine-head-watch-store"
	writerEngine, releaseWriter := startHeadWatchEngine(ctx, t, tb, "test-world-engine-head-watch-writer", objectStoreID)
	defer releaseWriter()
	readerEngine, releaseReader := startHeadWatchEngine(ctx, t, tb, "test-world-engine-head-watch-reader", objectStoreID)
	defer releaseReader()

	// A writer transaction waits while an external participant holds the
	// coordinator write lease, and acquires once the lease is released.
	externalLease, err := tb.Volume.WaitAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      volumeID,
		ObjectStoreID: objectStoreID,
		ParticipantID: "external-writer",
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	blockedTx, blockedErr := startWriteTx(ctx, writerEngine)
	requireWriteBlocked(t, blockedTx, blockedErr, "external coordinator lease was held")
	if err := externalLease.Release(ctx); err != nil {
		t.Fatal(err.Error())
	}
	waitWriteAcquired(t, blockedTx, blockedErr, "external lease release").Discard()

	// A commit against a head replaced underneath the transaction fails as
	// stale; then restore the original head.
	baseHead := writerEngine.(*world_block.Engine).GetRootRef()
	staleTx := createObjectTx(ctx, t, writerEngine, "coordinator-stale-head-object")
	writeRawHead(ctx, t, tb, objectStoreID, &bucket.ObjectRef{BucketId: tb.BucketId})
	if err := staleTx.Commit(ctx); !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatalf("stale head commit error = %v, want ErrStaleGeneration", err)
	}
	writeRawHead(ctx, t, tb, objectStoreID, baseHead)

	// Watch the coordinator from the current generation.
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

	// A writer commit publishes its root to the coordinator snapshot.
	tx := createObjectTx(ctx, t, writerEngine, "coordinator-head-watch-object")
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

	// The watch observes the publication of that root.
	waitRootPublished(ctx, t, watch, acceptedRoot)

	// The reader engine's write transaction waits while the writer engine holds
	// the lease, and acquires once the writer commits.
	firstWriterTx := createObjectTx(ctx, t, writerEngine, "coordinator-serialized-writer-a")
	secondWriterTx, secondWriterErr := startWriteTx(ctx, readerEngine)
	requireWriteBlocked(t, secondWriterTx, secondWriterErr, "first standalone writer held lease")
	if err := firstWriterTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	secondTx := waitWriteAcquired(t, secondWriterTx, secondWriterErr, "first writer commit")
	createObject(ctx, t, secondTx, "coordinator-serialized-writer-b")
	if err := secondTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Wait for the reader to adopt the durable head holding all three objects,
	// waking on each seqno change.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		seqno, found := readObjectsFound(waitCtx, t, readerEngine,
			"coordinator-head-watch-object",
			"coordinator-serialized-writer-a",
			"coordinator-serialized-writer-b",
		)
		if found {
			return
		}
		if _, err := readerEngine.WaitSeqno(waitCtx, seqno+1); err != nil {
			t.Fatalf("reader did not adopt durable world head from coordinator generation event: %v", err)
		}
	}
}

// startHeadWatchEngine starts a World engine on objectStoreID with an
// untransformed state and returns it with its release function.
func startHeadWatchEngine(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	engineID, objectStoreID string,
) (world.Engine, func()) {
	// Report failures at the caller and build the initial World reference.
	t.Helper()
	transformConf, err := block_transform.NewConfig(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	initWorldRef := &bucket.ObjectRef{
		BucketId:      tb.BucketId,
		TransformConf: transformConf,
	}

	// Start the controller and wait for its engine.
	engineConf := world_block_engine.NewConfig(
		engineID,
		tb.Volume.GetID(), tb.BucketId,
		objectStoreID,
		initWorldRef,
		nil,
		false,
	)
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, engineConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		worldCtrlRef.Release()
		t.Fatal(err.Error())
	}
	return eng, worldCtrlRef.Release
}

// createObjectTx opens a write transaction on eng and creates the object key
// in it, leaving the transaction open.
func createObjectTx(ctx context.Context, t *testing.T, eng world.Engine, key string) world.Tx {
	// Report failures at the caller and open the transaction.
	t.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the object.
	createObject(ctx, t, tx, key)
	return tx
}

// createObject creates the object key in tx, discarding tx on failure.
func createObject(ctx context.Context, t *testing.T, tx world.Tx, key string) {
	// Report failures at the caller and create the object.
	t.Helper()
	objectState, err := tx.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(objectState)
	if err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
}

// startWriteTx opens a write transaction on eng in the background and delivers
// the transaction or its error.
func startWriteTx(ctx context.Context, eng world.Engine) (<-chan world.Tx, <-chan error) {
	// Open the transaction without blocking the caller.
	txCh := make(chan world.Tx, 1)
	errCh := make(chan error, 1)
	go func() {
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			errCh <- err
			return
		}
		txCh <- tx
	}()
	return txCh, errCh
}

// requireWriteBlocked fails if the background write transaction resolves
// within a short window while holder keeps the write lease.
func requireWriteBlocked(t *testing.T, txCh <-chan world.Tx, errCh <-chan error, holder string) {
	// Report failures at the caller and watch the transaction briefly.
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("write transaction failed while %s: %v", holder, err)
	case tx := <-txCh:
		tx.Discard()
		t.Fatalf("write transaction acquired while %s", holder)
	case <-time.After(50 * time.Millisecond):
	}
}

// waitWriteAcquired returns the background write transaction once it acquires
// after the event named by after.
func waitWriteAcquired(t *testing.T, txCh <-chan world.Tx, errCh <-chan error, after string) world.Tx {
	// Report failures at the caller and wait for the transaction.
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("write transaction failed after %s: %v", after, err)
	case tx := <-txCh:
		return tx
	case <-time.After(5 * time.Second):
		t.Fatalf("write transaction did not acquire after %s", after)
	}
	return nil
}

// writeRawHead overwrites the World head key in the object store with ref.
func writeRawHead(ctx context.Context, t *testing.T, tb *testbed.Testbed, objectStoreID string, ref *bucket.ObjectRef) {
	// Report failures at the caller and open the object store.
	t.Helper()
	storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objectStoreID, tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRef.Release()

	// Write the encoded head state and commit.
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

// waitRootPublished waits until watch reports root published at the World
// head key.
func waitRootPublished(ctx context.Context, t *testing.T, watch coord.Watch, root *bucket.ObjectRef) {
	// Report failures at the caller and bound the wait.
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Read events until one publishes root.
	var seenEvents []coord.Event
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("coordinator watch did not observe accepted world root publication; events=%+v", seenEvents)
		case event, ok := <-watch.Events():
			if !ok {
				t.Fatal("coordinator watch closed before accepted world root publication")
			}
			seenEvents = append(seenEvents, event)
			if event.RootChanged != nil && event.RootChanged.EqualsRef(root) && string(event.KeyPrefixChanged) == "world-head" {
				return
			}
		}
	}
}

// readObjectsFound reads the engine's seqno and reports whether every key
// names an existing object, in one read transaction.
func readObjectsFound(ctx context.Context, t *testing.T, eng world.Engine, keys ...string) (uint64, bool) {
	// Open the read transaction and read its seqno.
	t.Helper()
	rtx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rtx.Discard()
	seqno, err := rtx.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Look up each object.
	for _, key := range keys {
		objectState, found, err := rtx.GetObject(ctx, key)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !found {
			return seqno, false
		}
	}
	return seqno, true
}

// TestWorldEngineController_DisableChangelog tests constructing the engine
// controller with the changelog disabled.
func TestWorldEngineController_DisableChangelog(t *testing.T) {
	// Start the engine without a changelog.
	ctx, le, tb, startEngine := newEngineTestbed(t, false)
	worldCtrl, worldCtrlRef := startEngine()
	defer worldCtrlRef.Release()

	// Open and discard a write transaction on the engine.
	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// Run the World engine suite against the engine the bus resolves.
	busEngine := world.NewBusEngine(ctx, tb.Bus, "test-world-engine")
	if err := world_mock.TestWorldEngine(ctx, le, busEngine); err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	// Check that the World recorded no change beyond its sequence number.
	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		// Decode the World block.
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			return err
		}
		worldState := wi.(*world_block.World)

		// Clear the sequence number and require an empty last change.
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
			createdObject, _, err := world.CreateWorldObject(ctx, wtx, objKey, func(bcs *block.Cursor) error {
				_, err := blob.BuildBlobWithBytes(ctx, []byte("Hello world"), bcs)
				return err
			})
			world.ReleaseObjectState(createdObject)
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
