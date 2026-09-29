package world_block_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestUpdateSnapshotPreservesHistoryAndCopiesFinalBlocks verifies replacement,
// removal, relationships, cancellation, and exclusion of superseded writes.
func TestUpdateSnapshotPreservesHistoryAndCopiesFinalBlocks(t *testing.T) {
	// Start a testbed and define a body-writing helper.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)

	// Write an object body through the state's object access.
	write := func(ctx context.Context, state *world_block.WorldState, key, message string) (*bucket.ObjectRef, error) {
		ref, _, err := world.AccessWorldObject(ctx, state, key, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample(message), true)
			return nil
		})
		return ref, err
	}

	// Import the base World with three objects and one relationship.
	base, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects("change", "keep", "remove"), []world.GraphQuad{
		world.NewGraphQuadWithKeys("keep", "<edge>", "remove", ""),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Update the snapshot: replace, add, delete, and refill the graph.
	var superseded *bucket.ObjectRef
	next, err := world_block.UpdateSnapshot(ctx, tb.Logger, tb.Engine, base, func(ctx context.Context, state *world_block.WorldState) error {
		// Write a superseded body first, then the final body for the same object.
		var err error
		superseded, err = write(ctx, state, "change", "superseded")
		if err != nil {
			return err
		}

		// Write the final body for the same object.
		if _, err := write(ctx, state, "change", "final"); err != nil {
			return err
		}

		// Add a new object created by this update.
		if _, err := write(ctx, state, "added", "new"); err != nil {
			return err
		}

		// Delete the removed object.
		if _, err := state.DeleteObject(ctx, "remove"); err != nil {
			return err
		}

		// Refill an emptied graph through the same batch API as initial import,
		// including an endpoint created by this update.
		return state.InsertGraphQuads(ctx, []world.GraphQuad{
			world.NewGraphQuadWithKeys("keep", "<edge>", "change", ""),
			world.NewGraphQuadWithKeys("added", "<edge>", "keep", ""),
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both immutable roots retain their own complete object and graph state.
	for _, ref := range []*bucket.ObjectRef{base, next} {
		// Open each root through the engine's access scope.
		if err := tb.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {

			// Build a read-only WorldState over the root.
			state, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
			if err != nil {
				return err
			}
			defer state.Discard()

			// Verify the change object's body matches each root's revision.
			body, err := world.LookupObjectBody[*block_mock.Example](ctx, state, "change", block_mock.NewExampleBlock)
			if err != nil {
				return fmt.Errorf("read change body from next=%v: %w", ref == next, err)
			}

			// Verify the change object's body matches each root's revision.
			want := "change"
			if ref == next {
				want = "final"
			}
			if body.Msg != want {
				t.Fatalf("body %q, want %q", body.Msg, want)
			}

			// Verify the added object exists only in the updated root.
			if ref == next {
				added, err := world.LookupObjectBody[*block_mock.Example](ctx, state, "added", block_mock.NewExampleBlock)
				if err != nil {
					return fmt.Errorf("read added body: %w", err)
				}
				if added.Msg != "new" {
					t.Fatalf("added body %q, want new", added.Msg)
				}
			}

			// Verify keep survives and remove exists only in the base root.
			objects, err := state.GetObjectRootRefsBatch(ctx, []string{"keep", "remove"})
			if err != nil {
				return fmt.Errorf("read object refs from next=%v: %w", ref == next, err)
			}
			if !objects[0].Exists || objects[1].Exists != (ref == base) {
				t.Fatalf("snapshot changed the wrong object set: base=%v keep=%v remove=%v", ref == base, objects[0].Exists, objects[1].Exists)
			}

			// Verify each root's graph targets the expected object set.
			quads, err := state.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
			if err != nil {
				return fmt.Errorf("read graph from next=%v: %w", ref == next, err)
			}

			// Verify each root's graph targets the expected object set.
			wantObjects := []string{world.KeyToGraphValue("remove").String()}
			if ref == next {
				wantObjects = []string{world.KeyToGraphValue("change").String(), world.KeyToGraphValue("keep").String()}
			}
			gotObjects := make([]string, len(quads))
			for i, quad := range quads {
				gotObjects[i] = quad.GetObj()
			}
			slices.Sort(gotObjects)
			if !slices.Equal(gotObjects, wantObjects) {
				t.Fatalf("relationships target %v, want %v", gotObjects, wantObjects)
			}

			// Verify the superseded body was not durably copied.
			found, err := cursor.GetBucket().GetBlockExists(ctx, superseded.RootRef)
			if found {
				t.Fatal("superseded body was durably copied")
			}
			if err != nil {
				return fmt.Errorf("check superseded body from next=%v: %w", ref == next, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A canceled update returns no candidate and leaves both prior roots intact.

	// Verify a canceled update returns no candidate and leaves both roots intact.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	ref, err := world_block.UpdateSnapshot(canceled, tb.Logger, tb.Engine, next, func(context.Context, *world_block.WorldState) error {
		t.Fatal("canceled update called its mutation")
		return nil
	})
	if err == nil || ref != nil {
		t.Fatalf("canceled update returned ref=%v err=%v", ref, err)
	}
}

// chainQuads links each key to the key after it.
func chainQuads(keys []string) []world.GraphQuad {
	quads := make([]world.GraphQuad, 0, len(keys)-1)
	for i := 1; i < len(keys); i++ {
		quads = append(quads, world.NewGraphQuadWithKeys(keys[i-1], "<edge>", keys[i], ""))
	}
	return quads
}

// BenchmarkUpdateSnapshot measures deleting one eighth of an existing graph,
// including final index packing and durable copying in the in-memory testbed.
func BenchmarkUpdateSnapshot(b *testing.B) {
	// Start a testbed and import a graph of 1024 objects.
	ctx := b.Context()
	tb := world_testbed.MustDefault(b, ctx)
	const size = 1024
	keys := make([]string, size)
	for i := range keys {
		keys[i] = fmt.Sprintf("object-%04d", i)
	}

	// Import the chain graph once and measure repeated one-eighth deletions.
	base, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects(keys...), chainQuads(keys))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		// Import the chain graph once and measure repeated one-eighth deletions.
		_, err := world_block.UpdateSnapshot(ctx, tb.Logger, tb.Engine, base, func(ctx context.Context, state *world_block.WorldState) error {
			// Delete one eighth of the objects in each iteration.
			for _, key := range keys[:size/8] {
				if _, err := state.DeleteObject(ctx, key); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdateSnapshotFixedChange measures one object replacement against
// growing unchanged object and graph indexes.
func BenchmarkUpdateSnapshotFixedChange(b *testing.B) {
	for _, size := range []int{128, 512, 2048} {
		b.Run(fmt.Sprintf("objects-%d", size), func(b *testing.B) {
			// Start a testbed sized for this benchmark case.
			ctx := b.Context()
			tb := world_testbed.MustDefault(b, ctx)
			keys := make([]string, size)
			for i := range keys {
				keys[i] = fmt.Sprintf("object-%05d", i)
			}

			// Import the chain graph once and measure repeated single replacements.
			base, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects(keys...), chainQuads(keys))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, err := world_block.UpdateSnapshot(ctx, tb.Logger, tb.Engine, base, func(ctx context.Context, state *world_block.WorldState) error {
					_, _, err := world.AccessWorldObject(ctx, state, keys[0], true, func(cursor *block.Cursor) error {
						cursor.SetBlock(block_mock.NewExample("changed"), true)
						return nil
					})
					return err
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
