package block_gc

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	block_store_kvtx "github.com/aperturerobotics/hydra/block/store/kvtx"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

// gcTestEnv holds the test environment for GCStoreOps tests.
type gcTestEnv struct {
	ctx      context.Context
	kvStore  *store_kvtx_inmem.Store
	rawStore block.StoreOps
	gcStore  *GCStoreOps
	refGraph *RefGraph
}

// newGCTestEnv creates a test environment with GCStoreOps wrapper.
func newGCTestEnv(t *testing.T) *gcTestEnv {
	t.Helper()
	ctx := context.Background()
	kvStore := store_kvtx_inmem.NewStore()
	kvKey := store_kvkey.NewDefaultKVKey()
	rawStore := block_store_kvtx.NewKVTxBlock(kvKey, kvStore, 0, false)

	rg, err := NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { rg.Close() })

	gcStore := NewGCStoreOps(rawStore, rg)
	return &gcTestEnv{
		ctx:      ctx,
		kvStore:  kvStore,
		rawStore: rawStore,
		gcStore:  gcStore,
		refGraph: rg,
	}
}

// putBlock stores a mock block via GCStoreOps and returns its ref.
// Flushes pending gc operations so the ref graph is up to date.
func (e *gcTestEnv) putBlock(t *testing.T, msg string) *block.BlockRef {
	t.Helper()
	ex := block_mock.NewExample(msg)
	ref, _, err := block.PutBlock(e.ctx, e.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := e.gcStore.FlushPending(e.ctx); err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// flush writes buffered GCStoreOps operations to the ref graph.
func (e *gcTestEnv) flush(t *testing.T) {
	t.Helper()
	if err := e.gcStore.FlushPending(e.ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// blockExists checks if a block exists in the raw store.
func (e *gcTestEnv) blockExists(t *testing.T, ref *block.BlockRef) bool {
	t.Helper()
	exists, err := e.rawStore.GetBlockExists(e.ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	return exists
}

// TestGCStoreOps_PutBlockAddsUnrefEdge tests that PutBlock adds an
// unreferenced gc/ref edge.
func TestGCStoreOps_PutBlockAddsUnrefEdge(t *testing.T) {
	env := newGCTestEnv(t)

	ex := block_mock.NewExample("test-block")
	ref, existed, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}
	if existed {
		t.Fatal("block should not have existed")
	}
	env.flush(t)

	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node, got %d", len(nodes))
	}
	expected := BlockIRI(ref)
	if nodes[0] != expected {
		t.Fatalf("expected unreferenced node %s, got %s", expected, nodes[0])
	}
}

// TestGCStoreOps_RecordRefsRemovesUnrefEdge tests that recording refs
// removes the unreferenced edge from the target.
func TestGCStoreOps_RecordRefsRemovesUnrefEdge(t *testing.T) {
	env := newGCTestEnv(t)

	aRef := env.putBlock(t, "block-a")
	bRef := env.putBlock(t, "block-b")

	// Both should be unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 unreferenced nodes before recording, got %d", len(nodes))
	}

	// Record a->b ref: b's unreferenced edge should be removed.
	err = env.gcStore.RecordBlockRefs(env.ctx, aRef, []*block.BlockRef{bRef})
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	nodes, err = env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	// Only a should remain unreferenced.
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node after recording, got %d", len(nodes))
	}
	expected := BlockIRI(aRef)
	if nodes[0] != expected {
		t.Fatalf("expected unreferenced node %s, got %s", expected, nodes[0])
	}
}

// TestGCStoreOps_DuplicatePutNoNewUnrefEdge tests that putting a
// duplicate block does not add another unreferenced edge.
func TestGCStoreOps_DuplicatePutNoNewUnrefEdge(t *testing.T) {
	env := newGCTestEnv(t)

	ex := block_mock.NewExample("dup-block")
	_, _, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Put again (duplicate).
	_, existed, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !existed {
		t.Fatal("duplicate block should report existed=true")
	}
	env.flush(t)

	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node (no dup), got %d", len(nodes))
	}
}

// TestGCStoreOps_RmBlockCleansGraph tests that RmBlock cleans up graph
// edges and cascades orphan detection.
func TestGCStoreOps_RmBlockCleansGraph(t *testing.T) {
	env := newGCTestEnv(t)

	aRef := env.putBlock(t, "block-a")
	bRef := env.putBlock(t, "block-b")

	// Record a->b, removing b's unreferenced edge.
	err := env.gcStore.RecordBlockRefs(env.ctx, aRef, []*block.BlockRef{bRef})
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// RmBlock on a should clean its outgoing edges and cascade.
	err = env.gcStore.RmBlock(env.ctx, aRef)
	if err != nil {
		t.Fatal(err.Error())
	}

	// a should have no outgoing refs.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, BlockIRI(aRef))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing refs from removed block, got %d", len(outgoing))
	}

	// b should now be unreferenced (cascade from a's removal).
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	found := slices.Contains(nodes, BlockIRI(bRef))
	if !found {
		t.Fatal("b should be marked unreferenced after a is removed")
	}
}

// TestGCStoreOps_AddGCRef tests that AddGCRef adds an edge and removes
// the unreferenced edge from the object.
func TestGCStoreOps_AddGCRef(t *testing.T) {
	env := newGCTestEnv(t)

	ref := env.putBlock(t, "block")
	blockIRI := BlockIRI(ref)

	// Block should be unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node, got %d", len(nodes))
	}

	// Add a GC ref from some entity to the block.
	err = env.gcStore.AddGCRef(env.ctx, "entity:my-bucket", blockIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Block should no longer be unreferenced.
	nodes, err = env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced nodes after AddGCRef, got %d", len(nodes))
	}

	// The entity -> block edge should exist.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, "entity:my-bucket")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected outgoing ref to %s, got %v", blockIRI, outgoing)
	}
}

// TestGCStoreOps_RemoveGCRef tests that RemoveGCRef removes the edge
// and marks the object orphaned if it has no remaining refs.
func TestGCStoreOps_RemoveGCRef(t *testing.T) {
	env := newGCTestEnv(t)

	ref := env.putBlock(t, "block")
	blockIRI := BlockIRI(ref)

	// Add then remove a GC ref.
	err := env.gcStore.AddGCRef(env.ctx, "entity:my-bucket", blockIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = env.gcStore.RemoveGCRef(env.ctx, "entity:my-bucket", blockIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Block should be unreferenced again.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	found := slices.Contains(nodes, blockIRI)
	if !found {
		t.Fatal("block should be unreferenced after RemoveGCRef")
	}
}

// TestGCStoreOps_TransactionRecordsRefs tests that Transaction.Write
// automatically records block refs when using GCStoreOps.
func TestGCStoreOps_TransactionRecordsRefs(t *testing.T) {
	env := newGCTestEnv(t)

	// Put a child block first.
	child := block_mock.NewExample("child")
	childRef, _, err := block.PutBlock(env.ctx, env.gcStore, child)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a SubBlock pointing to child.
	sub := &block_mock.SubBlock{ExamplePtr: childRef}

	// Create a Root with the sub-block.
	root := &block_mock.Root{ExampleSubBlock: sub}

	// Use a Transaction to write the root.
	tx, cursor := block.NewTransaction(env.gcStore, nil, nil, nil)
	cursor.SetBlock(root, true)

	rootRef, _, err := tx.Write(env.ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if rootRef == nil {
		t.Fatal("expected non-nil root ref")
	}
	env.flush(t)

	// The transaction should have recorded block ref edges.
	rootIRI := BlockIRI(rootRef)
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, rootIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 1 {
		t.Fatalf("expected 1 outgoing ref from root, got %d", len(outgoing))
	}
	expectedChild := BlockIRI(childRef)
	if outgoing[0] != expectedChild {
		t.Fatalf("expected ref to child %s, got %s", expectedChild, outgoing[0])
	}

	// Child's unreferenced edge should have been removed.
	// Root should still be unreferenced (no GC ref to it yet).
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node (root only), got %d", len(nodes))
	}
	if nodes[0] != rootIRI {
		t.Fatalf("expected unreferenced node to be root %s, got %s", rootIRI, nodes[0])
	}
}

// TestGCStoreOps_CollectWithGCStore tests full GC lifecycle: put, record
// refs, remove root, and collect.
func TestGCStoreOps_CollectWithGCStore(t *testing.T) {
	env := newGCTestEnv(t)

	orphan := env.putBlock(t, "orphan")
	rooted := env.putBlock(t, "rooted")

	rootedIRI := BlockIRI(rooted)

	// Add a GC ref for the rooted block (also removes its unreferenced edge).
	err := env.gcStore.AddGCRef(env.ctx, "entity:bucket-1", rootedIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// orphan still has its unreferenced edge from PutBlock.
	gc := NewCollector(env.refGraph, env.rawStore, nil)
	stats, err := gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	// Orphan should be gone.
	if env.blockExists(t, orphan) {
		t.Fatal("orphan should have been swept")
	}

	// Rooted should survive.
	if !env.blockExists(t, rooted) {
		t.Fatal("rooted block should survive")
	}
}

// newGCTestEnvWithParent creates a test environment with parentIRI set.
func newGCTestEnvWithParent(t *testing.T, parentIRI string) *gcTestEnv {
	t.Helper()
	ctx := context.Background()
	kvStore := store_kvtx_inmem.NewStore()
	kvKey := store_kvkey.NewDefaultKVKey()
	rawStore := block_store_kvtx.NewKVTxBlock(kvKey, kvStore, 0, false)

	rg, err := NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { rg.Close() })

	gcStore := NewGCStoreOpsWithParent(rawStore, rg, parentIRI)
	return &gcTestEnv{
		ctx:      ctx,
		kvStore:  kvStore,
		rawStore: rawStore,
		gcStore:  gcStore,
		refGraph: rg,
	}
}

// TestGCStoreOps_ParentIRI_PutBlockAddsParentEdge tests that PutBlock
// adds a parentIRI -> block edge when parentIRI is set.
func TestGCStoreOps_ParentIRI_PutBlockAddsParentEdge(t *testing.T) {
	parent := BucketIRI("my-bucket")
	env := newGCTestEnvWithParent(t, parent)

	ref := env.putBlock(t, "parent-block")
	blockIRI := BlockIRI(ref)

	// Should have parent -> block edge, not unreferenced -> block.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected parent -> %s, got %v", blockIRI, outgoing)
	}

	// Should NOT be in unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced nodes, got %d", len(nodes))
	}
}

// TestGCStoreOps_ParentIRI_RmBlockRemovesParentEdge tests that
// RmBlock removes the parentIRI -> block edge when parentIRI is set.
func TestGCStoreOps_ParentIRI_RmBlockRemovesParentEdge(t *testing.T) {
	parent := BucketIRI("my-bucket")
	env := newGCTestEnvWithParent(t, parent)

	ref := env.putBlock(t, "rm-parent-block")
	blockIRI := BlockIRI(ref)

	// Verify parent -> block edge exists.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected parent -> %s before rm, got %v", blockIRI, outgoing)
	}

	// RmBlock should remove parent -> block edge.
	if err := env.gcStore.RmBlock(env.ctx, ref); err != nil {
		t.Fatal(err.Error())
	}

	outgoing, err = env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing from parent after rm, got %d", len(outgoing))
	}
}

// TestGCStoreOps_ParentIRI_FlushPending tests that FlushPending
// works correctly with parentIRI mode.
func TestGCStoreOps_ParentIRI_FlushPending(t *testing.T) {
	parent := BucketIRI("test-bucket")
	env := newGCTestEnvWithParent(t, parent)

	// Put two blocks without flushing.
	ex1 := block_mock.NewExample("flush-a")
	ref1, _, err := block.PutBlock(env.ctx, env.gcStore, ex1)
	if err != nil {
		t.Fatal(err.Error())
	}

	ex2 := block_mock.NewExample("flush-b")
	ref2, _, err := block.PutBlock(env.ctx, env.gcStore, ex2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Before flush, no edges should exist.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing before flush, got %d", len(outgoing))
	}

	// Flush.
	env.flush(t)

	// After flush, parent should point to both blocks.
	outgoing, err = env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 2 {
		t.Fatalf("expected 2 outgoing after flush, got %d", len(outgoing))
	}
	sorted := sortedStrings(outgoing)
	iri1 := BlockIRI(ref1)
	iri2 := BlockIRI(ref2)
	expected := sortedStrings([]string{iri1, iri2})
	if sorted[0] != expected[0] || sorted[1] != expected[1] {
		t.Fatalf("expected %v, got %v", expected, sorted)
	}
}
