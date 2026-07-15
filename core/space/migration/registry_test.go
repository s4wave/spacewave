package space_migration_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	space_migration "github.com/s4wave/spacewave/core/space/migration"
	objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
)

func TestBuiltInsAreClassifiedAndBoundToCentralInventory(t *testing.T) {
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	central := objecttypes.BuiltInObjectTypeIDs()
	if !slices.Equal(central, registry.TypeIDs()) {
		t.Fatalf("migration registry diverges from central ObjectType inventory: central=%v migration=%v", central, registry.TypeIDs())
	}
	for _, typeID := range central {
		handler := registry.Lookup(typeID)
		if handler == nil {
			t.Fatalf("built-in type %q has no handler", typeID)
		}
		if handler.Classification() == space_migration.ClassificationUnclassified {
			t.Fatalf("built-in type %q is unclassified", typeID)
		}
	}
}

func TestSchemaHandlersRejectSyntheticMetadata(t *testing.T) {
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, typeID := range []string{"spacewave/secret", "canvas"} {
		handler := registry.Lookup(typeID)
		if handler == nil {
			t.Fatalf("handler %q is missing", typeID)
		}
		_, err := handler.Rewrite(context.Background(), &space_migration.ObjectDescriptor{
			ObjectKey:  typeID,
			ObjectType: typeID,
			References: []space_migration.TypedReference{{Kind: space_migration.ReferenceExternal, Value: "synthetic"}},
		}, space_migration.NewIdentityMap())
		if !errors.Is(err, space_migration.ErrPayloadSchemaRefused) {
			t.Fatalf("handler %q accepted synthetic metadata: %v", typeID, err)
		}
	}
}
