package space_world_test

import (
	"context"
	"testing"

	space_world "github.com/s4wave/spacewave/core/space/world"
)

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
