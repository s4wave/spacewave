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
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	engine, err := core_sync.Open(ctx, le, storage_native.NewBoltDB(false, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := createKvStoreObjectIn(ctx, engine.State, "counts"); err != nil {
		t.Fatal(err)
	}
	object, err := world.MustGetObject(ctx, engine.State, "counts")
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := object.GetRootRef(ctx)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	write, err := engine.World.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer write.Discard()
	if err := commitKvThroughFactory(ctx, le, engine.Bus, engine.World, write, "counts", "changed", "value"); err != nil {
		t.Fatal(err)
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := engine.World.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	resource := resource_world.NewWorldStateResource(le, engine.Bus, read, nil)
	result, err := resource.CompareObjectRecords(ctx, &sdk_world.CompareObjectRecordsRequest{Bases: []*sdk_world.ObjectRecordBase{{ObjectKey: "counts", RootRef: base}}})
	if err != nil {
		t.Fatal(err)
	}
	changes := result.GetChanges()
	if len(changes) != 1 || changes[0].GetUnknown() || len(changes[0].GetKeys()) != 1 || string(changes[0].GetKeys()[0]) != "changed" {
		t.Fatalf("changes = %#v", changes)
	}
}
