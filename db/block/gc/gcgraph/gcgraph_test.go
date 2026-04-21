//go:build js

package gcgraph

import (
	"context"
	"slices"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/opfs"
)

func newTestGraph(t *testing.T, name string) (*GCGraph, func()) {
	t.Helper()
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, name, true)
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewGCGraph(dir, name)
	if err != nil {
		opfs.DeleteEntry(root, name, true) //nolint
		t.Fatal(err)
	}
	return g, func() { opfs.DeleteEntry(root, name, true) } //nolint
}

func TestGCGraphAddRemoveRef(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-ref")
	defer cleanup()
	ctx := context.Background()

	// Add edge a -> b.
	if err := g.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	// Verify outgoing.
	out, err := g.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(out, "b") {
		t.Errorf("GetOutgoingRefs(a) = %v, want [b]", out)
	}

	// Verify incoming.
	in, err := g.GetIncomingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(in, "a") {
		t.Errorf("GetIncomingRefs(b) = %v, want [a]", in)
	}

	// HasIncomingRefs (excluding unreferenced).
	has, err := g.HasIncomingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("HasIncomingRefs(b) = false, want true")
	}

	// Remove edge.
	if err := g.RemoveRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	out2, err := g.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(out2) != 0 {
		t.Errorf("after remove, GetOutgoingRefs(a) = %v, want []", out2)
	}
}

func TestGCGraphNodeInventory(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-nodes")
	defer cleanup()
	ctx := context.Background()

	// Adding refs should create node inventory entries.
	if err := g.AddRef(ctx, "x", "y"); err != nil {
		t.Fatal(err)
	}
	nodes, err := g.IterateNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(nodes, "x") || !slices.Contains(nodes, "y") {
		t.Errorf("IterateNodes = %v, want [x, y]", nodes)
	}

	// RemoveNode.
	if err := g.RemoveNode(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	nodes2, err := g.IterateNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(nodes2, "x") {
		t.Errorf("after RemoveNode, x still in inventory: %v", nodes2)
	}
}

func TestGCGraphRootSet(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-roots")
	defer cleanup()
	ctx := context.Background()

	if err := g.AddRoot(ctx, "root1"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddRoot(ctx, "root2"); err != nil {
		t.Fatal(err)
	}
	roots, err := g.GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 || !slices.Contains(roots, "root1") || !slices.Contains(roots, "root2") {
		t.Errorf("GetRootNodes = %v, want [root1, root2]", roots)
	}

	// RemoveRoot.
	if err := g.RemoveRoot(ctx, "root1"); err != nil {
		t.Fatal(err)
	}
	roots2, err := g.GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots2) != 1 || roots2[0] != "root2" {
		t.Errorf("after RemoveRoot, GetRootNodes = %v, want [root2]", roots2)
	}
}

func TestGCGraphBatchAndOrphan(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch")
	defer cleanup()
	ctx := context.Background()

	// Batch add.
	adds := []block_gc.RefEdge{
		{Subject: "p", Object: "c1"},
		{Subject: "p", Object: "c2"},
		{Subject: "c1", Object: "leaf"},
	}
	if err := g.ApplyRefBatch(ctx, adds, nil); err != nil {
		t.Fatal(err)
	}
	out, err := g.GetOutgoingRefs(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Errorf("GetOutgoingRefs(p) = %v, want 2 targets", out)
	}

	// RemoveNodeRefs with orphan marking.
	targets, err := g.RemoveNodeRefs(ctx, "p", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Errorf("RemoveNodeRefs returned %d targets, want 2", len(targets))
	}

	// c2 should be unreferenced (no other incoming refs).
	unref, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unref, "c2") {
		t.Errorf("c2 not in unreferenced: %v", unref)
	}
	// c1 still has incoming from p removed, but c1->leaf means c1 has no
	// incoming refs either, so c1 should also be unreferenced.
	if !slices.Contains(unref, "c1") {
		t.Errorf("c1 not in unreferenced: %v", unref)
	}
}
