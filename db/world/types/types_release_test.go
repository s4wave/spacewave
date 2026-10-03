package world_types_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// releaseTestBlock is a minimal block for lifecycle tests.
type releaseTestBlock struct {
	Value string
}

func (b *releaseTestBlock) MarshalBlock() ([]byte, error) {
	return []byte(b.Value), nil
}

func (b *releaseTestBlock) UnmarshalBlock(data []byte) error {
	b.Value = string(data)
	return nil
}

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

// TestListCollectObjectsWithTypeReleasesStates fails if the typed batch
// collection leaves its remote-releasable ObjectState handles alive.
func TestListCollectObjectsWithTypeReleasesStates(t *testing.T) {
	// Use one context for the World lifecycle checks.
	ctx := context.Background()

	// Start a World testbed and release its resources after the test.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Store two typed object bodies for the successful collection path.
	const typeID = "test/release-check-block"
	store := func(ws world.WorldState, key, value string) {
		// Create the test object body and release its acquired state.
		t.Helper()
		{
			createdObject, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
				bcs.SetBlock(&releaseTestBlock{Value: value}, true)
				return nil
			})
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatalf("CreateWorldObject %s: %v", key, err)
			}
		}

		// Link the stored object to the test type.
		if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
			t.Fatalf("SetObjectType %s: %v", key, err)
		}
	}
	store(tb.WorldState, "test/one", "one")
	store(tb.WorldState, "test/two", "two")

	// Count state releases through a wrapper around the World testbed.
	var released int
	cws := &countingWorldState{WorldState: tb.WorldState, released: &released}
	ctor := func() block.Block { return &releaseTestBlock{} }

	// Collect both typed bodies through the release-counting World wrapper.
	objs, objKeys, err := world_types.ListCollectObjectsWithType[*releaseTestBlock](ctx, cws, typeID, ctor)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the collected bodies and keys and the successful state releases.
	if len(objs) != 2 || objs[0].Value != "one" || objs[1].Value != "two" {
		t.Fatalf("unexpected bodies: %#v", objs)
	}
	if len(objKeys) != 2 {
		t.Fatalf("unexpected keys: %#v", objKeys)
	}
	if released != 2 {
		t.Fatalf("success path: released %d states, want 2", released)
	}

	// CollectObjectBodies returns ErrNotFound for a missing key and hands the
	// found states to the caller, which releases them.
	objs2, states, err := world.CollectObjectBodies[*releaseTestBlock](
		ctx,
		cws,
		[]string{"test/one", "test/gone"},
		ctor,
	)

	// Verify the missing body is reported while the found body remains available.
	if err == nil {
		t.Fatal("expected error")
	}
	if len(objs2) != 2 || objs2[0].Value != "one" || objs2[1] != nil {
		t.Fatalf("unexpected bodies on not-found: %#v", objs2)
	}

	// Release the partial collection and verify the final state count.
	for _, state := range states {
		world.ReleaseObjectState(state)
	}
	if released != 3 {
		t.Fatalf("not-found path: released %d states total, want 3", released)
	}
}
