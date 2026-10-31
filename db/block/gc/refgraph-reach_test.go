package block_gc

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestReachable checks that the walk keeps the roots and everything they
// reach, and leaves out garbage and the unreferenced queue.
func TestReachable(t *testing.T) {
	// Open a graph with helpers for block nodes and edges.
	ctx := context.Background()
	rg := newTestRefGraph(t)
	block := func(name string) string { return BlockIRI(testBlockRef(t, name)) }
	addRef := func(subject, object string) {
		t.Helper()
		if err := rg.AddRef(ctx, subject, object); err != nil {
			t.Fatal(err)
		}
	}

	// A diamond under the graph root: a bucket owns two blocks sharing a leaf.
	addRef(NodeGCRoot, "bucket")
	addRef("bucket", block("left"))
	addRef("bucket", block("right"))
	addRef(block("left"), block("leaf"))
	addRef(block("right"), block("leaf"))

	// A registered root with a chain longer than a chunk, so the walk pops
	// several chunks and the set grows past its first table.
	addTestRoot(t, rg, "object")
	const chain = 3 * reachChunk
	edges := make([]RefEdge, 0, chain)
	prev := "object"
	for i := range chain {
		next := block("chain-" + strconv.Itoa(i))
		edges = append(edges, RefEdge{Subject: prev, Object: next})
		prev = next
	}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Garbage owned by nothing, and a candidate only the unreferenced queue
	// names.
	addRef(block("garbage"), block("garbage-leaf"))
	addRef(NodeUnreferenced, block("candidate"))
	addRef(block("candidate"), block("candidate-leaf"))

	// Walk the graph and check each node's membership and the count.
	mark, err := rg.Reachable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reached := mark.Live
	for _, node := range []string{
		NodeGCRoot, "bucket", block("left"), block("right"), block("leaf"),
		"object", block("chain-0"), block("chain-" + strconv.Itoa(chain-1)),
	} {
		if !reached.Has(node) {
			t.Errorf("reachable node %q missing", node)
		}
	}
	for _, node := range []string{
		NodeUnreferenced, block("garbage"), block("garbage-leaf"),
		block("candidate"), block("candidate-leaf"),
	} {
		if reached.Has(node) {
			t.Errorf("unreachable node %q reached", node)
		}
	}
	if want := 6 + chain; reached.Len() != want {
		t.Errorf("reached %d nodes, want %d", reached.Len(), want)
	}

	// Cutting the bucket kind reaches the bucket but none of its blocks,
	// unless another owner names one: the object root also holds the leaf.
	addRef("object", block("leaf"))
	mark, err = rg.ReachableWithout(ctx, "bucket")
	if err != nil {
		t.Fatal(err)
	}
	if !mark.Live.Has("bucket") || !mark.Live.Has(block("leaf")) {
		t.Error("cut walk lost the bucket or the leaf the object holds")
	}
	if mark.Live.Has(block("left")) || mark.Live.Has(block("right")) {
		t.Error("cut walk reached blocks only the bucket holds")
	}
	if want := 4 + chain; mark.Live.Len() != want {
		t.Errorf("cut walk reached %d nodes, want %d", mark.Live.Len(), want)
	}
}

// addTestRoot registers node as a root, as a version 2 graph carried.
func addTestRoot(t *testing.T, rg *RefGraph, node string) {
	t.Helper()
	err := rg.transaction(t.Context(), true, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, graphKey(kindRoot, node), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
}
