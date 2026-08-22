package space_world_test

import (
	"context"
	"testing"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// countingWorldState counts ObjectState releases issued through GetObject.
type countingWorldState struct {
	world.WorldState
	released *int
}

func (c *countingWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	obj, found, err := c.WorldState.GetObject(ctx, key)
	if err != nil || !found || obj == nil {
		return obj, found, err
	}
	return &countingObjectState{ObjectState: obj, released: c.released}, true, nil
}

// countingObjectState delegates to the wrapped state and counts Release calls.
type countingObjectState struct {
	world.ObjectState
	released *int
}

func (o *countingObjectState) Release() {
	*o.released++
	world.ReleaseObjectState(o.ObjectState)
}

// TestLookupSpaceSettingsBodyReleasesStates fails if a body-only SpaceSettings
// lookup leaves its remote-releasable ObjectState alive, or if the missing
// settings case stops returning nil without error.
func TestLookupSpaceSettingsBodyReleasesStates(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	var released int
	ws := &countingWorldState{WorldState: tb.WorldState, released: &released}

	// Missing settings still return nil without error.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if settings != nil {
		t.Fatalf("expected nil settings, got %#v", settings)
	}
	if released != 0 {
		t.Fatalf("missing settings: released %d states, want 0", released)
	}

	op := space_world_ops.NewSetSpaceSettingsOp(
		"",
		&space_world.SpaceSettings{IndexPath: "/files"},
		true,
		time.Unix(10, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp settings failed: %v", err)
	}

	settings, err = space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if settings == nil || settings.GetIndexPath() != "/files" {
		t.Fatalf("expected stored settings, got %#v", settings)
	}
	if released != 1 {
		t.Fatalf("present settings: released %d states, want 1", released)
	}
}
