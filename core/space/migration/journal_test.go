package space_migration_test

import (
	"context"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestMigrationJournalRoundTripRetainsCompletePreview(t *testing.T) {
	// Open source and destination Worlds for journal preview planning.
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

	// Populate the source World and construct the migration registry.
	setObject(t, ctx, source.WorldState, "journal-root", s4wave_kv_world.KvStoreTypeID)
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Plan the immutable preview retained by the migration journal.
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "journal-source", DestinationSpaceID: "journal-destination",
		Source: source.WorldState, Destination: destination.WorldState,
		SelectedObjectKeys: []string{"journal-root"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a migration journal from the complete preview.
	journal, err := space_migration.NewMigrationJournal("journal-op", preview, 123)
	if err != nil {
		t.Fatal(err)
	}

	// Serialize and decode the migration journal protobuf.
	data, err := journal.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(space_migration.MigrationJournal)
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	// Verify the journal retains the preview identity, contents, and block stores.
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
