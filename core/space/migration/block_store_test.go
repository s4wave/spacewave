package space_migration_test

import (
	"context"
	"errors"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
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

// TestSecretRewriteRemapsRefIdentity rewrites a Secret and checks that the ref
// readers and delete mount names the same remapped SharedObject as the id
// field, so the migrated Secret never points at the source Space's payload.
func TestSecretRewriteRemapsRefIdentity(t *testing.T) {
	// Store a Secret whose id field and ref name one nested SharedObject.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	setObjectBlock(t, ctx, tb.WorldState, "secret", s4wave_secret.SecretTypeID, &s4wave_secret.Secret{
		NestedSharedObjectId: "nested-secret",
		Ref: &sobject.SharedObjectRef{
			ProviderResourceRef: &provider.ProviderResourceRef{Id: "nested-secret", ProviderId: "local", ProviderAccountId: "acct"},
			BlockStoreId:        "source-store",
		},
	})

	// Rewrite it under a mapping that renames the nested SharedObject.
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mapping := space_migration.NewIdentityMap()
	mapping.NestedSharedObjects["nested-secret"] = "moved-secret"
	mapping.BlockStoreIDs["source-store"] = "destination-store"
	object := &space_migration.ObjectDescriptor{ObjectKey: "secret", ObjectType: s4wave_secret.SecretTypeID, World: tb.WorldState}
	result, err := registry.Lookup(s4wave_secret.SecretTypeID).Rewrite(ctx, object, mapping)
	if err != nil {
		t.Fatal(err)
	}

	// Verify both identities moved together.
	rewritten := &s4wave_secret.Secret{}
	if err := rewritten.UnmarshalVT(result.Payload); err != nil {
		t.Fatal(err)
	}
	if rewritten.GetNestedSharedObjectId() != "moved-secret" || rewritten.GetRef().GetProviderResourceRef().GetId() != "moved-secret" {
		t.Fatalf("rewrite left the identities apart: %v", rewritten)
	}
}
