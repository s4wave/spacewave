package world_block_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestWorldStateSetGraphQuadValidatesAndDeduplicates checks endpoint validation
// and revision stability when an existing relationship is inserted again.
func TestWorldStateSetGraphQuadValidatesAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	ws, cleanup := setupWorldWriteBench(ctx, t)
	defer cleanup()

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

	invalid := world.NewGraphQuadWithKeys(keys[0], "<graph-insert/relation>", "graph-insert/missing", "")
	if err := ws.SetGraphQuad(ctx, invalid); err == nil {
		t.Fatal("relationship with missing endpoint succeeded")
	}
	quads, err := ws.LookupGraphQuads(ctx, invalid, 1)
	if err != nil || len(quads) != 0 {
		t.Fatalf("invalid relationship was inserted: quads=%v err=%v", quads, err)
	}

	q := world.NewGraphQuadWithKeys(keys[0], "<graph-insert/relation>", keys[1], "")
	if err := ws.SetGraphQuad(ctx, q); err != nil {
		t.Fatal(err)
	}

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
