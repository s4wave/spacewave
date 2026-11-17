package block_gc

import (
	"context"
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// failingGraph is a RefGraph whose edge read of one node fails.
type failingGraph struct {
	*RefGraph
	fail string
}

// GetOutgoingRefs fails for the failing node.
func (g *failingGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	if node == g.fail {
		return nil, errors.New("unreadable")
	}
	return g.RefGraph.GetOutgoingRefs(ctx, node)
}

// TestOwnedRoots walks every owner kind the graph holds without knowing it:
// a named root other than the head, an unparented write, a reader's pin and
// a stage under it. It reports each block once, leaves a block under another
// block and another bucket's roots out, and fails on an unreadable node.
func TestOwnedRoots(t *testing.T) {
	// Build a graph with a bucket's named roots, a pin and another bucket.
	ctx := t.Context()
	rg := newTestRefGraph(t)
	base, child, write := testBlockRef(t, "replay-base"), testBlockRef(t, "child"), testBlockRef(t, "write")
	pinned, staged, other := testBlockRef(t, "pinned"), testBlockRef(t, "staged"), testBlockRef(t, "other")
	edges := [][2]string{
		{NodeGCRoot, BucketIRI("w")},
		{BucketIRI("w"), "head:replay-base"},
		{BucketIRI("w"), "head:replay-span"},
		{"head:replay-base", BlockIRI(base)},
		{"head:replay-span", BlockIRI(base)},
		{BlockIRI(base), BlockIRI(child)},
		{BucketIRI("w"), BlockIRI(write)},
		{NodeGCRoot, "reader:01"},
		{"reader:01", BlockIRI(pinned)},
		{"reader:01", "stage:01"},
		{"stage:01", BlockIRI(staged)},
		{NodeGCRoot, BucketIRI("o")},
		{BucketIRI("o"), "head:o"},
		{"head:o", BlockIRI(other)},
	}
	for _, e := range edges {
		if err := rg.AddRef(ctx, e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}

	// Walk it, skipping the other bucket.
	skip := func(node string) bool {
		id, ok := ParseBucketIRI(node)
		return ok && id != "w"
	}
	var got []string
	_, err := OwnedRoots(ctx, rg, skip, func(_ string, ref *block.BlockRef) error {
		got = append(got, ref.MarshalString())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{base.MarshalString(), write.MarshalString(), pinned.MarshalString(), staged.MarshalString()}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("owned roots = %v, want %v", got, want)
	}

	// An owner whose edges cannot be read fails the walk.
	bad := &failingGraph{RefGraph: rg, fail: "head:replay-span"}
	if _, err := OwnedRoots(ctx, bad, skip, func(string, *block.BlockRef) error { return nil }); err == nil {
		t.Fatal("a walk with an unreadable owner returned no error")
	}
}
