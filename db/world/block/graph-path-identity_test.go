package world_block

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

// TestGraphPathFieldBoundaries retains distinct traversed quads with embedded NULs.
func TestGraphPathFieldBoundaries(t *testing.T) {
	// Open a writer on the existing World engine testbed.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()

	// Create both endpoints of the path.
	const target = "a>\x00<p"
	for _, key := range []string{"a", target} {
		object, err := writer.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}

	// These two relationships share the old NUL-joined key.
	quads := []world.GraphQuad{
		world.NewGraphQuadWithKeys("a", "<p>\x00<q>", target, ""),
		world.NewGraphQuadWithKeys(target, "<q>", target, ""),
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

	// Traverse both committed relationships through a fresh public World reader.
	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	result, err := reader.QueryGraphPath(ctx, &world.GraphPathQuery{
		StartKeys: []string{"a"},
		Steps: []world.GraphPathStep{
			{Direction: world.GraphPathDirectionOut, Predicate: "<p>\x00<q>", Limit: 4},
			{Direction: world.GraphPathDirectionOut, Predicate: "<q>", Limit: 4},
		},
		ResultLimit:  4,
		IncludeQuads: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.ObjectKeys, []string{target}) {
		t.Errorf("reached keys = %q, want %q", result.ObjectKeys, []string{target})
	}

	// Each traversed predicate belongs to a distinct relationship in the included result.
	if len(result.Quads) != 2 {
		t.Fatalf("included relationships = %d, want 2", len(result.Quads))
	}
	for _, expected := range quads {
		if !slices.ContainsFunc(result.Quads, func(actual world.GraphQuad) bool {
			return actual.GetSubject() == expected.GetSubject() &&
				actual.GetPredicate() == expected.GetPredicate() &&
				actual.GetObj() == expected.GetObj() &&
				actual.GetLabel() == expected.GetLabel()
		}) {
			t.Errorf("missing traversed relationship: %q %q", expected.GetSubject(), expected.GetPredicate())
		}
	}
}
