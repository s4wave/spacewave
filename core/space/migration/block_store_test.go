package space_migration_test

import (
	"context"
	"errors"
	"testing"

	sobject "github.com/s4wave/spacewave/core/sobject"
	space_migration "github.com/s4wave/spacewave/core/space/migration"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

func TestPlannerBlocksTypedBlockStoreMismatch(t *testing.T) {
	// Open source and destination Worlds for block-store mismatch planning.
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

	// Store a Secret envelope that names a different block store.
	setObjectBlock(t, ctx, source.WorldState, "secret", s4wave_secret.SecretTypeID, &s4wave_secret.Secret{
		NestedSharedObjectId: "nested-secret",
		Ref:                  &sobject.SharedObjectRef{BlockStoreId: "wrong-store"},
	})

	// Construct the built-in registry for the Secret migration.
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Plan migration of the Secret with its mismatched block-store identity.
	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID: "store-source", DestinationSpaceID: "store-destination",
		Source: source.WorldState, Destination: destination.WorldState,
		SelectedObjectKeys: []string{"secret"},
	})

	// Verify the preview blocks the mismatched Secret block store.
	if !errors.Is(err, space_migration.ErrPlanBlocked) {
		t.Fatalf("mismatch error = %v, want ErrPlanBlocked", err)
	}
	found := false
	for _, blocker := range preview.GetBlockers() {
		if blocker.GetCode() == "block-store-mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing block-store mismatch blocker: %#v", preview.GetBlockers())
	}
}
