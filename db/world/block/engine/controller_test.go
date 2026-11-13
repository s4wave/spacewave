package world_block_engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestControllerCoordinatorSupported checks that direct writes require durable generations.
func TestControllerCoordinatorSupported(t *testing.T) {
	// Prepare the World controller and its coordination scope.
	ctx := context.Background()
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	scope := coord.Scope{
		VolumeID:      "volume",
		ObjectStoreID: "store",
		ParticipantID: "engine",
	}

	// Require generation support as well as coordinator availability, and
	// fail on a lookup error other than unsupported.
	for _, c := range []struct {
		coord coord.Coordinator
		want  bool
		fails bool
	}{
		{coord: fakeCoordinator{capability: &coord.Capability{Supported: true, Generations: true}}, want: true},
		{coord: fakeCoordinator{capability: &coord.Capability{Supported: true}}},
		{coord: fakeCoordinator{capability: &coord.Capability{Supported: false}}},
		{coord: fakeCoordinator{err: coord.ErrUnsupported}},
		{coord: fakeCoordinator{err: errors.New("lookup failed")}, fails: true},
	} {
		got, err := ctrl.coordinatorSupported(ctx, c.coord, scope)
		if got != c.want || (err != nil) != c.fails {
			t.Fatalf("coordinatorSupported = %v, %v; want %v, failing %v", got, err, c.want, c.fails)
		}
	}
}

// TestControllerGetWorldEngineReturnsMissingInitHeadError checks that an absent immutable root resolves lookup with its error.
func TestControllerGetWorldEngineReturnsMissingInitHeadError(t *testing.T) {
	// Prepare the context and logger for immutable World startup.
	ctx := t.Context()
	log := logrus.New()
	le := logrus.NewEntry(log)

	// Build the real storage and controller bus used by initialization.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Configure an immutable root absent from storage.
	missingRootRef := controllerTestBlockRef(t, "world-engine-missing-init-head")
	conf := NewConfig(
		"test-world-engine-missing-init-head",
		tb.Volume.GetID(),
		tb.BucketId,
		"",
		&bucket.ObjectRef{
			BucketId: tb.BucketId,
			RootRef:  missingRootRef,
		},
		nil,
		false,
	)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Run initialization through the controller execution lifecycle.
	execCtx, execCancel := context.WithCancel(ctx)
	t.Cleanup(execCancel)
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- ctrl.Execute(execCtx)
	}()

	// Require a concrete missing-root error instead of an unresolved wait.
	getCtx, getCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(getCancel)
	eng, err := ctrl.GetWorldEngine(getCtx)
	if eng != nil {
		t.Fatalf("GetWorldEngine returned engine %T, want nil with fatal startup error", eng)
	}
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("GetWorldEngine error = %v, want block.ErrNotFound", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetWorldEngine waited until the guard context expired: %v", err)
	}

	// Initialization must end after publishing its fatal result.
	select {
	case err := <-execErrCh:
		if err != nil {
			t.Fatalf("Execute error = %v, want nil after publishing fatal startup error", err)
		}
	case <-getCtx.Done():
		t.Fatalf("Execute did not return after publishing fatal startup error: %v", getCtx.Err())
	}
}

// TestControllerRecoversMissingPersistedHead checks explicit recovery publishes the configured replacement.
func TestControllerRecoversMissingPersistedHead(t *testing.T) {
	// Prepare the context and logger for persisted-head recovery.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Build the real store that will retain the recovered head.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Persist a valid replacement World root.
	currentCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the replacement World root through the bucket cursor.
	currentTx, currentBlocks := currentCursor.BuildTransaction(nil)
	currentBlocks.ClearAllRefs()
	currentBlocks.SetBlock(world_block.NewWorld(world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV, true), true)
	currentRootRef, _, err := currentTx.Write(ctx, true)
	currentCursor.Release()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Retain the replacement World root for recovery configuration.
	currentHeadRef := &bucket.ObjectRef{
		BucketId: tb.BucketId,
		RootRef:  currentRootRef,
	}

	// Record an invalid persisted head to exercise explicit recovery.
	objectStoreID := "test-world-engine-recover-missing-head"
	missingRootRef := controllerTestBlockRef(t, objectStoreID)
	missingHeadRef := &bucket.ObjectRef{
		BucketId: tb.BucketId,
		RootRef:  missingRootRef,
	}
	writeControllerTestHead(t, ctx, tb, objectStoreID, missingHeadRef)

	// Enable recovery only for this configured replacement.
	conf := NewConfig(
		objectStoreID,
		tb.Volume.GetID(),
		tb.BucketId,
		objectStoreID,
		currentHeadRef,
		nil,
		false,
	)
	conf.RecoverMissingPersistedHead = true
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
	})

	// Run recovery through ordinary controller initialization.
	execCtx, execCancel := context.WithCancel(ctx)
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- ctrl.Execute(execCtx)
	}()

	// Require a readable World before the controller is exposed.
	getCtx, getCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(getCancel)
	eng, err := ctrl.GetWorldEngine(getCtx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if seqno, err := eng.GetSeqno(getCtx); err != nil {
		t.Fatal(err.Error())
	} else if seqno != 0 {
		t.Fatalf("recovered world seqno = %d, want 0", seqno)
	}

	// The World root holds the configured replacement, and the old head
	// record is gone.
	storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objectStoreID, tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(storeRef.Release)
	if _, found, err := (LegacyHead{Store: storeVal.GetObjectStore(), Key: ctrl.objectStoreHeadKey()}).load(ctx); err != nil || found {
		t.Fatalf("old world head record found=%v err=%v after recovery", found, err)
	}
	recovered := controllerTestWorldRoot(t, ctx, tb, objectStoreID)
	if recovered.EqualsRef(missingRootRef) {
		t.Fatal("recovered world retained the missing root")
	}
	if !recovered.EqualsRef(currentRootRef) {
		t.Fatal("recovered world did not select the configured current generation")
	}

	// Recovery must leave the execution serving the engine.
	select {
	case err := <-execErrCh:
		t.Fatalf("Execute exited after recovery: %v", err)
	default:
	}

	// Cancel and join the serving execution.
	execCancel()
	select {
	case err := <-execErrCh:
		if err != nil {
			t.Fatalf("Execute shutdown error = %v", err)
		}
	case <-getCtx.Done():
		t.Fatalf("Execute did not stop after cancellation: %v", getCtx.Err())
	}
}

// TestControllerAdoptsLegacyHead checks that a World an older engine kept
// under a HeadState record and a retained root opens at the record's head,
// held by the World root alone: the open moves the head to the World root,
// releases the retained root and deletes the record, and the World commits.
func TestControllerAdoptsLegacyHead(t *testing.T) {
	// Write two Worlds: the record's head, and an older one the retained root
	// still holds, so the two disagree.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	engineID := "test-world-engine-adopt"
	recorded := controllerTestWorld(t, ctx, tb, "recorded")
	retained := controllerTestWorld(t, ctx, tb, "retained")

	// Keep them as the older engine did.
	writeControllerTestHead(t, ctx, tb, engineID, &bucket.ObjectRef{BucketId: tb.BucketId, RootRef: recorded})
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)
	oldName := retainedRootName(engineID, []byte(defaultHeadStateKey))
	if err := cursor.GetBucket().SetRoots(ctx, nil, []block.NamedRoot{{Name: oldName, Ref: retained}}); err != nil {
		t.Fatal(err.Error())
	}

	// Open the World through the controller.
	conf := NewConfig(engineID, tb.Volume.GetID(), tb.BucketId, engineID, nil, nil, true)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	execCtx, execCancel := context.WithCancel(ctx)
	t.Cleanup(execCancel)
	go func() { _ = ctrl.Execute(execCtx) }()
	getCtx, getCancel := context.WithTimeout(ctx, 5*time.Second)
	t.Cleanup(getCancel)
	eng, err := ctrl.GetWorldEngine(getCtx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The record's head is the World root's, and nothing else holds it.
	if root := controllerTestWorldRoot(t, ctx, tb, engineID); !root.EqualsRef(recorded) {
		t.Fatalf("World root %v, want the recorded head", root)
	}
	if old, err := cursor.GetBucket().GetRoot(ctx, oldName); err != nil || old != nil {
		t.Fatalf("retained root %v %v survived the adoption", old, err)
	}
	storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, engineID, tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(storeRef.Release)
	if _, found, err := (LegacyHead{Store: storeVal.GetObjectStore(), Key: ctrl.objectStoreHeadKey()}).load(ctx); err != nil || found {
		t.Fatalf("head record found=%v err=%v after the adoption", found, err)
	}

	// The adopted World reads its object and commits through the World root.
	ws := world.NewEngineWorldState(eng, true)
	if found, err := ws.HasObject(ctx, "recorded"); err != nil || !found {
		t.Fatalf("adopted World lost its object: %v %v", found, err)
	}
	obj, err := ws.CreateObject(ctx, "after-adoption", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if root := controllerTestWorldRoot(t, ctx, tb, engineID); root.EqualsRef(recorded) {
		t.Fatal("a commit after the adoption did not move the World root")
	}
}

// TestControllerEphemeralWorldStartsEmpty checks that an ephemeral World
// persists no head: a second open finds nothing the first committed.
func TestControllerEphemeralWorldStartsEmpty(t *testing.T) {
	// Prepare a testbed and the ephemeral World's config.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	conf := NewConfig("test-world-engine-ephemeral", tb.Volume.GetID(), tb.BucketId, "", &bucket.ObjectRef{BucketId: tb.BucketId}, nil, true)
	conf.Ephemeral = true

	// open runs a controller on conf and returns its World with its stop.
	open := func() (Engine, func()) {
		t.Helper()
		ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
		if err != nil {
			t.Fatal(err.Error())
		}
		execCtx, execCancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = ctrl.Execute(execCtx)
		}()
		getCtx, getCancel := context.WithTimeout(ctx, 5*time.Second)
		defer getCancel()
		eng, err := ctrl.GetWorldEngine(getCtx)
		if err != nil {
			t.Fatal(err.Error())
		}
		return eng, func() {
			execCancel()
			<-done
			_ = ctrl.Close()
		}
	}

	// Commit an object in the first open.
	eng, stop := open()
	ws := world.NewEngineWorldState(eng, true)
	obj, err := ws.CreateObject(ctx, "first-open", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	stop()

	// The second open starts empty.
	eng, stop = open()
	defer stop()
	if found, err := world.NewEngineWorldState(eng, false).HasObject(ctx, "first-open"); err != nil || found {
		t.Fatalf("second open found the first open's object: %v %v", found, err)
	}
	if root := controllerTestWorldRoot(t, ctx, tb, "test-world-engine-ephemeral"); root != nil {
		t.Fatalf("an ephemeral World holds a head root %v", root)
	}
}

// TestControllerEngineSurvivesExecuteRestartUntilClose checks retained handles keep their storage through execution restarts.
func TestControllerEngineSurvivesExecuteRestartUntilClose(t *testing.T) {
	// Prepare the context and logger for controller restart checks.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Build shared storage that survives controller execution restarts.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Configure one durable World managed by the controller.
	const objectStoreID = "test-world-engine-execute-restart"
	conf := NewConfig(
		objectStoreID,
		tb.Volume.GetID(),
		tb.BucketId,
		objectStoreID,
		&bucket.ObjectRef{BucketId: tb.BucketId},
		nil,
		false,
	)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
	})

	// Bound startup and shutdown while retaining engine handles.
	getCtx, getCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(getCancel)
	startExecution := func() (Engine, context.CancelFunc, <-chan error) {
		// Start a serving World controller execution.
		execCtx, execCancel := context.WithCancel(ctx)
		execErrCh := make(chan error, 1)
		go func() {
			execErrCh <- ctrl.Execute(execCtx)
		}()

		// Wait for the execution to publish its World engine.
		eng, err := ctrl.GetWorldEngine(getCtx)
		if err != nil {
			execCancel()
			t.Fatal(err.Error())
		}
		return eng, execCancel, execErrCh
	}
	stopExecution := func(execCancel context.CancelFunc, execErrCh <-chan error) {
		execCancel()
		select {
		case err := <-execErrCh:
			if err != nil {
				t.Fatalf("Execute shutdown error = %v", err)
			}
		case <-getCtx.Done():
			t.Fatalf("Execute did not stop after cancellation: %v", getCtx.Err())
		}
	}
	assertOpen := func(name string, eng Engine) {
		// Require the retained World engine to expose a readable sequence.
		t.Helper()
		if _, err := eng.GetSeqno(ctx); err != nil {
			t.Fatalf("%s engine is unusable while controller remains attached: %v", name, err)
		}

		// Require the retained engine to have a live block World root.
		blockEngine, ok := eng.(*world_block.Engine)
		if !ok {
			t.Fatalf("%s engine type = %T, want *world_block.Engine", name, eng)
		}
		rootRef := blockEngine.GetRootRef()
		if rootRef == nil || rootRef.GetRootRef().GetEmpty() {
			t.Fatalf("%s engine has no live world root", name)
		}

		// Open and discard a read transaction through the retained engine.
		readTx, err := eng.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("%s engine read transaction: %v", name, err)
		}
		readTx.Discard()
	}
	assertClosed := func(name string, eng Engine) {
		t.Helper()
		_, err := eng.GetSeqno(ctx)
		if !errors.Is(err, world_block.ErrEngineClosed) {
			t.Fatalf("%s engine error after Close = %v, want %v", name, err, world_block.ErrEngineClosed)
		}
	}

	// Keep the first engine usable after its execution returns.
	firstEngine, firstCancel, firstErrCh := startExecution()
	stopExecution(firstCancel, firstErrCh)
	assertOpen("first after Execute return", firstEngine)

	// Commit and sync an object through the engine after execution ends.
	firstTx, err := firstEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	{
		createdObject, err := firstTx.CreateObject(ctx, "after-execute-return", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			firstTx.Discard()
			t.Fatal(err.Error())
		}
	}
	if err := firstTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := firstEngine.Sync(ctx); err != nil {
		t.Fatalf("sync after Execute return: %v", err)
	}

	// Restart execution while retaining access through the first handle.
	secondEngine, secondCancel, secondErrCh := startExecution()
	t.Cleanup(secondCancel)
	assertOpen("first after Execute restart", firstEngine)

	// Require the first engine to read its object after execution restarts.
	restartReadTx, err := firstEngine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	objectState, found, err := restartReadTx.GetObject(ctx, "after-execute-return")
	world.ReleaseObjectState(objectState)
	restartReadTx.Discard()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("retained engine did not read committed object after Execute restart")
	}

	// Close both retained engines and join the current execution.
	if err := ctrl.Close(); err != nil {
		t.Fatal(err.Error())
	}
	select {
	case err := <-secondErrCh:
		if err != nil {
			t.Fatalf("Execute shutdown during Close = %v", err)
		}
	case <-getCtx.Done():
		t.Fatalf("Close did not join Execute: %v", getCtx.Err())
	}
	assertClosed("first", firstEngine)
	assertClosed("second", secondEngine)
}

// TestControllerDoesNotRecoverMissingPersistedHeadByDefault checks missing durable data is not replaced without configured recovery.
func TestControllerDoesNotRecoverMissingPersistedHeadByDefault(t *testing.T) {
	// Prepare the context and logger for missing-head failure checks.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Build storage for a missing persisted root.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Persist a head whose blocks are unavailable.
	objectStoreID := "test-world-engine-missing-persisted-head"
	missingHeadRef := &bucket.ObjectRef{
		BucketId: tb.BucketId,
		RootRef:  controllerTestBlockRef(t, objectStoreID),
	}
	writeControllerTestHead(t, ctx, tb, objectStoreID, missingHeadRef)

	// Leave recovery disabled and require the original storage error.
	conf := NewConfig(
		objectStoreID,
		tb.Volume.GetID(),
		tb.BucketId,
		objectStoreID,
		&bucket.ObjectRef{BucketId: tb.BucketId},
		nil,
		false,
	)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ctrl.Execute(ctx); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Execute error = %v, want block.ErrNotFound", err)
	}
}

// writeControllerTestHead commits test metadata through the real head store.
func writeControllerTestHead(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	objectStoreID string,
	headRef *bucket.ObjectRef,
) {
	// Resolve the real ObjectStore for test head metadata.
	t.Helper()
	storeVal, _, storeRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		tb.Bus,
		false,
		objectStoreID,
		tb.Volume.GetID(),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(storeRef.Release)

	// Commit the head through the real ObjectStore transaction.
	ktx, err := storeVal.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ktx.Discard)
	data, err := (&HeadState{HeadRef: headRef}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ktx.Set(ctx, []byte(defaultHeadStateKey), data); err != nil {
		t.Fatal(err.Error())
	}
	if err := ktx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// controllerTestBlockRef constructs a deterministic reference for a test root.
func controllerTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_BLAKE3, []byte(data))
	if err != nil {
		t.Fatal(err.Error())
	}
	return block.NewBlockRef(h)
}

// controllerTestWorldRoot returns the block the World root of engineID holds
// in the testbed's bucket.
func controllerTestWorldRoot(t *testing.T, ctx context.Context, tb *testbed.Testbed, engineID string) *block.BlockRef {
	// Read the root through a cursor on the bucket.
	t.Helper()
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cursor.Release()
	ref, err := cursor.GetBucket().GetRoot(ctx, world_block.WorldRootName(engineID))
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// controllerTestWorld writes a new World with object key and returns its root.
func controllerTestWorld(t *testing.T, ctx context.Context, tb *testbed.Testbed, key string) *block.BlockRef {
	// Open a World state on an empty cursor.
	t.Helper()
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cursor.Release()
	ws, err := world_block.BuildMockWorldState(ctx, tb.Logger, true, cursor, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ws.Discard()

	// Create the object and write the World.
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	return ws.GetRootRef()
}

// fakeCoordinator reports only the configured capability response.
type fakeCoordinator struct {
	capability *coord.Capability
	err        error
}

// Capability returns the configured capability or lookup error.
func (f fakeCoordinator) Capability(context.Context, coord.Scope) (*coord.Capability, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.capability, nil
}

// Snapshot rejects unsupported generation reads.
func (fakeCoordinator) Snapshot(context.Context, coord.Scope) (*coord.Snapshot, error) {
	return nil, coord.ErrUnsupported
}

// Watch rejects unsupported observation.
func (fakeCoordinator) Watch(context.Context, coord.Scope, uint64) (coord.Watch, error) {
	return nil, coord.ErrUnsupported
}

// TryAcquireWriteLease rejects unsupported write leases.
func (fakeCoordinator) TryAcquireWriteLease(context.Context, coord.Scope) (coord.WriteLease, bool, error) {
	return nil, false, coord.ErrUnsupported
}

// WaitAcquireWriteLease rejects unsupported write leases.
func (fakeCoordinator) WaitAcquireWriteLease(context.Context, coord.Scope) (coord.WriteLease, error) {
	return nil, coord.ErrUnsupported
}

// _ verifies the coordinator test adapter contract.
var _ coord.Coordinator = fakeCoordinator{}

// TestControllerSharesDecodedBlocksAcrossTransactions checks that a read
// transaction reuses blocks decoded by an earlier transaction.
func TestControllerSharesDecodedBlocksAcrossTransactions(t *testing.T) {
	// Build storage for one durable World managed by the controller.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Construct the controller for one World in the testbed bucket.
	const objectStoreID = "test-world-engine-decoded-blocks"
	conf := NewConfig(
		objectStoreID,
		tb.Volume.GetID(),
		tb.BucketId,
		objectStoreID,
		&bucket.ObjectRef{BucketId: tb.BucketId},
		nil,
		false,
	)
	ctrl, err := NewController(le, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		_ = ctrl.Close()
	})

	// Start the controller and wait for its World engine.
	execCtx, execCancel := context.WithCancel(ctx)
	t.Cleanup(execCancel)
	go func() {
		_ = ctrl.Execute(execCtx)
	}()
	getCtx, getCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(getCancel)
	eng, err := ctrl.GetWorldEngine(getCtx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit an object so reads decode the World root and object tree.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj, err := tx.CreateObject(ctx, "object", nil)
	if err != nil {
		tx.Discard()
		t.Fatal(err.Error())
	}
	world.ReleaseObjectState(obj)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Read the object in a fresh transaction and count its decoded-block hits.
	readObject := func() block.ReadCounterSnapshot {
		// Open the read transaction under a fresh counter.
		t.Helper()
		readCtx, counter := block.WithReadCounter(ctx)
		readTx, err := eng.NewTransaction(readCtx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer readTx.Discard()

		// Decode the object through the transaction.
		obj, err := world.MustGetObject(readCtx, readTx, "object")
		if err != nil {
			t.Fatal(err.Error())
		}
		world.ReleaseObjectState(obj)
		return counter.Snapshot()
	}

	// The second transaction must hit blocks the first one stored.
	readObject()
	if hits := readObject().DecodedBlockCacheHitCount; hits == 0 {
		t.Fatal("second read transaction had no decoded-block cache hits")
	}
}
