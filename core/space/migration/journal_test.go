package space_migration_test

import (
	"context"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestMigrationJournalRoundTripRetainsCompletePreview(t *testing.T) {
	ctx := context.Background()
	source, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	destination, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Release()
	setObject(t, ctx, source.WorldState, "journal-root", s4wave_kv_world.KvStoreTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "journal-source", DestinationSpaceID: "journal-destination",
		Source: source.WorldState, Destination: destination.WorldState,
		SelectedObjectKeys: []string{"journal-root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	journal, err := space_migration.NewMigrationJournal("journal-op", preview, 123)
	if err != nil {
		t.Fatal(err)
	}
	data, err := journal.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(space_migration.MigrationJournal)
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPreviewDigest() != preview.GetDigest() || decoded.GetPreview() == nil {
		t.Fatalf("journal preview identity lost: %#v", decoded)
	}
	if !decoded.GetPreview().EqualVT(preview) {
		t.Fatal("journal preview differs after protobuf round-trip")
	}
	if decoded.GetSourceBlockStoreId() == "" || decoded.GetDestinationBlockStoreId() == "" {
		t.Fatal("journal omitted derived block-store identities")
	}
}
