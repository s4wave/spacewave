//go:build !js && !bldr_sqlite

package sync_test

import (
	"testing"

	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	core_sync "github.com/s4wave/spacewave/core/sync"
	"github.com/s4wave/spacewave/db/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestRecordChanges compares immutable record roots through a committed World snapshot.
func TestRecordChanges(t *testing.T) {
	// Open a native sync engine for the record comparison.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	engine, err := core_sync.Open(ctx, le, storage_native.NewS4db(false, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Create the object whose immutable record root will change.
	if err := createKvStoreObjectIn(ctx, engine.State, "counts"); err != nil {
		t.Fatal(err)
	}

	// Capture the initial object root before applying a write.
	object, err := world.MustGetObject(ctx, engine.State, "counts")
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := object.GetRootRef(ctx)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}

	// Open a writable World transaction and retain its cleanup.
	write, err := engine.World.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer write.Discard()

	// Commit a changed key-value record through its registered factory.
	if err := commitKvThroughFactory(ctx, le, engine.Bus, engine.World, write, "counts", "changed", "value"); err != nil {
		t.Fatal(err)
	}

	// Commit the write before comparing immutable roots.
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open a read-only World transaction for the comparison.
	read, err := engine.World.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()

	// Compare the saved root with the current object records.
	resource := resource_world.NewWorldStateResource(le, engine.Bus, read, nil)
	result, err := resource.CompareObjectRecords(ctx, &sdk_world.CompareObjectRecordsRequest{Bases: []*sdk_world.ObjectRecordBase{{ObjectKey: "counts", RootRef: base}}})
	if err != nil {
		t.Fatal(err)
	}

	// Assert that only the changed key appears in the record diff.
	changes := result.GetChanges()
	if len(changes) != 1 || changes[0].GetUnknown() || len(changes[0].GetKeys()) != 1 || string(changes[0].GetKeys()[0]) != "changed" {
		t.Fatalf("changes = %#v", changes)
	}
}
