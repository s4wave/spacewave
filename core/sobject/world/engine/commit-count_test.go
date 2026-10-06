//go:build !js

package sobject_world_engine_test

import (
	"strconv"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/s4db"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// localWorld is the World engine of a SharedObject in a local provider
// account whose volume is an s4db database.
type localWorld struct {
	// eng is the World engine.
	eng sobject_world_engine.Engine
	// db is the account volume's database.
	db *s4db.DB
	// soRef identifies the SharedObject.
	soRef *sobject.SharedObjectRef
}

// startLocalWorld starts a local provider on s4db storage under dir and the
// World engine of the SharedObject soRef in its account, creating the
// SharedObject when soRef is nil. Test cleanup stops them.
func startLocalWorld(t *testing.T, dir string, soRef *sobject.SharedObjectRef) *localWorld {
	// Start a testbed on s4db storage.
	t.Helper()
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewS4db(false, dir)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Run the local provider and the World engine factory on it.
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	// Open the account and its SharedObject.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "local-world-account", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRef.Release)
	if soRef == nil {
		soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
		if err != nil {
			t.Fatal(err)
		}
		soRef, err = soProv.CreateSharedObject(ctx, "local-world", &sobject.SharedObjectMeta{BodyType: "local-world"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
	}

	// Start its World engine with the mock operations.
	engineID := "local-world-engine"
	ctrl, _, ctrlRef, err := sobject_world_engine.StartEngineWithConfig(ctx, tb.Bus, sobject_world_engine.NewConfig(engineID, soRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrlRef.Release)
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("local-world-ops", engineID, world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relOpc)

	// Wait for the engine and find the account database.
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db := volume_s4db.GetDB(acc.(*provider_local.ProviderAccount).GetVolume())
	if db == nil {
		t.Fatal("account volume is not s4db")
	}
	return &localWorld{eng: eng, db: db, soRef: soRef}
}

// create commits a World write that creates the object key.
func (w *localWorld) create(t *testing.T, key string) {
	// Build the write and commit it.
	t.Helper()
	tx := w.newWrite(t, key)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// newWrite returns an uncommitted World write that creates the object key.
func (w *localWorld) newWrite(t *testing.T, key string) world.Tx {
	// Open a write and create the object in it.
	t.Helper()
	tx, err := w.eng.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(t.Context(), key, &bucket.ObjectRef{BucketId: "local-world-bucket"})
	world.ReleaseObjectState(obj)
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	return tx
}

// seq returns the sequence of the last database commit.
func (w *localWorld) seq() uint64 {
	return w.db.Seq()
}

// TestLocalWorldCommitPhysicalCommits checks that one local SharedObject World
// commit is one database commit: the blocks, the operation, the
// accepted World and the replay save publish together.
func TestLocalWorldCommitPhysicalCommits(t *testing.T) {
	// Commit one object per transaction, after a warm-up commit.
	w := startLocalWorld(t, t.TempDir(), nil)
	w.create(t, "commit-count/0")
	const n = 8
	before := w.seq()
	for i := 1; i <= n; i++ {
		w.create(t, "commit-count/"+strconv.Itoa(i))
	}
	if got := w.seq() - before; got != n {
		t.Fatalf("%d World commits made %d physical commits", n, got)
	}
}
