package space_world_test

import (
	"context"
	"testing"

	space_world "github.com/s4wave/spacewave/core/space/world"
)

// TestLookupBlockTypeSpaceSettings resolves persisted settings by their block ID.
func TestLookupBlockTypeSpaceSettings(t *testing.T) {
	// Resolve the persisted Space settings block type.
	got, err := space_world.LookupBlockType(context.Background(), "space/settings")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the registered block type constructs SpaceSettings.
	if got == nil {
		t.Fatal("space/settings block type was not found")
	}
	if got.GetBlockTypeID() != "space/settings" {
		t.Fatalf("block type ID = %q, want space/settings", got.GetBlockTypeID())
	}
	if _, ok := got.Constructor().(*space_world.SpaceSettings); !ok {
		t.Fatal("space/settings constructor did not return SpaceSettings")
	}
}

// TestLookupBlockTypeExcludesSQL checks that core does not own the SQL plugin's
// block types. They resolve through the sql plugin's LookupBlockType directive
// handler, so keeping them out of the core lookup is what keeps the sql
// packages out of the core goscript build closure.
func TestLookupBlockTypeExcludesSQL(t *testing.T) {
	ctx := context.Background()
	for _, typeID := range []string{
		"github.com/s4wave/spacewave/sdk/sql/query.Query",
		"github.com/s4wave/spacewave/sdk/sql/workbench.Workbench",
	} {
		got, err := space_world.LookupBlockType(ctx, typeID)
		if err != nil {
			t.Fatalf("LookupBlockType(%s): %v", typeID, err)
		}
		if got != nil {
			t.Fatalf("LookupBlockType(%s) = %T, want nil", typeID, got)
		}
	}
}
