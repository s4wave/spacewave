package world_block_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestWorldStateSetGraphQuadValidatesAndDeduplicates checks endpoint validation
// and revision stability when an existing relationship is inserted again.
func TestWorldStateSetGraphQuadValidatesAndDeduplicates(t *testing.T) {
	// Open a fresh write state.
	ctx := context.Background()
	ws, cleanup := setupWorldState(ctx, t)
	defer cleanup()

	// Create the source and target objects in the write state.
	keys := []string{"graph-insert/source", "graph-insert/target"}
	for _, key := range keys {
		{
			createdObject, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Reject a relationship whose endpoint object does not exist.
	invalid := world.NewGraphQuadWithKeys(keys[0], "<graph-insert/relation>", "graph-insert/missing", "")
	if err := ws.SetGraphQuad(ctx, invalid); err == nil {
		t.Fatal("relationship with missing endpoint succeeded")
	}

	// Verify the invalid relationship was not inserted.
	quads, err := ws.LookupGraphQuads(ctx, invalid, 1)
	if err != nil || len(quads) != 0 {
		t.Fatalf("invalid relationship was inserted: quads=%v err=%v", quads, err)
	}

	// Insert a valid relationship between the two objects.
	q := world.NewGraphQuadWithKeys(keys[0], "<graph-insert/relation>", keys[1], "")
	if err := ws.SetGraphQuad(ctx, q); err != nil {
		t.Fatal(err)
	}

	// Record both endpoint revisions before the duplicate insert.
	revisions := make([]uint64, len(keys))
	for i, key := range keys {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		_, revisions[i], err = obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Insert the same relationship again and require unchanged revisions.
	if err := ws.SetGraphQuad(ctx, q); err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		_, revision, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil || revision != revisions[i] {
			t.Fatalf("duplicate changed %s revision: got=%d want=%d err=%v", key, revision, revisions[i], err)
		}
	}
}
