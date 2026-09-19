package world_block_test

import (
	"context"
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
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	write := func(ctx context.Context, state *world_block.WorldState, key, message string) (*bucket.ObjectRef, error) {
		ref, _, err := world.AccessWorldObject(ctx, state, key, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample(message), true)
			return nil
		})
		return ref, err
	}
	base, err := world_block.BuildSnapshot(ctx, tb.Logger, tb.Engine, func(ctx context.Context, state *world_block.WorldState) error {
		for _, key := range []string{"keep", "change", "remove"} {
			if _, err := write(ctx, state, key, key); err != nil {
				return err
			}
		}
		return state.InsertGraphQuads(ctx, []world.GraphQuad{world.NewGraphQuadWithKeys("keep", "<edge>", "remove", "")})
	})
	if err != nil {
		t.Fatal(err)
	}
	var superseded *bucket.ObjectRef
	next, err := world_block.UpdateSnapshot(ctx, tb.Logger, tb.Engine, base, func(ctx context.Context, state *world_block.WorldState) error {
		var err error
		superseded, err = write(ctx, state, "change", "superseded")
		if err != nil {
			return err
		}
		if _, err := write(ctx, state, "change", "final"); err != nil {
			return err
		}
		_, err = state.DeleteObject(ctx, "remove")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both immutable roots retain their own complete object and graph state.
	for _, ref := range []*bucket.ObjectRef{base, next} {
		if err := tb.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
			state, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
			if err != nil {
				return err
			}
			defer state.Discard()
			body, err := world.LookupObjectBody[*block_mock.Example](ctx, state, "change", block_mock.NewExampleBlock)
			if err != nil {
				return err
			}
			want := "change"
			if ref == next {
				want = "final"
			}
			if body.Msg != want {
				t.Fatalf("body %q, want %q", body.Msg, want)
			}
			objects, err := state.GetObjectRootRefsBatch(ctx, []string{"keep", "remove"})
			if err != nil {
				return err
			}
			if !objects[0].Exists || objects[1].Exists != (ref == base) {
				t.Fatal("snapshot changed the wrong object set")
			}
			quads, err := state.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
			if err != nil {
				return err
			}
			if (len(quads) == 1) != (ref == base) {
				t.Fatal("snapshot did not preserve its relationship set")
			}
			found, err := cursor.GetBucket().GetBlockExists(ctx, superseded.RootRef)
			if found {
				t.Fatal("superseded body was durably copied")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	// A canceled update returns no candidate and leaves both prior roots intact.
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
