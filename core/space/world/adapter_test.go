package space_world_test

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
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
}

// newAdapterFixture creates a Space on a local provider.
func newAdapterFixture(t *testing.T) *adapterFixture {
	return newAdapterFixtureOn(t, nil)
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

// newAdapterFixtureOn creates a Space on a local provider, with its blocks
// on the SQL location as the account's primary store when sql is set.
func newAdapterFixtureOn(t *testing.T, sql *account_settings.SqlLocation, creds ...*block_store_s3.Credentials) *adapterFixture {
	// Build the testbed with the engine and local provider factories.
	t.Helper()
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(space_world.NewFactory(tb.Bus, space_world.OpTypes{Lookup: world_mock.LookupMockOp}))
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
	if sql != nil {
		acc := provAcc.(*provider_local.ProviderAccount)
		acc.SetSQLPrimary(true)
		backendID, check, err := acc.AddSQLStorageBackend(ctx, "sql", sql, creds[0], true)
		if err != nil || backendID == "" {
			t.Fatalf("add the SQL backend: %v %v", check, err)
		}
	}
	ref, err := soProv.CreateSharedObject(ctx, "test-space", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if sql != nil {
		acc := provAcc.(*provider_local.ProviderAccount)
		settings, err := acc.ResolveNewSpaceStorageBackend(ctx, "", false)
		if err != nil {
			t.Fatal(err)
		}
		placement := &account_settings.BlockStorePlacement{BlockStoreId: ref.GetBlockStoreId(), StorageBackendId: settings}
		if err := acc.PlaceBlockStore(ctx, placement); err != nil {
			t.Fatal(err)
		}
	}
	return &adapterFixture{tb: tb, ref: ref, engineID: "test-space-world"}
}

// start starts the engine controller and returns its engine.
func (f *adapterFixture) start(t *testing.T) (*world.BusEngine, directive.Reference) {
	t.Helper()
	eng, ref, err := world.StartEngine(t.Context(), f.tb.Bus, space_world.NewConfig(f.engineID, f.ref), nil)
	if err != nil {
		t.Fatal(err)
	}
	return eng, ref
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

// waitBackfilled waits for a complete backfill of the World at or after its
// current seqno.
func waitBackfilled(ctx context.Context, eng *space_world.Engine) error {
	_, seqno, err := eng.GetHead(ctx)
	if err != nil {
		return err
	}
	_, err = eng.GetBackfillState().WaitValueWithValidator(ctx, func(state *space_world.BackfillState) (bool, error) {
		return state.IsComplete() && !state.GetRoot().GetEmpty() && state.GetSeqno() >= seqno, nil
	}, nil)
	return err
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
	t.Run("sql-primary", func(t *testing.T) {
		location, creds := sqlTestLocation(t)
		runAdapterThrough(t, newAdapterFixtureOn(t, location, creds))
	})
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

	// Backfill copies the installed World and holds it, follows a commit, and
	// clears when turned off; a retained root holds the head it names.
	err := world.AccessEngine(ctx, busEngine, func(ctx context.Context, eng world.Engine) error {
		// Turn backfill on and wait for a copy of the current World.
		se := eng.(*space_world.Engine)
		if err := se.SetBackfill(ctx, true); err != nil {
			return err
		}
		if err := waitBackfilled(ctx, se); err != nil {
			return err
		}

		// Retain the head and wait for a copy that covers the next commit.
		head, _, err := se.GetHead(ctx)
		if err != nil {
			return err
		}
		if err := se.SetRetainedRoot(ctx, "export", head); err != nil {
			return err
		}
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
		if err != nil {
			return err
		}
		if err := waitBackfilled(ctx, se); err != nil {
			return err
		}

		// Turn it off, which clears its state.
		if err := se.SetBackfill(ctx, false); err != nil {
			return err
		}
		if state := se.GetBackfillState().GetValue(); state != nil {
			return errors.Errorf("backfill state after turning it off: %v", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	store := f.space(t).GetBlockStore()
	for name, want := range map[string]bool{space_world.RootRetained: true, space_world.RootBackfill: false} {
		ref, err := store.GetRoot(ctx, name)
		if err != nil || ref.GetEmpty() == want {
			t.Fatalf("root %s = %v, %v; want held %v", name, ref, err, want)
		}
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
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		}()
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

	// The reader still reads every object its snapshot holds.
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

	// The replay base holds the checkpoint's World, and the checkpoint root
	// is released once the checkpoint is installed.
	deadline := time.Now().Add(10 * time.Second)
	for {
		base, berr := store.GetRoot(ctx, space_world.RootReplayBase)
		pending, perr := store.GetRoot(ctx, space_world.RootCheckpoint)
		if berr != nil || perr != nil {
			t.Fatal(berr, perr)
		}
		if !base.GetEmpty() && pending.GetEmpty() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay base %v and checkpoint %v roots after the checkpoint", base, pending)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A remounted engine reads every object.
	engRef.Release()
	_ = eng
	eng, engRef = f.start(t)
	defer engRef.Release()
	keys := []string{"test-object", "pipe-0", "pipe-" + strconv.Itoa(sobject.MinCheckpointOperations-1)}
	if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, keys...)
	}); err != nil {
		t.Fatal(err)
	}
}
