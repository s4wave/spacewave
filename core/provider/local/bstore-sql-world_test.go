//go:build !js

package provider_local

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// sqlWorldSO uses the real SQL publisher and provider state host. Only the SQL
// service boundary is in memory; account metadata uses a real temporary volume.
type sqlWorldSO struct {
	*SharedObject
	// store supplies the SQL-primary block domain for the mounted engine.
	store *sqlWorldStore
	// waiting observes intent waits entered through the public engine path.
	waiting chan struct{}
}

// ReserveStateWrite observes a real wait entered after releasing the engine writer.
func (s *sqlWorldSO) ReserveStateWrite(ctx context.Context, settled bool) (sobject.StateWriteReservation, error) {
	select {
	case s.waiting <- struct{}{}:
	default:
	}
	return s.SharedObject.ReserveStateWrite(ctx, settled)
}

// GetBlockStore supplies the SQL-primary store to the real World engine.
func (s *sqlWorldSO) GetBlockStore() bstore.BlockStore { return s.store }

// sqlWorldStore observes blocking claims and can fail one physical state head.
type sqlWorldStore struct {
	*BlockStore
	// entered observes capacity waits through the production store interface.
	entered chan struct{}
	// head injects one physical state-head failure at the SQL boundary.
	head atomic.Pointer[sqlWorldHeadGate]
	// decline rejects one atomic submission before it acquires any publication.
	decline atomic.Bool
}

// sqlWorldHeadGate holds one state head before its injected failure.
type sqlWorldHeadGate struct {
	entered, release chan struct{}
	err              error
}

// sqlWorldMount supplies the fixture through the engine's normal mount directive.
type sqlWorldMount struct{ so sobject.SharedObject }

// GetControllerInfo identifies the fixture's mount provider.
func (m *sqlWorldMount) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/sql-world-mount", controller.MustParseVersion("0.0.1"), "SQL World mount")
}

// Execute has no work beyond resolving mounts.
func (m *sqlWorldMount) Execute(context.Context) error { return nil }

// Close releases no resource; the fixture owns the mounted SharedObject.
func (m *sqlWorldMount) Close() error { return nil }

// HandleDirective resolves the fixture's alias without altering the real provider.
func (m *sqlWorldMount) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if d, ok := di.GetDirective().(sobject.MountSharedObject); ok && d.MountSharedObjectRef().GetProviderResourceRef().GetProviderId() == "sql-world-test" {
		return directive.R(directive.NewValueResolver([]sobject.SharedObject{m.so}), nil)
	}
	return nil, nil
}

// ReservePublication observes the real capacity wait after the engine unlocked.
func (s *sqlWorldStore) ReservePublication(ctx context.Context, size int64) (block.PublicationReservation, error) {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	return s.BlockStore.ReservePublication(ctx, size)
}

// SubmitAtomic injects a physical head failure through the real publisher.
func (s *sqlWorldStore) SubmitAtomic(ctx context.Context, pub *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// A declined submission returns ownership to the real engine retry path.
	if pub.Root != nil && s.decline.Swap(false) {
		return nil, block.ErrAtomicPublicationUnsupported
	}

	if gate := s.head.Swap(nil); gate != nil {
		copy := *pub
		copy.Heads = append([]*block.AtomicHeadUpdate(nil), pub.Heads...)
		head := *copy.Heads[0]
		head.Replace = func(ctx context.Context, _ []byte, _ bool) ([]byte, error) {
			close(gate.entered)
			select {
			case <-gate.release:
				return nil, gate.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		copy.Heads[0] = &head
		pub = &copy
	}
	return s.BlockStore.SubmitAtomic(ctx, pub)
}

// TestSQLWorldReservationLifecycle walks full-queue cancellation, an overtaken
// failed base and stable checkpoints through the real World and state host.
func TestSQLWorldReservationLifecycle(t *testing.T) {
	// Exercise the opt-in path and restore the default for other fixtures.
	SetSQLPublicationReservations(true)
	t.Cleanup(func() { SetSQLPublicationReservations(false) })

	// Open a local account and SharedObject on a temporary native volume.
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewS4db(false, t.TempDir())))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: "local", PeerId: tb.Volume.GetPeerID().String(), StorageId: tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	// Open the local provider account for the fixture.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "sql-world", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRef.Release)

	// Create the SharedObject through the provider feature.
	feature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := feature.CreateSharedObject(ctx, "world", &sobject.SharedObjectMeta{BodyType: "world"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Mount the normal host before attaching the SQL boundary.
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(soRef.Release)
	local := so.(*SharedObject)
	original := local.GetBlockStore().(*BlockStore)

	// Compose the production SQL read/write chain over an in-memory SQL boundary.
	sql := newBatchForwardTestStore()
	p := newSQLPublisher(tb.Logger, sql, original.placement.local.(block.AtomicPublisher))
	write, lower := sqlPrimaryStores(original.GetID(), original.store, sql, func() block.StoreOps { return nil })
	reads := newSQLPublishReads(p, lower)
	store := &sqlWorldStore{BlockStore: &BlockStore{
		store: write, readStore: block_store.NewStore(original.GetID(), reads), copyStore: reads,
		decodedBlocks: original.decodedBlocks,
		placement:     &placementState{local: original.placement.local, sqlPrimary: true, sqlPublish: p},
	}, entered: make(chan struct{}, 8)}

	// Run the SQL drain and head committer for the fixture lifetime.
	pubCtx, stopPub := context.WithCancel(ctx)
	pubDone := make(chan struct{})
	go func() { p.run(pubCtx); close(pubDone) }()
	t.Cleanup(func() { stopPub(); <-pubDone })

	// Serve the actual engine and its ordinary mock object operations.
	alias := ref.CloneVT()
	alias.ProviderResourceRef.ProviderId = "sql-world-test"
	mounted := &sqlWorldSO{SharedObject: local, store: store, waiting: make(chan struct{}, 8)}
	stopMount, err := tb.Bus.AddController(ctx, &sqlWorldMount{so: mounted}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopMount)

	// Build the engine with ordinary object operations.
	conf := sobject_world_engine.NewConfig("sql-world", alias)
	conf.InitWorldOp = &sobject_world_engine.InitWorldOp{LastChangeDisable: true}
	conf.StoreEveryWorld = true
	ctrl, err := sobject_world_engine.NewController(tb.Logger, tb.Bus, conf, transform_all.BuildFactorySet())
	if err != nil {
		t.Fatal(err)
	}
	ctrl.SetOpTypes(sobject_world_engine.OpTypes{Lookup: world_mock.LookupMockOp})

	// Serve the fixture through the public controller entry point.
	engCtx, stopEng := context.WithCancel(ctx)
	engDone := make(chan error, 1)
	go func() { engDone <- ctrl.Execute(engCtx) }()
	stopEngine := sync.OnceFunc(func() { stopEng(); <-engDone })
	t.Cleanup(stopEngine)
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Build writes through the same World interface as callers.
	newWrite := func(key string) world.Tx {

		// Open a write and create its object.
		t.Helper()
		tx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		obj, err := tx.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "objects"})
		world.ReleaseObjectState(obj)
		if err != nil {
			tx.Discard()
			t.Fatal(err)
		}
		return tx
	}

	// Commit the initial durable World before filling capacity.
	commit := func(key string) {
		t.Helper()
		if err := newWrite(key).Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	commit("before")

	// A declined publication drops its locks and claims before preparing a
	// settled retry, without discarding the candidate's borrowed blocks.
	store.decline.Store(true)
	commit("declined-retry")

	// A direct state writer's intent cannot make a World or control writer
	// keep the engine locked while waiting for it.
	for _, control := range []bool{false, true} {
		intent, err := local.ReserveStateWrite(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		waiting, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		if control {
			go func() { done <- eng.(world.RootRetainingEngine).SetRetainedRoot(waiting, "probe", nil) }()
		} else {
			tx := newWrite("intent-canceled")
			go func() { done <- tx.Commit(waiting) }()
		}
		select {
		case <-mounted.waiting:
		case <-time.After(5 * time.Second):
			intent.Release()
			t.Fatal("state intent wait not reached")
		}
		free, stop := context.WithTimeout(ctx, time.Second)
		err = eng.(sobject_world_engine.BackfillEngine).SetBackfill(free, false)
		stop()
		cancel()
		waitErr := <-done
		intent.Release()
		if err != nil {
			t.Fatalf("state intent wait held engine: %v", err)
		}
		if !errors.Is(waitErr, context.Canceled) {
			t.Fatalf("state intent cancellation: %v", waitErr)
		}
	}

	// Fill capacity and cancel a waiting candidate while another writer enters.
	var claims []*sqlReservation
	fill := func() {
		t.Helper()
		for p.getStats().Publications+len(claims) < sqlPublishMaxPending {
			claim, err := p.reserve(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			claims = append(claims, claim)
		}
	}
	clear := func() {
		for _, claim := range claims {
			claim.Release()
		}
		claims = nil
	}
	defer clear()
	fill()

	// Start a candidate whose queue claim cannot fit yet.
	cancelCtx, cancel := context.WithCancel(ctx)
	tx := newWrite("canceled")
	result := make(chan error, 1)
	go func() { result <- tx.Commit(cancelCtx) }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("candidate did not wait for capacity")
	}

	// A second writer can enter while the first waits for capacity.
	freeCtx, freeCancel := context.WithTimeout(ctx, time.Second)
	err = eng.(sobject_world_engine.BackfillEngine).SetBackfill(freeCtx, false)
	freeCancel()
	if err != nil {
		t.Fatalf("capacity wait held the engine writer: %v", err)
	}

	// The publisher still bounds prepared candidates while that writer is free.
	boundCtx, boundCancel := context.WithTimeout(ctx, 10*time.Millisecond)
	other, err := eng.NewTransaction(boundCtx, true)
	boundCancel()
	if err == nil {
		other.Discard()
		t.Fatal("a second candidate escaped the preparation bound")
	}

	// Cancel the waiting candidate before freeing any capacity.
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled candidate = %v", err)
	}
	clear()

	// Fail a predecessor while its successor waits outside the writer lock.
	injected := errors.New("physical state head failed")
	gate := &sqlWorldHeadGate{entered: make(chan struct{}), release: make(chan struct{}), err: injected}
	store.head.Store(gate)
	releaseHead := sync.OnceFunc(func() { close(gate.release) })

	// Admit the predecessor but hold its physical state replacement.
	first, err := world.CommitAdmit(ctx, newWrite("failed-base"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	defer releaseHead()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("head gate not reached")
	}

	// Fill the remaining capacity and wait with a dependent candidate.
	fill()
	tx = newWrite("failed-successor")
	go func() { result <- tx.Commit(ctx) }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("successor did not wait")
	}

	// Fail the predecessor and reject the candidate prepared on it.
	releaseHead()
	if _, err := first.WaitDurable(ctx); !errors.Is(err, injected) {
		t.Fatalf("first outcome = %v", err)
	}
	if err := <-result; !errors.Is(err, block.ErrPublicationDependency) {
		t.Fatalf("successor outcome = %v", err)
	}
	clear()
	commit("after")

	// Enough successful writes checkpoint without the page view.
	for i := 0; i < 2*sobject.MinCheckpointOperations; i++ {
		commit("checkpoint/" + strconv.Itoa(i))
	}
	durable, release, err := local.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Wait for the provider to make a stable checkpoint durable.
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	_, err = durable.WaitValueWithValidator(waitCtx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		checkpoint, err := snap.GetCheckpoint(waitCtx)
		return checkpoint.GetHeight() > 0, err
	}, nil)
	if err != nil {
		t.Fatalf("checkpoint adoption: %v", err)
	}

	// No canceled or failed candidate is invented in the acknowledged World.
	read, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for _, key := range []string{"before", "after", "canceled", "intent-canceled", "failed-base", "failed-successor"} {
		obj, found, err := read.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || found != (key == "before" || key == "after") {
			t.Fatalf("object %s = %v, %v", key, found, err)
		}
	}

	// Retiring an engine cancels an unadmitted wait even with a live caller.
	read.Discard()
	for draining := true; draining; {
		select {
		case <-store.entered:
		default:
			draining = false
		}
	}
	fill()
	tx = newWrite("retired")
	go func() { result <- tx.Commit(ctx) }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("retired candidate did not wait")
	}

	// Retirement must return a still unadmitted candidate without freeing capacity.
	stopEngine()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("retired engine acknowledged an unadmitted candidate")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retired engine retained its capacity waiter")
	}
	clear()
}
