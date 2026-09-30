package world_block

import (
	"slices"
	"testing"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestChangedObjectKeys checks that comparing two World roots lists exactly
// the created, changed and deleted objects.
func TestChangedObjectKeys(t *testing.T) {
	// Create three objects and record the World root.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx)
	ws := world.NewEngineWorldState(e, true)
	createKeyWatchTestObject(t, ctx, ws, "diff/kept")
	createKeyWatchTestObject(t, ctx, ws, "diff/changed")
	createKeyWatchTestObject(t, ctx, ws, "diff/deleted")
	before := e.GetRootRef()

	// Create, change and delete one object each.
	createKeyWatchTestObject(t, ctx, ws, "diff/created")
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("diff/changed", "changed"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.DeleteObject(ctx, "diff/deleted"); err != nil {
		t.Fatal(err)
	}
	after := e.GetRootRef()

	// Compare the two roots and expect only the touched objects.
	var keys []string
	var complete bool
	err := e.AccessWorldState(ctx, before, func(beforeCs *bucket_lookup.Cursor) error {
		return e.AccessWorldState(ctx, after, func(afterCs *bucket_lookup.Cursor) error {
			// List the objects that differ between the roots.
			var err error
			keys, complete, err = e.changedObjectKeys(ctx, beforeCs, afterCs)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Expect exactly the created, changed and deleted objects.
	slices.Sort(keys)
	want := []string{"diff/changed", "diff/created", "diff/deleted"}
	if !complete || !slices.Equal(keys, want) {
		t.Fatalf("ChangedObjectKeys = %q complete=%v, want %q", keys, complete, want)
	}

	// Identical roots differ in no object.
	err = e.AccessWorldState(ctx, after, func(cs *bucket_lookup.Cursor) error {
		// Clone the root so both sides are independent cursors.
		same := cs.Clone()
		defer same.Release()

		// Compare the root with itself.
		var err error
		keys, complete, err = e.changedObjectKeys(ctx, cs, same)
		return err
	})
	if err != nil || !complete || len(keys) != 0 {
		t.Fatalf("ChangedObjectKeys(same) = %q complete=%v err=%v, want none", keys, complete, err)
	}
}
