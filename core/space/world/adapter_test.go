package space_world_test

import (
	"context"
	"math"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// adapterFixture is a Space on the local provider served by the Space World
// controller.
type adapterFixture struct {
	// tb is the testbed.
	tb *testbed.Testbed
	// ref is the Space's SharedObject.
	ref *sobject.SharedObjectRef
	// engineID is the World engine ID.
	engineID string
	// volume is the account's Volume.
	volume volume.Volume
	// frees is set when the Volume's collector frees at once.
	frees bool
	// freed counts the blocks the collections freed.
	freed uint64
	// maxPending bounds the pages the adapter keeps, zero for its default.
	maxPending uint64
	// acc is the provider account.
	acc *provider_local.ProviderAccount
}

// fixtureOpts selects how a fixture stores its Space.
type fixtureOpts struct {
	// sql and creds make an SQL location the account's primary store.
	sql   *account_settings.SqlLocation
	creds *block_store_s3.Credentials
	// frees has the account's Volume free every block no root holds, at
	// once, whenever the test collects.
	frees bool
	// maxPending bounds the pages the adapter keeps, zero for its default.
	maxPending uint64
	// opTypes resolves the operation types, or nil for the mock operations.
	opTypes *space_world.OpTypes
}

// newAdapterFixture creates a Space on a local provider.
func newAdapterFixture(t *testing.T) *adapterFixture {
	return newAdapterFixtureOn(t, fixtureOpts{})
}

// collect runs the account Volume's collector until a pass leaves no work,
// when the fixture frees.
func (f *adapterFixture) collect(t *testing.T) {
	t.Helper()
	if !f.frees {
		return
	}
	collector := f.volume.(volume.Collector)
	for range 64 {
		res, err := collector.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		f.freed += res.Freed
		if !res.More {
			return
		}
	}
	t.Fatal("collection did not finish")
}

// sqlTestLocation returns the database BLOCK_STORE_SQL_TEST_HOST names, such
// as a MariaDB container, with its credentials, and skips the test without
// one.
func sqlTestLocation(t *testing.T) (*account_settings.SqlLocation, *block_store_s3.Credentials) {
	t.Helper()
	host := os.Getenv("BLOCK_STORE_SQL_TEST_HOST")
	if host == "" {
		t.Skip("BLOCK_STORE_SQL_TEST_HOST names no MySQL-compatible server")
	}
	port, _ := strconv.ParseUint(os.Getenv("BLOCK_STORE_SQL_TEST_PORT"), 10, 32)
	location := &account_settings.SqlLocation{
		Host:        host,
		Port:        uint32(port),
		Database:    os.Getenv("BLOCK_STORE_SQL_TEST_DATABASE"),
		TablePrefix: "adapter_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_",
	}
	creds := &block_store_s3.Credentials{
		AccessKeyId:     os.Getenv("BLOCK_STORE_SQL_TEST_USER"),
		SecretAccessKey: os.Getenv("BLOCK_STORE_SQL_TEST_PASSWORD"),
	}
	return location, creds
}

// newAdapterFixtureOn creates a Space on a local provider as opts select.
func newAdapterFixtureOn(t *testing.T, opts fixtureOpts) *adapterFixture {
	// Build the testbed with the engine and local provider factories, on
	// storage that frees at once when asked.
	t.Helper()
	ctx := t.Context()
	var tbOpts []testbed.Option
	if opts.frees {
		storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
		tbOpts = append(tbOpts, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))
	}
	tb, err := testbed.Default(ctx, tbOpts...)
	if err != nil {
		t.Fatal(err)
	}
	opTypes := space_world.OpTypes{Lookup: world_mock.LookupMockOp}
	if opts.opTypes != nil {
		opTypes = *opts.opTypes
	}
	tb.StaticResolver.AddFactory(space_world.NewFactory(tb.Bus, opTypes))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	// Start the local provider and create the Space.
	_, provRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "test-account", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provAccRef.Release)
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err)
	}

	// Make the SQL location the account's primary store.
	acc := provAcc.(*provider_local.ProviderAccount)
	if opts.sql != nil {
		acc.SetSQLPrimary(true)
		backendID, check, err := acc.AddSQLStorageBackend(ctx, "sql", opts.sql, opts.creds, true)
		if err != nil || backendID == "" {
			t.Fatalf("add the SQL backend: %v %v", check, err)
		}
	}
	ref, err := soProv.CreateSharedObject(ctx, "test-space", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.sql != nil {
		settings, err := acc.ResolveNewSpaceStorageBackend(ctx, "", false)
		if err != nil {
			t.Fatal(err)
		}
		placement := &account_settings.BlockStorePlacement{BlockStoreId: ref.GetBlockStoreId(), StorageBackendId: settings}
		if err := acc.PlaceBlockStore(ctx, placement); err != nil {
			t.Fatal(err)
		}
	}
	return &adapterFixture{tb: tb, ref: ref, engineID: "test-space-world", volume: acc.GetVolume(), frees: opts.frees, maxPending: opts.maxPending, acc: acc}
}

// start starts the engine controller and returns its engine.
func (f *adapterFixture) start(t *testing.T) (*world.BusEngine, directive.Reference) {
	t.Helper()
	conf := space_world.NewConfig(f.engineID, f.ref)
	if f.frees {
		// The Volume frees at once, so a reader's replaced head is pinned
		// at once, as the production pin delay stays below the grace.
		conf.ReaderPinDelay = "0s"
	}
	conf.MaxPendingBytes = f.maxPending
	eng, ref, err := world.StartEngine(t.Context(), f.tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	return eng, ref
}

// remount releases ref, waits for the engine eng served to retire, then
// starts the Space World engine again, so no read or write after it reaches
// the released engine. Starting before the retire would let the bus keep the
// same controller. The retire is prompt: the engine ends its open
// transactions as superseded.
func (f *adapterFixture) remount(ctx context.Context, t *testing.T, eng world.Engine, ref directive.Reference) (*world.BusEngine, directive.Reference) {
	t.Helper()
	var served *space_world.Engine
	if err := world.AccessEngine(ctx, eng, func(_ context.Context, eng world.Engine) error {
		served = eng.(*space_world.Engine)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ref.Release()
	if _, err := served.WaitSeqno(ctx, math.MaxUint64); !world.IsSuperseded(err) {
		t.Fatalf("waiting for the released engine to retire: %v", err)
	}
	return f.start(t)
}

// space returns the Space's SharedObject.
func (f *adapterFixture) space(t *testing.T) sobject.SharedObject {
	t.Helper()
	so, ref, err := sobject.ExMountSharedObject(t.Context(), f.tb.Bus, f.ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
	return so
}

// hasObjects reports an error unless eng reads every key.
func hasObjects(ctx context.Context, eng world.Engine, keys ...string) error {
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, key := range keys {
		obj, found, err := tx.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			return errors.Errorf("object %s: found %v, %v", key, found, err)
		}
	}
	return nil
}

// waitBackfilled waits for a backfill outcome that covers the World the
// replay base root holds, the installed checkpoint's, or that reaches the
// head's seqno, whichever comes first, and fails unless it covers the
// checkpoint World whole. A copy at each head reaches the head's seqno without
// covering the checkpoint World while the head is above the checkpoint, so it
// fails here rather than waiting out the context.
func waitBackfilled(ctx context.Context, t *testing.T, eng *space_world.Engine, store block.StoreOps) {
	t.Helper()
	_, seqno, err := eng.GetHead(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var base *block.BlockRef
	state, err := eng.GetBackfillState().WaitValueWithValidator(ctx, func(state *space_world.BackfillState) (bool, error) {
		if state == nil {
			return false, nil
		}
		base, err = store.GetRoot(ctx, space_world.RootReplayBase)
		return err != nil || state.GetRoot().EqualsRef(base) || state.GetSeqno() >= seqno, err
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GetRoot().EqualsRef(base) || !state.IsComplete() {
		t.Fatalf("backfill covered %v at seqno %d with %d holes, want the checkpoint World %v whole", state.GetRoot(), state.GetSeqno(), state.GetHoles(), base)
	}
}

// backfillHolds reports whether the backfill root holds root.
func backfillHolds(ctx context.Context, t *testing.T, store block.StoreOps, root *block.BlockRef) bool {
	t.Helper()
	ref, err := store.GetRoot(ctx, space_world.RootBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetEmpty() {
		return false
	}
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found {
		t.Fatalf("backfill root block: %v, %v", found, err)
	}
	held := &space_world.HeldRoots{}
	if err := held.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(held.GetRoots(), root.EqualsRef)
}

// TestAdapterRunThrough serves a Space World through its adapter on the
// local provider: the World engine suite runs through the bus; concurrent
// writers pipeline and each reads its own write after its commit; the owner
// checkpoints its stable history, which moves the replay base root and
// releases the checkpoint root; and a remounted engine reads every object.
func TestAdapterRunThrough(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		runAdapterThrough(t, newAdapterFixture(t))
	})
	t.Run("frees", func(t *testing.T) {
		f := newAdapterFixtureOn(t, fixtureOpts{frees: true})
		runAdapterThrough(t, f)
		if f.freed == 0 {
			t.Fatal("the collections freed nothing, so the run-through checked no hold")
		}
	})
	t.Run("frees-overflow", func(t *testing.T) {
		f := newAdapterFixtureOn(t, fixtureOpts{frees: true, maxPending: 4096})
		runAdapterThrough(t, f)
		if f.freed == 0 {
			t.Fatal("the collections freed nothing, so the run-through checked no hold")
		}
	})
	t.Run("plugin-op", runPluginOpThrough)
	t.Run("sql-primary", func(t *testing.T) {
		location, creds := sqlTestLocation(t)
		runAdapterThrough(t, newAdapterFixtureOn(t, fixtureOpts{sql: location, creds: creds}))
	})

	// A local commit stores no block over 1x and 10x objects, as the Phase 4
	// path walk says: its one Volume write is the SharedObject's state, and
	// the World's pages wait in memory for the next stored World.
	t.Run("scaling", func(t *testing.T) {
		small, large := measureAdapterCommit(t, 20), measureAdapterCommit(t, 200)
		t.Logf("a local commit stored %d blocks over 1x objects and %d over 10x", small, large)
		if small != 0 || large != 0 {
			t.Fatalf("a local commit stored %d blocks over 1x objects and %d over 10x, want none", small, large)
		}

		// A remount applies only the operations past the saved replay
		// cursor, whatever stands above the checkpoint. The cursor falls at
		// most 32 operations behind, and a save in flight at the remount at
		// most 32 more; each operation resolves the mock type twice, to run
		// and for its payloads.
		small, large = measureRemountReplay(t, 40), measureRemountReplay(t, 240)
		t.Logf("a remount resolved the mock type %d times over 40 operations above the checkpoint and %d over 240", small, large)
		if small > 128 || large > 128 {
			t.Fatalf("a remount resolved the mock type %d times over 40 operations above the checkpoint and %d over 240, want at most 128", small, large)
		}
	})
}

// measureRemountReplay writes n World operations to a Space whose stable
// point a silent device holds, so all n stay above the checkpoint, then
// remounts the Space World and returns how many times replay resolved the
// mock World operation type before the remounted engine read the last object.
func measureRemountReplay(t *testing.T, n int) uint64 {
	// Count the mock World operations the Space's lookup resolves.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	var ran atomic.Uint64
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{
		Lookup: func(ctx context.Context, opTypeID string) (world.Operation, error) {
			if opTypeID == world_mock.MockWorldOpTypeID {
				ran.Add(1)
			}
			return world_mock.LookupMockOp(ctx, opTypeID)
		},
	}})
	eng, engRef := f.start(t)
	addDevice(t, f.space(t))

	// Write n operations, each creating an object and running the mock
	// World operation on it.
	for i := range n {
		key := "replay-" + strconv.Itoa(i)
		err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			_, _, err = ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp(key, "replay"), "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Remount and count what replay runs to reach the last object.
	last := "replay-" + strconv.Itoa(n-1)
	ran.Store(0)
	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	waitObjects(ctx, t, eng, last)
	return ran.Load()
}

// measureAdapterCommit starts a Space World with n objects and returns the
// blocks the account Volume stored during one more local commit.
func measureAdapterCommit(t *testing.T, n int) uint64 {
	// Start the engine and write the objects, fifty per transaction.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	create := func(keys ...string) {
		t.Helper()
		err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
			for _, key := range keys {
				obj, err := ws.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < n; i += 50 {
		var keys []string
		for j := i; j < min(i+50, n); j++ {
			keys = append(keys, "object-"+strconv.Itoa(j))
		}
		create(keys...)
	}

	// Count the blocks the Volume stores during one more commit.
	stored := func() uint64 {
		var blocks uint64
		for _, put := range f.volume.(volume.FlowReporter).GetFlows().Puts {
			blocks += put.Blocks
		}
		return blocks
	}
	before := stored()
	create("probe")
	if err := hasObjects(ctx, busEngine, "probe"); err != nil {
		t.Fatal(err)
	}
	delta := stored() - before

	// The count sees this Space's writes: enough commits for a checkpoint
	// store the checkpoint's World.
	for i := range sobject.MinCheckpointOperations {
		create("checkpoint-" + strconv.Itoa(i))
	}
	for stored() == before+delta {
		if ctx.Err() != nil {
			t.Fatal("the checkpoint stored no block the Volume counted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return delta
}

// runAdapterThrough walks the adapter run-through on f.
func runAdapterThrough(t *testing.T, f *adapterFixture) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	eng, engRef := f.start(t)

	// The World engine suite passes through the bus.
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	if err := world_mock.TestWorldEngine(ctx, f.tb.Logger, busEngine); err != nil {
		t.Fatal(err)
	}
	f.collect(t)

	// Backfill copies the installed checkpoint's World and the retained roots
	// and holds them; a retained root holds the head it names.
	store := f.space(t).GetBlockStore()
	var se *space_world.Engine
	var retained *block.BlockRef
	err := world.AccessEngine(ctx, busEngine, func(ctx context.Context, eng world.Engine) error {
		// Turn backfill on and wait for its first outcome.
		se = eng.(*space_world.Engine)
		if err := se.SetBackfill(ctx, true); err != nil {
			return err
		}
		_, err := se.GetBackfillState().WaitValueWithValidator(ctx, func(state *space_world.BackfillState) (bool, error) {
			return state.IsComplete(), nil
		}, nil)
		if err != nil {
			return err
		}

		// Retain the head, commit past it, and wait for a copy that holds it.
		head, _, err := se.GetHead(ctx)
		if err != nil {
			return err
		}
		if err := se.SetRetainedRoot(ctx, "export", head); err != nil {
			return err
		}
		retained = head
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		obj, err := tx.CreateObject(ctx, "backfill-object", nil)
		world.ReleaseObjectState(obj)
		if err == nil {
			err = tx.Commit(ctx)
		}
		tx.Discard()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for !backfillHolds(ctx, t, store, retained) {
		if ctx.Err() != nil {
			t.Fatal("backfill never held the retained root")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A reader holds a snapshot across the writes and the checkpoint below.
	snapshot, err := busEngine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Discard()

	// Concurrent writers each commit and read their own write, and together
	// pass the checkpoint threshold.
	var wg sync.WaitGroup
	errs := make(chan error, sobject.MinCheckpointOperations)
	for i := range sobject.MinCheckpointOperations {
		wg.Go(func() {
			key := "pipe-" + strconv.Itoa(i)
			err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
				obj, err := ws.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				return err
			})
			if err == nil {
				err = hasObjects(ctx, busEngine, key)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// The owner checkpoints its stable history and trims the operation set.
	so := f.space(t)
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	_, err = ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		checkpoint, err := snap.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		return checkpoint.GetHeight() > 0 && set.Len() < sobject.MinCheckpointOperations, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A collection frees nothing a held root, a kept World or the reader's
	// snapshot reaches: the reader still reads every object its snapshot
	// holds.
	f.collect(t)
	for _, key := range []string{"backfill-object"} {
		obj, found, err := snapshot.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("snapshot read of %s after the checkpoint: %v, %v", key, found, err)
		}
	}
	iter := snapshot.IterateObjects(ctx, "", false)
	for iter.Next() {
	}
	err = iter.Err()
	iter.Close()
	if err != nil {
		t.Fatalf("snapshot listing after the checkpoint: %v", err)
	}

	// The replay base holds the checkpoint's World, and once the checkpoint
	// is installed the checkpoint root no longer holds that World.
	heldByCheckpoint := func(world *block.BlockRef) bool {
		ref, err := store.GetRoot(ctx, space_world.RootCheckpoint)
		if err != nil {
			t.Fatal(err)
		}
		if ref.GetEmpty() {
			return false
		}
		data, found, err := store.GetBlock(ctx, ref)
		if err != nil || !found {
			t.Fatalf("checkpoint root block: %v, %v", found, err)
		}
		held := &space_world.HeldRoots{}
		if err := held.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(held.GetRoots(), world.EqualsRef)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		base, err := store.GetRoot(ctx, space_world.RootReplayBase)
		if err != nil {
			t.Fatal(err)
		}
		pending := heldByCheckpoint(base)
		if !base.GetEmpty() && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay base %v and checkpoint %v roots after the checkpoint", base, pending)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A commit above the checkpoint leaves backfill on the checkpoint's
	// World: it copies at checkpoints, not at each head.
	if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "above-checkpoint", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	waitBackfilled(ctx, t, se, store)

	// A remount ends the reader still open as superseded rather than with a
	// missing block, and both a new engine and the bus engine the reader
	// came from read every object after a collection.
	f.collect(t)
	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	f.collect(t)
	obj, _, err := snapshot.GetObject(ctx, "test-object")
	world.ReleaseObjectState(obj)
	if !world.IsSuperseded(err) {
		t.Fatalf("reader read across the remount = %v, want superseded", err)
	}
	keys := []string{"test-object", "backfill-object", "above-checkpoint", "pipe-0", "pipe-" + strconv.Itoa(sobject.MinCheckpointOperations-1)}
	for _, eng := range []world.Engine{eng, busEngine} {
		if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
			return hasObjects(ctx, eng, keys...)
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The collections across the checkpoint advance and the remount freed
	// nothing backfill holds: on the remounted engine, whose choice stays on,
	// a recheck that trusts no proof finds every block of the checkpoint
	// World and the retained root.
	if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		se = eng.(*space_world.Engine)
		_, err := se.GetBackfill().WaitValueWithValidator(ctx, func(on bool) (bool, error) { return on, nil }, nil)
		if err != nil {
			return err
		}
		return se.RecheckBackfill(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	waitBackfilled(ctx, t, se, store)
	if !backfillHolds(ctx, t, store, retained) {
		t.Fatal("backfill dropped the retained root across the remount")
	}

	// Turning it off clears its state and releases what it held.
	if err := se.SetBackfill(ctx, false); err != nil {
		t.Fatal(err)
	}
	if state := se.GetBackfillState().GetValue(); state != nil {
		t.Fatalf("backfill state after turning it off: %v", state)
	}
	for name, want := range map[string]bool{space_world.RootRetained: true, space_world.RootBackfill: false} {
		ref, err := store.GetRoot(ctx, name)
		if err != nil || ref.GetEmpty() == want {
			t.Fatalf("root %s = %v, %v; want held %v", name, ref, err, want)
		}
	}
	if f.frees {
		deleteSpace(ctx, t, f, engRef)
	}
}

// deleteSpace deletes f's Space beside a second Space on the account, and
// checks that a collection frees the deleted Space's blocks and keeps every
// object of the other.
func deleteSpace(ctx context.Context, t *testing.T, f *adapterFixture, engRef directive.Reference) {
	// Write an object to a second Space.
	t.Helper()
	other, err := f.acc.CreateSharedObject(ctx, "other-space", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	otherEng, otherRef, err := world.StartEngine(ctx, f.tb.Bus, space_world.NewConfig("other-space-world", other), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer otherRef.Release()
	if err := world.ExecTransaction(ctx, otherEng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "other-object", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Delete the first Space and collect.
	base, err := f.space(t).GetBlockStore().GetRoot(ctx, space_world.RootReplayBase)
	if err != nil || base.GetEmpty() {
		t.Fatalf("replay base before the deletion: %v, %v", base, err)
	}
	engRef.Release()
	if err := f.acc.DeleteSharedObject(ctx, f.ref.GetProviderResourceRef().GetId()); err != nil {
		t.Fatal(err)
	}
	f.collect(t)

	// The deleted Space's World is gone, and the other Space reads its
	// object after a remount.
	if found, err := f.volume.GetBlockExists(ctx, base); err != nil || found {
		t.Fatalf("the deleted Space's World root after a collection: %v, %v", found, err)
	}
	otherRef.Release()
	otherEng, otherRef, err = world.StartEngine(ctx, f.tb.Bus, space_world.NewConfig("other-space-world", other), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer otherRef.Release()
	if err := world.AccessEngine(ctx, otherEng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, "other-object")
	}); err != nil {
		t.Fatal(err)
	}
}
