package world_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestRenameGraphFieldBoundaries preserves distinct quads with embedded NULs.
func TestRenameGraphFieldBoundaries(t *testing.T) {
	// Create every endpoint in the existing World engine testbed.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	for _, key := range []string{"a", "b", "x>\x00<b"} {
		object, err := writer.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}

	// These different field tuples collide when joined with a NUL separator.
	quads := []world.GraphQuad{
		world.NewGraphQuadWithKeys("a", "<p>\x00<x>", "b", ""),
		world.NewGraphQuadWithKeys("a", "<p>", "x>\x00<b", ""),
	}
	for _, q := range quads {
		if err := writer.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writer.Discard()

	// Rename their shared subject through another committed transaction.
	writer, err = engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	object, err := writer.RenameObject(ctx, "a", "renamed", false)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	writer.Discard()

	// A fresh snapshot must have no relationship referring to the old subject.
	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	old, err := reader.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("a", "", "", ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 0 {
		t.Errorf("old subject retains %d relationships, want none", len(old))
	}

	// Both complete tuples must survive under the renamed subject.
	for _, before := range quads {
		filter := world.NewGraphQuad("<renamed>", before.GetPredicate(), before.GetObj(), before.GetLabel())
		matches, err := reader.LookupGraphQuads(ctx, filter, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Errorf("renamed relationship %q %q count = %d, want 1", before.GetPredicate(), before.GetObj(), len(matches))
		}
	}
}
