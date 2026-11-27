package world_block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestWorldStateSetGraphQuadValidatesAndDeduplicates checks endpoint validation
// and revision stability when an existing relationship is inserted again, and
// that neither rejection keeps the state from committing.
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

	// Reject a relationship whose subject, its holder, does not exist.
	invalid := world.NewGraphQuadWithKeys("graph-insert/missing", "<graph-insert/relation>", keys[1], "")
	if err := ws.SetGraphQuad(ctx, invalid); !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("relationship with missing subject: got %v, want ErrObjectNotFound", err)
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

	// A rejected relationship wrote nothing, so the state still commits.
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWorldStateSetGraphQuadTargetWrittenLater checks that a relationship may
// reference a target not yet written, reads from the target once it exists,
// and is removed with the target.
func TestWorldStateSetGraphQuadTargetWrittenLater(t *testing.T) {
	// Open a fresh write state holding only the source.
	ctx := context.Background()
	ws, cleanup := setupWorldState(ctx, t)
	defer cleanup()
	source, target := "graph-later/source", "graph-later/target"
	createdObject, err := ws.CreateObject(ctx, source, nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Write the relationship before its target exists.
	q := world.NewGraphQuadWithKeys(source, "<graph-later/relation>", target, "")
	if err := ws.SetGraphQuad(ctx, q); err != nil {
		t.Fatalf("relationship to a target written later: %v", err)
	}
	if found, err := ws.HasObject(ctx, target); err != nil || found {
		t.Fatalf("target exists before it was created: found=%v err=%v", found, err)
	}

	// Create the target and read the relationship from its end.
	createdObject, err = ws.CreateObject(ctx, target, nil)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	incoming := world.NewGraphQuad("", "", world.KeyToGraphValue(target).String(), "")
	quads, err := ws.LookupGraphQuads(ctx, incoming, 0)
	if err != nil || len(quads) != 1 || quads[0].GetSubject() != q.GetSubject() {
		t.Fatalf("incoming relationship of the created target: quads=%v err=%v", quads, err)
	}

	// Delete the target and require the relationship gone.
	deleted, err := ws.DeleteObject(ctx, target)
	if err != nil || !deleted {
		t.Fatalf("delete target: deleted=%v err=%v", deleted, err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	quads, err = ws.LookupGraphQuads(ctx, q, 0)
	if err != nil || len(quads) != 0 {
		t.Fatalf("relationship survived its target's deletion: quads=%v err=%v", quads, err)
	}
}
