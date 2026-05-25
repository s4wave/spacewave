package space_world

import (
	"context"
	"testing"

	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestLookupSpaceSettingsMissing checks missing settings return nil without error.
func TestLookupSpaceSettingsMissing(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	settings, state, err := LookupSpaceSettings(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	if settings != nil {
		t.Fatalf("expected nil settings, got %#v", settings)
	}
	if state != nil {
		t.Fatalf("expected nil state, got %#v", state)
	}
}
