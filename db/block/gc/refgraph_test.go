package block_gc

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

func newTestRefGraph(t *testing.T) *RefGraph {
	// Open an in-memory reference graph and register its cleanup.
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })
	return rg
}

// snapshotRefGraphRecords captures the underlying Cayley value, refcount,
// primitive-log, and edge-index records for the refgraph prefix.
func snapshotRefGraphRecords(t *testing.T, store kvtx.Store) map[string][]byte {
	// Open a read transaction for the reference graph records.
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Copy the reference graph records before the transaction closes.
	records := make(map[string][]byte)
	err = tx.ScanPrefix(ctx, []byte("gc/"), func(key, value []byte) error {
		records[string(key)] = slices.Clone(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func equalRefGraphRecords(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !bytes.Equal(value, other) {
			return false
		}
	}
	return true
}

func testBlockRef(t *testing.T, data string) *block.BlockRef {
	// Hash the test payload into a block reference.
	t.Helper()
	ht := hash.HashType_HashType_BLAKE3
	h, err := hash.Sum(ht, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}

func sortedStrings(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func TestAddAndGetOutgoingRefs(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Add two outgoing edges from node a.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}

	// Read the outgoing edges from node a.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify node a references exactly b and c.
	sorted := sortedStrings(refs)
	if len(sorted) != 2 || sorted[0] != "b" || sorted[1] != "c" {
		t.Fatalf("expected [b c], got %v", sorted)
	}
}

func TestGetIncomingRefs(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect two source nodes to node d.
	if err := rg.AddRef(ctx, "a", "d"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "b", "d"); err != nil {
		t.Fatal(err)
	}

	// Read the incoming sources of node d.
	sources, err := rg.GetIncomingRefs(ctx, "d")
	if err != nil {
		t.Fatal(err)
	}

	// Verify node d has exactly the two source nodes.
	sorted := sortedStrings(sources)
	if len(sorted) != 2 || sorted[0] != "a" || sorted[1] != "b" {
		t.Fatalf("expected [a b], got %v", sorted)
	}
}

func TestRemoveSingleRef(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect node a to two targets.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}

	// Remove the edge from a to b.
	if err := rg.RemoveRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	// Read the remaining outgoing edges of node a.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify only the edge to c remains.
	if len(refs) != 1 || refs[0] != "c" {
		t.Fatalf("expected [c], got %v", refs)
	}
}

func TestRemoveNonExistentRef(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Should not error when removing a non-existent edge.
	if err := rg.RemoveRef(ctx, "x", "y"); err != nil {
		t.Fatal(err)
	}
}

func TestFilterRefChangesPreservesBatchAndGraphEdges(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Prepare an added edge and an existing graph edge.
	adds := []RefEdge{{Subject: "batch", Object: "in-adds"}}
	if err := rg.AddRef(ctx, "graph", "in-graph"); err != nil {
		t.Fatal(err)
	}

	// Filter removals against the batch and the stored graph.
	removes := []RefEdge{
		{Subject: "batch", Object: "in-adds"},
		{Subject: "graph", Object: "in-graph"},
		{Subject: "absent", Object: "absent"},
	}
	got, err := rg.presentRemoves(ctx, adds, removes)
	if err != nil {
		t.Fatal(err)
	}

	// Verify only edges present in the batch or graph survive filtering.
	want := removes[:2]
	if !slices.Equal(got, want) {
		t.Fatalf("filtered removes = %#v, want %#v", got, want)
	}

	// An absent edge whose node names collide with the predicate IRI adds no
	// lookup entries; it must still be probed, not assumed to exist.
	got, err = rg.presentRemoves(ctx, nil, []RefEdge{
		{Subject: PredGCRef, Object: PredGCRef},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the absent predicate-named edge is excluded.
	if len(got) != 0 {
		t.Fatalf("filtered predicate-named removes = %#v, want none", got)
	}
}

func TestRemoveNodeRefs(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect node a to three targets.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "d"); err != nil {
		t.Fatal(err)
	}

	// Remove all outgoing edges from node a.
	targets, err := rg.RemoveNodeRefs(ctx, "a", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify removal returns all three targets.
	sorted := sortedStrings(targets)
	if len(sorted) != 3 || sorted[0] != "b" || sorted[1] != "c" || sorted[2] != "d" {
		t.Fatalf("expected [b c d], got %v", sorted)
	}

	// Verify all outgoing edges are gone.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify node a has no remaining outgoing edges.
	if len(refs) != 0 {
		t.Fatalf("expected no outgoing refs, got %v", refs)
	}
}

func TestHasIncomingRefs(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect node a to node b.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	// Read whether node b has incoming references.
	has, err := rg.HasIncomingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the target has an incoming reference.
	if !has {
		t.Fatal("expected b to have incoming refs")
	}

	// Read whether the source node has incoming references.
	has, err = rg.HasIncomingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the source has no incoming reference.
	if has {
		t.Fatal("expected a to have no incoming refs")
	}
}

func TestHasIncomingRefsExcludesUnreferenced(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Only edge is from "unreferenced" -- should not count.
	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan"); err != nil {
		t.Fatal(err)
	}

	// Read whether the orphan has a real incoming reference.
	has, err := rg.HasIncomingRefs(ctx, "orphan")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the staging edge does not keep the orphan reachable.
	if has {
		t.Fatal("expected orphan to have no real incoming refs")
	}

	// Add a real ref, now it should count.
	if err := rg.AddRef(ctx, "root", "orphan"); err != nil {
		t.Fatal(err)
	}

	// Read incoming references after adding the root edge.
	has, err = rg.HasIncomingRefs(ctx, "orphan")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the root edge keeps the orphan reachable.
	if !has {
		t.Fatal("expected orphan to have incoming refs after adding root edge")
	}
}

func TestHasIncomingRefsExcluding(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect the object and staging nodes to the same block.
	if err := rg.AddRef(ctx, "object:foo", "block:bar"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, NodeUnreferenced, "block:bar"); err != nil {
		t.Fatal(err)
	}

	// Read incoming references while excluding the object edge.
	has, err := rg.HasIncomingRefsExcluding(ctx, "block:bar", "object:foo")
	if err != nil {
		t.Fatal(err)
	}

	// Verify excluded and staging edges do not retain the block.
	if has {
		t.Fatal("expected only excluded and unreferenced edges to be ignored")
	}

	// Connect a second object to the block.
	if err := rg.AddRef(ctx, "object:other", "block:bar"); err != nil {
		t.Fatal(err)
	}

	// Read incoming references while retaining the second object edge.
	has, err = rg.HasIncomingRefsExcluding(ctx, "block:bar", "object:foo")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the second object keeps the block reachable.
	if !has {
		t.Fatal("expected remaining non-excluded incoming ref to be detected")
	}
}

func TestHasIncomingRefsExcludingMissingNameDoesNotSuppressIncomingRef(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect a real source to the target.
	if err := rg.AddRef(ctx, "real-source", "target"); err != nil {
		t.Fatal(err)
	}

	// Read incoming references while excluding a missing source.
	has, err := rg.HasIncomingRefsExcluding(ctx, "target", "missing-source")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the real source still keeps the target reachable.
	if !has {
		t.Fatal("expected real incoming ref to remain visible when excluded source is missing")
	}
}

func TestGetUnreferencedNodes(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Stage two orphan nodes in the reference graph.
	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan1"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan2"); err != nil {
		t.Fatal(err)
	}

	// Read the staged orphan nodes.
	nodes, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify both orphan nodes are listed.
	sorted := sortedStrings(nodes)
	if len(sorted) != 2 || sorted[0] != "orphan1" || sorted[1] != "orphan2" {
		t.Fatalf("expected [orphan1 orphan2], got %v", sorted)
	}
}

func TestDiamondDAG(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Build diamond: A -> B, A -> C, B -> D, C -> D
	if err := rg.AddRef(ctx, "A", "B"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "A", "C"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "B", "D"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "C", "D"); err != nil {
		t.Fatal(err)
	}

	// Remove A's outgoing refs (simulating A being collected).
	targets, err := rg.RemoveNodeRefs(ctx, "A", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify collecting A returns both child nodes.
	sorted := sortedStrings(targets)
	if len(sorted) != 2 || sorted[0] != "B" || sorted[1] != "C" {
		t.Fatalf("expected [B C], got %v", sorted)
	}

	// B lost its only incoming ref from A.
	has, err := rg.HasIncomingRefs(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}

	// Verify B has lost its incoming reference.
	if has {
		t.Fatal("expected B to have no incoming refs")
	}

	// C lost its only incoming ref from A.
	has, err = rg.HasIncomingRefs(ctx, "C")
	if err != nil {
		t.Fatal(err)
	}

	// Verify C has lost its incoming reference.
	if has {
		t.Fatal("expected C to have no incoming refs")
	}

	// D still has incoming refs from B and C (edges not yet removed).
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}

	// Verify D remains reachable through both children.
	if !has {
		t.Fatal("expected D to still have incoming refs from B and C")
	}

	// Cascade: remove B's outgoing refs.
	_, err = rg.RemoveNodeRefs(ctx, "B", false)
	if err != nil {
		t.Fatal(err)
	}

	// D still has incoming ref from C.
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}

	// Verify C still keeps D reachable.
	if !has {
		t.Fatal("expected D to still have incoming ref from C")
	}

	// Cascade: remove C's outgoing refs.
	_, err = rg.RemoveNodeRefs(ctx, "C", false)
	if err != nil {
		t.Fatal(err)
	}

	// Now D has no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}

	// Verify D becomes unreachable after both children are collected.
	if has {
		t.Fatal("expected D to have no incoming refs after full cascade")
	}
}

func TestMultipleRefsFromSameSource(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Adding the same edge twice should be idempotent.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	// Read outgoing edges after adding the duplicate reference.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the duplicate reference produces only one edge.
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b], got %v", refs)
	}
}

func TestPermanentRoots(t *testing.T) {
	if !IsPermanentRoot(NodeGCRoot) {
		t.Fatal("gcroot should be permanent")
	}
	if !IsPermanentRoot(NodeUnreferenced) {
		t.Fatal("unreferenced should be permanent")
	}
	if IsPermanentRoot("block:abc") {
		t.Fatal("block:abc should not be permanent")
	}
}

func TestAddBlockRef(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Create distinct source and target block references.
	src := testBlockRef(t, "source-block")
	tgt := testBlockRef(t, "target-block")

	// Connect the source block to the target block.
	if err := rg.AddBlockRef(ctx, src, tgt); err != nil {
		t.Fatal(err)
	}

	// Read the source block edges by IRI.
	refs, err := rg.GetOutgoingRefs(ctx, BlockIRI(src))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the graph stores the target block IRI.
	if len(refs) != 1 || refs[0] != BlockIRI(tgt) {
		t.Fatalf("expected [%s], got %v", BlockIRI(tgt), refs)
	}
}

func TestAddBlockRefNil(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Create the block reference used beside nil endpoints.
	src := testBlockRef(t, "source-block")

	// Nil target should be a no-op.
	if err := rg.AddBlockRef(ctx, src, nil); err != nil {
		t.Fatal(err)
	}

	// Nil source should be a no-op.
	if err := rg.AddBlockRef(ctx, nil, src); err != nil {
		t.Fatal(err)
	}
}

func TestAddObjectRoot(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Create and register the object root block.
	ref := testBlockRef(t, "obj-block")
	if err := rg.AddObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}

	// Read outgoing edges from the object root.
	refs, err := rg.GetOutgoingRefs(ctx, ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the object root references its block.
	if len(refs) != 1 || refs[0] != BlockIRI(ref) {
		t.Fatalf("expected [%s], got %v", BlockIRI(ref), refs)
	}
}

func TestRemoveObjectRoot(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Register a block as the object root.
	ref := testBlockRef(t, "obj-block")
	if err := rg.AddObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}

	// Remove the block from the object root.
	if err := rg.RemoveObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}

	// Read outgoing edges after removing the object root.
	refs, err := rg.GetOutgoingRefs(ctx, ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the object root no longer references any block.
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %v", refs)
	}
}

func TestDeepCascade(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Build a deep chain: gcroot -> E1 -> E2 -> E3 -> E4 -> block
	blk := BlockIRI(testBlockRef(t, "leaf-block"))

	// Connect the permanent root through the entity chain to the leaf block.
	if err := rg.AddRef(ctx, NodeGCRoot, "E1"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E1", "E2"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E2", "E3"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E3", "E4"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E4", blk); err != nil {
		t.Fatal(err)
	}

	// Remove E2 edge from E1 (simulate mid-chain removal).
	if err := rg.RemoveRef(ctx, "E1", "E2"); err != nil {
		t.Fatal(err)
	}

	// E2 should now have no incoming refs.
	has, err := rg.HasIncomingRefs(ctx, "E2")
	if err != nil {
		t.Fatal(err)
	}

	// Verify E2 has lost its incoming reference.
	if has {
		t.Fatal("expected E2 to have no incoming refs")
	}

	// Cascade: remove E2's outgoing.
	targets, err := rg.RemoveNodeRefs(ctx, "E2", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify collecting E2 returns E3.
	if len(targets) != 1 || targets[0] != "E3" {
		t.Fatalf("expected [E3], got %v", targets)
	}

	// E3 should have no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, "E3")
	if err != nil {
		t.Fatal(err)
	}

	// Verify E3 has lost its incoming reference.
	if has {
		t.Fatal("expected E3 to have no incoming refs")
	}

	// Cascade: remove E3's outgoing.
	targets, err = rg.RemoveNodeRefs(ctx, "E3", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify collecting E3 returns E4.
	if len(targets) != 1 || targets[0] != "E4" {
		t.Fatalf("expected [E4], got %v", targets)
	}

	// Cascade: remove E4's outgoing.
	targets, err = rg.RemoveNodeRefs(ctx, "E4", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify collecting E4 returns the leaf block.
	if len(targets) != 1 || targets[0] != blk {
		t.Fatalf("expected [%s], got %v", blk, targets)
	}

	// Block should have no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, blk)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the leaf block becomes unreachable.
	if has {
		t.Fatal("expected block to have no incoming refs after cascade")
	}

	// gcroot and E1 should still be connected.
	has, err = rg.HasIncomingRefs(ctx, "E1")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the permanent root still retains E1.
	if !has {
		t.Fatal("expected E1 to still have incoming ref from gcroot")
	}
}

func TestMixedNodeTypes(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Create an object IRI and two block IRIs.
	obj := ObjectIRI("world-obj")
	blk1 := BlockIRI(testBlockRef(t, "block1"))
	blk2 := BlockIRI(testBlockRef(t, "block2"))

	// Object references both blocks and an arbitrary entity node.
	if err := rg.AddRef(ctx, obj, blk1); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, obj, blk2); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, obj, "entity:foo"); err != nil {
		t.Fatal(err)
	}

	// Read the mixed outgoing references of the object.
	refs, err := rg.GetOutgoingRefs(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the object has three outgoing references.
	sorted := sortedStrings(refs)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 outgoing refs, got %v", sorted)
	}

	// All three should have incoming refs from obj.
	for _, node := range []string{blk1, blk2, "entity:foo"} {
		// Read incoming references for each mixed target.
		has, err := rg.HasIncomingRefs(ctx, node)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the object retains each mixed target.
		if !has {
			t.Fatalf("expected %s to have incoming refs", node)
		}
	}
}

func TestApplyRefBatchAddsBeforeRemoves(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Apply a batch that adds and removes the same edge.
	edge := RefEdge{Subject: "owner", Object: "target"}
	if err := rg.ApplyRefBatch(ctx, []RefEdge{edge}, []RefEdge{edge}); err != nil {
		t.Fatal(err)
	}

	// Read outgoing edges after the colliding batch.
	refs, err := rg.GetOutgoingRefs(ctx, edge.Subject)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the removal takes effect after the addition.
	if slices.Contains(refs, edge.Object) {
		t.Fatal("edge survived; removals must follow additions")
	}
}

func TestApplyRefBatchMissingRemovalPreservesGraphRecords(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	// Create an owned target and a staged orphan.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, NodeUnreferenced, "staged"); err != nil {
		t.Fatal(err)
	}

	// Capture graph records and outgoing edges before missing removals.
	beforeRecords := snapshotRefGraphRecords(t, store)
	beforeOwner, err := rg.GetOutgoingRefs(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}

	// Capture incoming edges before missing removals.
	beforeTarget, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}

	// Capture staged nodes before missing removals.
	beforeUnreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Apply removals whose source edges do not exist.
	removes := []RefEdge{
		{Subject: "missing-owner", Object: "target"},
		{Subject: "missing-owner", Object: "never-seen"},
	}
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		t.Fatal(err)
	}

	// Verify missing removals preserve the stored graph records.
	afterRecords := snapshotRefGraphRecords(t, store)
	if !equalRefGraphRecords(beforeRecords, afterRecords) {
		t.Fatal("missing removals changed underlying Cayley records")
	}

	// Read outgoing edges after missing removals.
	afterOwner, err := rg.GetOutgoingRefs(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}

	// Read incoming edges after missing removals.
	afterTarget, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}

	// Read staged nodes after missing removals.
	afterUnreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify all graph views remain unchanged.
	if !slices.Equal(sortedStrings(beforeOwner), sortedStrings(afterOwner)) ||
		!slices.Equal(sortedStrings(beforeTarget), sortedStrings(afterTarget)) ||
		!slices.Equal(sortedStrings(beforeUnreferenced), sortedStrings(afterUnreferenced)) {
		t.Fatalf(
			"missing removals changed graph views: before owner=%v target=%v unreferenced=%v, after owner=%v target=%v unreferenced=%v",
			beforeOwner,
			beforeTarget,
			beforeUnreferenced,
			afterOwner,
			afterTarget,
			afterUnreferenced,
		)
	}
}

func TestPrepareRefBatchFiltersBackendRemovals(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Create the existing edge used among absent removals.
	existing := RefEdge{Subject: "owner", Object: "target"}
	if err := rg.AddRef(ctx, existing.Subject, existing.Object); err != nil {
		t.Fatal(err)
	}

	// Prepare backend removals against the stored graph.
	removes := []RefEdge{
		existing,
		{Subject: "missing-owner", Object: "target"},
		{Subject: "missing-owner", Object: "never-seen"},
	}
	_, backendRemoves, err := rg.prepareRefBatch(ctx, nil, removes, false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the backend receives only the existing edge.
	if got, want := len(backendRemoves), 1; got != want {
		t.Fatalf("backend removal count = %d, want %d", got, want)
	}
	if !slices.Equal(backendRemoves, []RefEdge{existing}) {
		t.Fatalf("backend removals = %#v, want only %#v", backendRemoves, existing)
	}
}

func TestApplyRefBatchExistingRemovalUpdatesIndexesAndOrphan(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	// Create an owned target and capture its stored records.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	beforeRecords := snapshotRefGraphRecords(t, store)

	// Remove the existing ownership edge in a batch.
	if err := rg.ApplyRefBatch(ctx, nil, []RefEdge{{
		Subject: "owner",
		Object:  "target",
	}}); err != nil {
		t.Fatal(err)
	}

	// Verify removing the edge changes the graph records.
	afterRecords := snapshotRefGraphRecords(t, store)
	if equalRefGraphRecords(beforeRecords, afterRecords) {
		t.Fatal("existing removal left underlying Cayley records unchanged")
	}

	// Verify the forward index no longer contains the ownership edge.
	if refs, err := rg.GetOutgoingRefs(ctx, "owner"); err != nil {
		t.Fatal(err)
	} else if slices.Contains(refs, "target") {
		t.Fatal("existing edge remains in forward index")
	}

	// Verify the reverse index contains only the orphan marker.
	if refs, err := rg.GetIncomingRefs(ctx, "target"); err != nil {
		t.Fatal(err)
	} else if !slices.Equal(refs, []string{NodeUnreferenced}) {
		t.Fatalf("existing edge reverse index = %v, want only unreferenced", refs)
	}

	// Read orphan nodes after removing the last owner.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the target is marked orphaned.
	if !slices.Contains(unreferenced, "target") {
		t.Fatalf("last owner removal did not mark target orphaned: %v", unreferenced)
	}
}

func TestApplyRefBatchCollidingAddAndRemoveOrphans(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Add and remove the same ownership edge in one batch.
	edge := RefEdge{Subject: "owner", Object: "target"}
	if err := rg.ApplyRefBatch(ctx, []RefEdge{edge}, []RefEdge{edge}); err != nil {
		t.Fatal(err)
	}

	// Verify the colliding ownership edge is removed.
	if refs, err := rg.GetOutgoingRefs(ctx, edge.Subject); err != nil {
		t.Fatal(err)
	} else if slices.Contains(refs, edge.Object) {
		t.Fatal("colliding edge remains after add-before-remove")
	}

	// Read orphan nodes after the colliding batch.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the colliding batch marks its target orphaned.
	if !slices.Contains(unreferenced, edge.Object) {
		t.Fatalf("colliding edge did not produce orphan mark: %v", unreferenced)
	}
}

func TestApplyRefBatchSharedOwnerRemovalKeepsObjectReachable(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect two owners to the shared target.
	for _, owner := range []string{"owner-a", "owner-b"} {
		if err := rg.AddRef(ctx, owner, "shared"); err != nil {
			t.Fatal(err)
		}
	}

	// Remove only the first ownership edge.
	if err := rg.ApplyRefBatch(ctx, nil, []RefEdge{{
		Subject: "owner-a",
		Object:  "shared",
	}}); err != nil {
		t.Fatal(err)
	}

	// Read remaining incoming edges of the shared target.
	incoming, err := rg.GetIncomingRefs(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the second owner still retains the shared target.
	if !slices.Equal(sortedStrings(incoming), []string{"owner-b"}) {
		t.Fatalf("shared incoming owners = %v, want [owner-b]", incoming)
	}

	// Read orphan nodes after removing one shared owner.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the shared target remains outside the orphan set.
	if slices.Contains(unreferenced, "shared") {
		t.Fatalf("shared object was incorrectly orphaned: %v", unreferenced)
	}
}

func TestApplyRefBatchLastOwnerRemovalOrphansObject(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Connect the sole owner to its target.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}

	// Remove the sole ownership edge in a batch.
	if err := rg.ApplyRefBatch(ctx, nil, []RefEdge{{
		Subject: "owner",
		Object:  "target",
	}}); err != nil {
		t.Fatal(err)
	}

	// Read incoming edges after removing the sole owner.
	incoming, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}

	// Verify only the orphan marker remains in the reverse index.
	if !slices.Equal(incoming, []string{NodeUnreferenced}) {
		t.Fatalf("last owner reverse index = %v, want only unreferenced", incoming)
	}

	// Read orphan nodes after removing the sole owner.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the target belongs to the orphan set.
	if !slices.Contains(unreferenced, "target") {
		t.Fatalf("last owner removal did not orphan target: %v", unreferenced)
	}
}

func TestApplyRefBatchLargeBatchKeepsAddThenRemoveSemantics(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Prepare additions spanning several batches and a colliding removal.
	edgeCount := refGraphApplyBatchLimit*3 + 17
	adds := make([]RefEdge, 0, edgeCount+1)
	for i := range edgeCount {
		adds = append(adds, RefEdge{
			Subject: "root",
			Object:  "node-" + strconv.Itoa(i),
		})
	}
	adds = append(adds, RefEdge{Subject: "root", Object: "removed"})
	removes := []RefEdge{{Subject: "root", Object: "removed"}}

	// Apply the large batch of additions and removals.
	if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
		t.Fatal(err)
	}

	// Read outgoing references from the batch root.
	refs, err := rg.GetOutgoingRefs(ctx, "root")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the large batch retains every surviving target.
	if len(refs) != edgeCount {
		t.Fatalf("outgoing ref count = %d, want %d", len(refs), edgeCount)
	}
	if slices.Contains(refs, "removed") {
		t.Fatal("removed edge should be absent after add-before-remove batch")
	}

	// Verify targets from different batch positions are present.
	for _, node := range []string{"node-0", "node-512", "node-1536"} {
		if !slices.Contains(refs, node) {
			t.Fatalf("missing edge to %s", node)
		}
	}
}

func TestApplyRefBatchPreparesEachSliceAfterPriorCommit(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	baseStore := store_kvtx_inmem.NewStore()
	store := &refGraphTrackingStore{Store: baseStore}
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	// Create an existing edge and reset transaction counters.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	store.commits.Store(0)
	store.readsBeforeCommit.Store(0)

	// Prepare additions that span multiple committed slices.
	adds := make([]RefEdge, 4096)
	for i := range adds {
		adds[i] = RefEdge{
			Subject: "batch-owner",
			Object:  "batch-target-" + strconv.Itoa(i),
		}
	}

	// Apply additions before removing the existing edge.
	if err := rg.ApplyRefBatch(ctx, adds, []RefEdge{{Subject: "owner", Object: "target"}}); err != nil {
		t.Fatal(err)
	}

	// Verify at least one add slice has committed.
	if store.commits.Load() == 0 {
		t.Fatal("expected a committed add slice before the removal slice")
	}

	// The first add slice probes node IDs and exact edges in two snapshots.
	// Removal preparation must still wait until that slice commits.
	if got := store.readsBeforeCommit.Load(); got > 2 {
		t.Fatalf("read transactions before first slice commit = %d, want at most 2", got)
	}

	// Read outgoing edges created by the committed slices.
	refs, err := rg.GetOutgoingRefs(ctx, "batch-owner")
	if err != nil {
		t.Fatal(err)
	}

	// Verify every batch addition is visible.
	if len(refs) != len(adds) {
		t.Fatalf("batch-owner refs = %d, want %d", len(refs), len(adds))
	}

	// Verify the final removal leaves its target unreachable.
	if has, err := rg.HasIncomingRefs(ctx, "target"); err != nil {
		t.Fatal(err)
	} else if has {
		t.Fatal("target should have no real incoming refs after the removal slice")
	}
}

func TestApplyRefBatchOrphanMarkingAcrossSliceBoundary(t *testing.T) {
	// Prepare absent removals followed by both shared ownership edges.
	ctx := context.Background()
	removes := make([]RefEdge, 0, 4097)
	for i := range 4095 {
		removes = append(removes, RefEdge{
			Subject: "absent-source-" + strconv.Itoa(i),
			Object:  "absent-target-" + strconv.Itoa(i),
		})
	}
	removes = append(removes,
		RefEdge{Subject: "owner-a", Object: "shared"},
		RefEdge{Subject: "owner-b", Object: "shared"},
	)

	// Compare orphan marking across incremental and single-batch removals.
	for _, tc := range []struct {
		name string
		make func(*testing.T) *RefGraph
	}{
		{name: "incremental", make: func(t *testing.T) *RefGraph {
			// Create a graph with both owners retaining the shared target.
			rg := newTestRefGraph(t)
			if err := rg.AddRef(ctx, "owner-a", "shared"); err != nil {
				t.Fatal(err)
			}
			if err := rg.AddRef(ctx, "owner-b", "shared"); err != nil {
				t.Fatal(err)
			}
			return rg
		}},
		{name: "single-batch", make: func(t *testing.T) *RefGraph {
			// Create a graph with both owners retaining the shared target.
			rg := newTestRefGraph(t)
			if err := rg.AddRef(ctx, "owner-a", "shared"); err != nil {
				t.Fatal(err)
			}
			if err := rg.AddRef(ctx, "owner-b", "shared"); err != nil {
				t.Fatal(err)
			}
			return rg
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Choose the removal batch for the graph under test.
			rg := tc.make(t)
			inputRemoves := removes
			if tc.name == "single-batch" {
				inputRemoves = removes[4095:]
			}

			// Remove both owners with the selected batch shape.
			if err := rg.ApplyRefBatch(ctx, nil, inputRemoves); err != nil {
				t.Fatal(err)
			}

			// Read incoming edges after both owners are removed.
			incoming, err := rg.GetIncomingRefs(ctx, "shared")
			if err != nil {
				t.Fatal(err)
			}

			// Read orphan nodes after both owners are removed.
			unreferenced, err := rg.GetUnreferencedNodes(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// Verify the shared target is marked orphaned.
			if !slices.Contains(unreferenced, "shared") {
				t.Fatalf("shared should be marked unreferenced, got %v (incoming=%v)", unreferenced, incoming)
			}

			// Verify neither former owner retains outgoing edges.
			for _, owner := range []string{"owner-a", "owner-b"} {
				// Read outgoing edges for the former owner.
				refs, err := rg.GetOutgoingRefs(ctx, owner)
				if err != nil {
					t.Fatal(err)
				}

				// Verify the former owner has no remaining target.
				if len(refs) != 0 {
					t.Fatalf("%s outgoing refs = %v, want none", owner, refs)
				}
			}
		})
	}
}

func TestApplyRefBatchInterruptionKeepsCommittedPrefix(t *testing.T) {
	// Open a reference graph whose store cancels after the first slice commits.
	ctx, cancel := context.WithCancel(context.Background())
	store := &refGraphCancelOnCommitStore{
		Store:    store_kvtx_inmem.NewStore(),
		cancel:   cancel,
		cancelAt: int64(refGraphApplySliceLimit / refGraphApplyBatchLimit),
	}
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })
	store.commits.Store(0)

	// Prepare additions spanning one full slice and a trailing edge.
	adds := make([]RefEdge, refGraphApplySliceLimit+1)
	for i := range adds {
		adds[i] = RefEdge{
			Subject: "interrupt-owner",
			Object:  "interrupt-target-" + strconv.Itoa(i),
		}
	}

	// Verify cancellation interrupts the batch after a committed prefix.
	err = rg.ApplyRefBatch(ctx, adds, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted ApplyRefBatch error = %v, want context.Canceled", err)
	}

	// Read the committed prefix through a fresh context.
	refs, err := rg.GetOutgoingRefs(context.Background(), "interrupt-owner")
	if err != nil {
		t.Fatal(err)
	}

	// Verify exactly one full slice survives cancellation.
	if len(refs) != refGraphApplySliceLimit {
		t.Fatalf("committed prefix refs = %d, want %d", len(refs), refGraphApplySliceLimit)
	}

	// Apply the remaining edge through a fresh context.
	if err := rg.ApplyRefBatch(context.Background(), adds[refGraphApplySliceLimit:], nil); err != nil {
		t.Fatal(err)
	}

	// Read outgoing edges after completing the interrupted batch.
	refs, err = rg.GetOutgoingRefs(context.Background(), "interrupt-owner")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the completed batch retains all additions.
	if len(refs) != len(adds) {
		t.Fatalf("rerun refs = %d, want %d", len(refs), len(adds))
	}
}

type refGraphTrackingStore struct {
	kvtx.Store
	commits           atomic.Int64
	readsBeforeCommit atomic.Int64
}

func (s *refGraphTrackingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if !write && s.commits.Load() == 0 {
		s.readsBeforeCommit.Add(1)
	}
	return &refGraphTrackingTx{Tx: tx, store: s, write: write}, nil
}

type refGraphTrackingTx struct {
	kvtx.Tx
	store *refGraphTrackingStore
	write bool
}

func (t *refGraphTrackingTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	if err == nil && t.write {
		t.store.commits.Add(1)
	}
	return err
}

type refGraphCancelOnCommitStore struct {
	kvtx.Store
	commits  atomic.Int64
	cancel   context.CancelFunc
	cancelAt int64
}

func (s *refGraphCancelOnCommitStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &refGraphCancelOnCommitTx{Tx: tx, store: s, write: write}, nil
}

type refGraphCancelOnCommitTx struct {
	kvtx.Tx
	store *refGraphCancelOnCommitStore
	write bool
}

func (t *refGraphCancelOnCommitTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	if err == nil && t.write && t.store.commits.Add(1) == t.store.cancelAt {
		t.store.cancel()
	}
	return err
}

func TestBlockIRIRoundTrip(t *testing.T) {
	// Encode a block reference as an IRI.
	ref := testBlockRef(t, "roundtrip-data")
	iri := BlockIRI(ref)
	if iri == "" {
		t.Fatal("expected non-empty IRI")
	}

	// Parse the encoded block IRI.
	parsed, ok := ParseBlockIRI(iri)
	if !ok {
		t.Fatal("expected parse to succeed")
	}

	// Verify the parsed reference matches the original block.
	if !ref.EqualsRef(parsed) {
		t.Fatalf("round-trip mismatch: %s vs %s", ref.MarshalString(), parsed.MarshalString())
	}
}

func TestParseBlockIRIInvalid(t *testing.T) {
	// Verify block parsing rejects a foreign IRI.
	_, ok := ParseBlockIRI("not-a-block-iri")
	if ok {
		t.Fatal("expected parse to fail for non-block IRI")
	}

	// Verify block parsing rejects an empty block suffix.
	_, ok = ParseBlockIRI("block:")
	if ok {
		t.Fatal("expected parse to fail for empty block IRI")
	}

	// Verify block parsing rejects an empty string.
	_, ok = ParseBlockIRI("")
	if ok {
		t.Fatal("expected parse to fail for empty string")
	}
}

func TestBlockIRINilRef(t *testing.T) {
	// Verify a nil block reference has no IRI.
	iri := BlockIRI(nil)
	if iri != "" {
		t.Fatalf("expected empty IRI for nil ref, got %s", iri)
	}

	// Verify an empty block reference has no IRI.
	iri = BlockIRI(&block.BlockRef{})
	if iri != "" {
		t.Fatalf("expected empty IRI for empty ref, got %s", iri)
	}
}

func TestObjectRootNilRef(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Verify adding a nil object root succeeds.
	if err := rg.AddObjectRoot(ctx, "obj", nil); err != nil {
		t.Fatal(err)
	}

	// Verify removing a nil object root succeeds.
	if err := rg.RemoveObjectRoot(ctx, "obj", nil); err != nil {
		t.Fatal(err)
	}
}

func TestBucketIRI(t *testing.T) {
	iri := BucketIRI("my-bucket-123")
	if iri != "bucket:my-bucket-123" {
		t.Fatalf("expected bucket:my-bucket-123, got %s", iri)
	}
}

func TestParseBucketIRI(t *testing.T) {
	id, ok := ParseBucketIRI("bucket:my-bucket-123")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if id != "my-bucket-123" {
		t.Fatalf("expected my-bucket-123, got %s", id)
	}
}

func TestParseBucketIRIInvalid(t *testing.T) {
	// Verify bucket parsing rejects a foreign IRI.
	_, ok := ParseBucketIRI("not-a-bucket")
	if ok {
		t.Fatal("expected parse to fail for non-bucket IRI")
	}

	// Verify bucket parsing rejects an empty bucket suffix.
	_, ok = ParseBucketIRI("bucket:")
	if ok {
		t.Fatal("expected parse to fail for empty bucket IRI")
	}

	// Verify bucket parsing rejects an empty string.
	_, ok = ParseBucketIRI("")
	if ok {
		t.Fatal("expected parse to fail for empty string")
	}
}

// TestApplyRefBatchIgnoresAbsentStagingRemoval covers a removal whose subject
// exists in the graph but whose exact edge does not. Removing the unreferenced
// staging edge of an object that is not currently staged must not read as a
// real removal: doing so tells the orphan pass the caller is un-staging the
// object, and the object silently loses the orphan mark its last owner
// departing should have produced.
func TestApplyRefBatchIgnoresAbsentStagingRemoval(t *testing.T) {
	// Open a reference graph for the test.
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Stage and un-stage an unrelated object so NodeUnreferenced is a live
	// node in the graph rather than an IRI the store has never seen.
	if err := rg.AddRef(ctx, NodeUnreferenced, "decoy"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}

	// Remove the ownership edge alongside an absent staging edge.
	if err := rg.ApplyRefBatch(ctx, nil, []RefEdge{
		{Subject: "owner", Object: "target"},
		{Subject: NodeUnreferenced, Object: "target"},
	}); err != nil {
		t.Fatal(err)
	}

	// Read orphan nodes after the absent staging removal.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the target retains its orphan mark.
	if !slices.Contains(unreferenced, "target") {
		t.Fatalf("target lost its owner but was not marked orphaned: %v", unreferenced)
	}
}

func TestRegisterEntityChainRetriesInvalidSnapshot(t *testing.T) {
	// Open a reference graph with one injected invalid snapshot.
	ctx := context.Background()
	store := &invalidSnapshotOnceStore{Store: store_kvtx_inmem.NewStore()}
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Register the existing provider edge and arm the snapshot failure.
	if err := rg.AddRef(ctx, NodeGCRoot, "provider:local"); err != nil {
		t.Fatal(err)
	}
	store.failNextRead.Store(true)

	// Register the entity chain through the invalid snapshot.
	if err := RegisterEntityChain(
		ctx,
		rg,
		NodeGCRoot,
		"provider:local",
		"bucket:account-settings",
	); err != nil {
		t.Fatal(err)
	}

	// Verify the store injected the snapshot failure.
	if !store.failed.Load() {
		t.Fatal("test store did not inject an invalid snapshot")
	}

	// Read the new provider-to-bucket edge.
	found, err := rg.hasRef(ctx, "provider:local", "bucket:account-settings")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the chain extends beyond its duplicate root edge.
	if !found {
		t.Fatal("entity chain stopped after its duplicate root edge")
	}
}

type invalidSnapshotOnceStore struct {
	kvtx.Store
	failNextRead atomic.Bool
	failed       atomic.Bool
}

func (s *invalidSnapshotOnceStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	if write || !s.failNextRead.Load() {
		return tx, nil
	}
	return &invalidSnapshotOnceTx{Tx: tx, store: s}, nil
}

type invalidSnapshotOnceTx struct {
	kvtx.Tx
	store *invalidSnapshotOnceStore
	reads int
}

func (t *invalidSnapshotOnceTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.reads++
	if t.reads == 2 && t.store.failNextRead.CompareAndSwap(true, false) {
		t.store.failed.Store(true)
		return nil, false, kvtx.ErrInvalidSnapshot
	}
	return t.Tx.Get(ctx, key)
}
