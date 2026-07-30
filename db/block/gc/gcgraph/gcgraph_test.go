//go:build js

package gcgraph

import (
	"bytes"
	"context"
	"slices"
	"syscall/js"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/opfs"
)

func newTestGraph(t *testing.T, name string) (*GCGraph, func()) {
	t.Helper()
	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		t.Skip("browser OPFS not available")
	}
	storage := navigator.Get("storage")
	if storage.IsUndefined() || storage.IsNull() ||
		storage.Get("getDirectory").Type() != js.TypeFunction {
		t.Skip("browser OPFS not available")
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

func snapshotEdgeDirectory(t *testing.T, g *GCGraph, root js.Value) map[string][]byte {
	t.Helper()
	subdirs, err := opfs.ListDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make(map[string][]byte)
	for _, subdirName := range subdirs {
		snapshot[subdirName+"/"] = nil
		subdir, err := opfs.GetDirectory(root, subdirName, false)
		if err != nil {
			t.Fatal(err)
		}
		names, err := opfs.ListDirectory(subdir)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			data, err := g.readFileContent(subdir, name)
			if err != nil {
				t.Fatal(err)
			}
			snapshot[subdirName+"/"+name] = data
		}
	}
	return snapshot
}

func equalEdgeDirectorySnapshots(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for name, leftData := range left {
		rightData, ok := right[name]
		if !ok || !bytes.Equal(leftData, rightData) {
			return false
		}
	}
	return true
}

func sortedRefs(refs []string) []string {
	refs = slices.Clone(refs)
	slices.Sort(refs)
	return refs
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

func TestApplyRefBatchMissingRemovalPreservesEdgeDirectories(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch-missing")
	defer cleanup()
	ctx := context.Background()

	if err := g.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddRef(ctx, "missing-owner", "other"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddRef(ctx, block_gc.NodeUnreferenced, "staged"); err != nil {
		t.Fatal(err)
	}
	incomingTargetDir, err := opfs.GetDirectory(g.incomingDir, hashName("target"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.writeFile(
		incomingTargetDir,
		hashName("missing-owner"),
		[]byte("missing-owner\ntarget"),
	); err != nil {
		t.Fatal(err)
	}
	beforeForward := snapshotEdgeDirectory(t, g, g.edgesDir)
	beforeIncoming := snapshotEdgeDirectory(t, g, g.incomingDir)
	beforeUnreferenced, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{
		{Subject: "missing-owner", Object: "target"},
		{Subject: "missing-owner", Object: "never-seen"},
	}); err != nil {
		t.Fatal(err)
	}

	afterForward := snapshotEdgeDirectory(t, g, g.edgesDir)
	if !equalEdgeDirectorySnapshots(beforeForward, afterForward) {
		t.Fatal("missing removal changed forward edge directory")
	}
	afterIncoming := snapshotEdgeDirectory(t, g, g.incomingDir)
	if !equalEdgeDirectorySnapshots(beforeIncoming, afterIncoming) {
		t.Fatal("missing removal changed incoming edge directory")
	}
	afterUnreferenced, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedRefs(beforeUnreferenced), sortedRefs(afterUnreferenced)) {
		t.Fatalf(
			"missing removal changed orphan marks: before=%v after=%v",
			beforeUnreferenced,
			afterUnreferenced,
		)
	}
}

func TestApplyRefBatchExistingRemovalCleansPairedEdges(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch-existing")
	defer cleanup()
	ctx := context.Background()

	if err := g.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := g.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{
		Subject: "owner",
		Object:  "target",
	}}); err != nil {
		t.Fatal(err)
	}

	exists, err := g.hasRef("owner", "target")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("existing removal left forward edge file")
	}
	incoming, err := g.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(incoming, []string{block_gc.NodeUnreferenced}) {
		t.Fatalf("existing removal left paired reverse edge: incoming=%v", incoming)
	}
}

func TestApplyRefBatchCollidingAddAndRemoveOrphans(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch-collision")
	defer cleanup()
	ctx := context.Background()

	edge := block_gc.RefEdge{Subject: "owner", Object: "target"}
	if err := g.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, []block_gc.RefEdge{edge}); err != nil {
		t.Fatal(err)
	}

	exists, err := g.hasRef(edge.Subject, edge.Object)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("colliding add and remove left forward edge file")
	}
	unreferenced, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unreferenced, edge.Object) {
		t.Fatalf("colliding add and remove did not mark target orphaned: %v", unreferenced)
	}
}

func TestApplyRefBatchSharedOwnerRemovalKeepsObjectReachable(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch-shared-owner")
	defer cleanup()
	ctx := context.Background()

	for _, owner := range []string{"owner-a", "owner-b"} {
		if err := g.AddRef(ctx, owner, "shared"); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{
		Subject: "owner-a",
		Object:  "shared",
	}}); err != nil {
		t.Fatal(err)
	}

	incoming, err := g.GetIncomingRefs(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(incoming, []string{"owner-b"}) {
		t.Fatalf("shared owner removal incoming=%v, want [owner-b]", incoming)
	}
	unreferenced, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(unreferenced, "shared") {
		t.Fatalf("shared owner removal incorrectly marked object orphaned: %v", unreferenced)
	}
}

func TestApplyRefBatchLastOwnerRemovalOrphansObject(t *testing.T) {
	g, cleanup := newTestGraph(t, "test-gcgraph-batch-last-owner")
	defer cleanup()
	ctx := context.Background()

	if err := g.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := g.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{
		Subject: "owner",
		Object:  "target",
	}}); err != nil {
		t.Fatal(err)
	}

	incoming, err := g.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(incoming, []string{block_gc.NodeUnreferenced}) {
		t.Fatalf("last owner removal incoming=%v, want only unreferenced", incoming)
	}
	unreferenced, err := g.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unreferenced, "target") {
		t.Fatalf("last owner removal did not mark target orphaned: %v", unreferenced)
	}
}
