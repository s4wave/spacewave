package block_gc

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
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
	// Create an in-memory block store for GC tests.
	t.Helper()
	ctx := context.Background()
	kvStore := store_kvtx_inmem.NewStore()
	kvKey := store_kvkey.NewDefaultKVKey()
	rawStore := block_store_kvtx.NewKVTxBlock(kvKey, kvStore, 0, false)

	// Open the reference graph and register its cleanup.
	rg, err := NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { rg.Close() })

	// Wrap the block store with GC reference tracking.
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
	// Store the mock block through GC reference tracking.
	t.Helper()
	ex := block_mock.NewExample(msg)
	ref, _, err := block.PutBlock(e.ctx, e.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Flush the block reference changes into the graph.
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

// recordRefs buffers the edges a writer's put of source records.
func (e *gcTestEnv) recordRefs(source *block.BlockRef, targets []*block.BlockRef) {
	e.gcStore.mu.Lock()
	e.gcStore.bufferRefEdgesLocked(source, targets, true)
	e.gcStore.mu.Unlock()
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
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write a new block through the GC store.
	ex := block_mock.NewExample("test-block")
	ref, existed, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the write creates a new block.
	if existed {
		t.Fatal("block should not have existed")
	}

	// Flush pending reference changes into the graph.
	env.flush(t)

	// Read unreferenced nodes after the new write.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new block is the only unreferenced node.
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node, got %d", len(nodes))
	}
	expected := BlockIRI(ref)
	if nodes[0] != expected {
		t.Fatalf("expected unreferenced node %s, got %s", expected, nodes[0])
	}
}

func TestGCStoreOps_NestedDeferFlushFlushesOnce(t *testing.T) {
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write a block while two nested scopes defer flushing.
	env.gcStore.BeginDeferFlush()
	env.gcStore.BeginDeferFlush()
	ex := block_mock.NewExample("nested-defer")
	ref, _, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// End the inner deferred-flush scope.
	if err := env.gcStore.EndDeferFlush(env.ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the inner scope leaves the unreferenced edge buffered.
	env.gcStore.mu.Lock()
	pending := len(env.gcStore.pendingUnref)
	env.gcStore.mu.Unlock()
	if pending != 1 {
		t.Fatalf("expected pending unref to remain after inner End, got %d", pending)
	}

	// End the outer deferred-flush scope.
	if err := env.gcStore.EndDeferFlush(env.ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the outer scope flushes the buffered edge.
	env.gcStore.mu.Lock()
	pending = len(env.gcStore.pendingUnref)
	env.gcStore.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected pending unref to flush at outer End, got %d", pending)
	}

	// Verify the block remains stored after both scopes end.
	if !env.blockExists(t, ref) {
		t.Fatal("expected block to persist")
	}
}

func TestGCStoreOps_FlushPendingNormalizesDuplicateEdges(t *testing.T) {
	// Create a GC store with a recording reference graph.
	ctx := context.Background()
	refGraph := &recordingRefGraph{}
	gcStore := NewGCStoreOps(block.NopStoreOps{}, refGraph)

	// Buffer duplicate reference and staging changes.
	gcStore.mu.Lock()
	gcStore.pendingUnref = append(gcStore.pendingUnref, "block:a", "block:a", "block:b")
	gcStore.pendingRefs = append(
		gcStore.pendingRefs,
		pendingRef{"block:a", "block:b"},
		pendingRef{"block:a", "block:b"},
		pendingRef{"block:c", "block:d"},
	)
	gcStore.pendingUnunref = append(gcStore.pendingUnunref, "block:b", "block:b")
	gcStore.mu.Unlock()

	// Flush duplicate changes to the reference graph.
	if err := gcStore.FlushPending(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify duplicate changes use one graph batch.
	if refGraph.applyCount != 1 {
		t.Fatalf("ApplyRefBatch called %d times, want 1", refGraph.applyCount)
	}

	// Verify the batch contains distinct additions and removals.
	wantAdds := []RefEdge{
		{Subject: NodeUnreferenced, Object: "block:a"},
		{Subject: NodeUnreferenced, Object: "block:b"},
		{Subject: "block:a", Object: "block:b"},
		{Subject: "block:c", Object: "block:d"},
	}
	wantRemoves := []RefEdge{
		{Subject: NodeUnreferenced, Object: "block:b"},
	}
	if !slices.Equal(refGraph.adds, wantAdds) {
		t.Fatalf("adds = %#v, want %#v", refGraph.adds, wantAdds)
	}
	if !slices.Equal(refGraph.removes, wantRemoves) {
		t.Fatalf("removes = %#v, want %#v", refGraph.removes, wantRemoves)
	}
}

func TestGCStoreOps_FlushPendingRebuffersApplyRemainder(t *testing.T) {
	// Open a reference graph for the partial-flush test.
	ctx := context.Background()
	rg, err := NewRefGraph(ctx, store_kvtx_inmem.NewStore(), []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	// Buffer three staging edges behind a graph that fails once.
	refGraph := &failOnceRefGraph{RefGraphOps: rg}
	gcStore := NewGCStoreOps(block.NopStoreOps{}, refGraph)
	gcStore.mu.Lock()
	gcStore.pendingUnref = []string{"block:a", "block:b", "block:c"}
	gcStore.mu.Unlock()

	// Verify the first flush reports its injected failure.
	err = gcStore.FlushPending(ctx)
	if err == nil || !errors.Is(err, errInjectedRefBatch) {
		t.Fatalf("first flush error = %v, want injected ref batch error", err)
	}

	// Flush the buffered remainder after the partial failure.
	if err := gcStore.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Read staging edges after completing the flush.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify all three blocks are staged exactly once.
	slices.Sort(unreferenced)
	want := []string{"block:a", "block:b", "block:c"}
	if !slices.Equal(unreferenced, want) {
		t.Fatalf("unreferenced nodes = %v, want %v", unreferenced, want)
	}

	// Verify each staging edge was applied exactly once.
	if !slices.Equal(refGraph.applied, wantRefEdges(
		RefEdge{Subject: NodeUnreferenced, Object: "block:a"},
		RefEdge{Subject: NodeUnreferenced, Object: "block:b"},
		RefEdge{Subject: NodeUnreferenced, Object: "block:c"},
	)) {
		t.Fatalf("applied edges = %v, want each edge once", refGraph.applied)
	}
}

var errInjectedRefBatch = errors.New("injected ref batch failure")

type failOnceRefGraph struct {
	RefGraphOps
	failed  bool
	applied []RefEdge
}

func (r *failOnceRefGraph) ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error {
	// Apply the complete reference batch after the injected failure.
	if r.failed {
		r.applied = append(r.applied, adds...)
		r.applied = append(r.applied, removes...)
		return r.RefGraphOps.ApplyRefBatch(ctx, adds, removes)
	}

	// Apply a committed prefix and report the remaining reference changes.
	r.failed = true
	prefix := 1
	r.applied = append(r.applied, adds[:prefix]...)
	if err := r.RefGraphOps.ApplyRefBatch(ctx, adds[:prefix], nil); err != nil {
		return err
	}
	return &refBatchError{
		err:     errInjectedRefBatch,
		adds:    cloneRefEdges(adds[prefix:]),
		removes: cloneRefEdges(removes),
	}
}

func wantRefEdges(edges ...RefEdge) []RefEdge {
	return edges
}

// TestGCStoreOps_RecordRefsRemovesUnrefEdge tests that recording refs
// removes the unreferenced edge from the target.
func TestGCStoreOps_RecordRefsRemovesUnrefEdge(t *testing.T) {
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write two initially unreferenced blocks.
	aRef := env.putBlock(t, "block-a")
	bRef := env.putBlock(t, "block-b")

	// Both should be unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify both blocks begin in the staging set.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 unreferenced nodes before recording, got %d", len(nodes))
	}

	// Record a->b ref: b's unreferenced edge should be removed.
	env.recordRefs(aRef, []*block.BlockRef{bRef})
	env.flush(t)

	// Read staging edges after recording the block reference.
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
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write the original block before testing a duplicate.
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

	// Verify the repeated write reports an existing block.
	if !existed {
		t.Fatal("duplicate block should report existed=true")
	}

	// Flush reference changes from both writes.
	env.flush(t)

	// Read staged nodes after the duplicate write.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the duplicate produces only one staged node.
	if len(nodes) != 1 {
		t.Fatalf("expected 1 unreferenced node (no dup), got %d", len(nodes))
	}
}

// TestGCStoreOps_AddGCRef tests that AddGCRef adds an edge and removes
// the unreferenced edge from the object.
func TestGCStoreOps_AddGCRef(t *testing.T) {
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write a block and derive its graph IRI.
	ref := env.putBlock(t, "block")
	blockIRI := BlockIRI(ref)

	// Block should be unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the new block begins unreferenced.
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

	// Verify the entity reference removes the staging edge.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced nodes after AddGCRef, got %d", len(nodes))
	}

	// The entity -> block edge should exist.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, "entity:my-bucket")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the graph retains the entity-to-block edge.
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected outgoing ref to %s, got %v", blockIRI, outgoing)
	}
}

// TestGCStoreOps_RemoveGCRef tests that RemoveGCRef removes the edge
// and marks the object orphaned if it has no remaining refs.
func TestGCStoreOps_RemoveGCRef(t *testing.T) {
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write the block whose ownership edge will be removed.
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

	// Verify removing the entity reference marks the block unreferenced.
	found := slices.Contains(nodes, blockIRI)
	if !found {
		t.Fatal("block should be unreferenced after RemoveGCRef")
	}
}

// TestGCStoreOps_TransactionRecordsRefs tests that Transaction.Write
// automatically records block refs when using GCStoreOps.
func TestGCStoreOps_TransactionRecordsRefs(t *testing.T) {
	// Create the GC block store and reference graph for the test.
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

	// Write the root transaction to the block store.
	rootRef, _, err := tx.Write(env.ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify writing the root returns a block reference.
	if rootRef == nil {
		t.Fatal("expected non-nil root ref")
	}

	// Flush the transaction reference changes.
	env.flush(t)

	// The transaction should have recorded block ref edges.
	rootIRI := BlockIRI(rootRef)
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, rootIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the root references exactly its child block.
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

	// Verify only the root remains in the staging set.
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
	// Create the GC block store and reference graph for the test.
	env := newGCTestEnv(t)

	// Write an orphan block and a block that will be retained.
	orphan := env.putBlock(t, "orphan")
	rooted := env.putBlock(t, "rooted")

	// Derive the rooted block IRI for its ownership edge.
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

	// Verify the collector sweeps exactly one orphan.
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
	// Create an in-memory block store for parent ownership tests.
	t.Helper()
	ctx := context.Background()
	kvStore := store_kvtx_inmem.NewStore()
	kvKey := store_kvkey.NewDefaultKVKey()
	rawStore := block_store_kvtx.NewKVTxBlock(kvKey, kvStore, 0, false)

	// Open the reference graph and register its cleanup.
	rg, err := NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { rg.Close() })

	// Wrap the block store with parent reference tracking.
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
	// Create a GC store with bucket ownership.
	parent := BucketIRI("my-bucket")
	env := newGCTestEnvWithParent(t, parent)

	// Write a block owned by the bucket.
	ref := env.putBlock(t, "parent-block")
	blockIRI := BlockIRI(ref)

	// Should have parent -> block edge, not unreferenced -> block.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the bucket references the stored block.
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected parent -> %s, got %v", blockIRI, outgoing)
	}

	// Should NOT be in unreferenced.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the bucket-owned block is absent from staging.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced nodes, got %d", len(nodes))
	}
}

// TestGCStoreOps_ParentIRI_RmBlockRemovesParentEdge tests that
// RmBlock removes the parentIRI -> block edge when parentIRI is set.
func TestGCStoreOps_ParentIRI_RmBlockRemovesParentEdge(t *testing.T) {
	// Create a GC store with bucket ownership.
	parent := BucketIRI("my-bucket")
	env := newGCTestEnvWithParent(t, parent)

	// Write the block whose parent edge will be removed.
	ref := env.putBlock(t, "rm-parent-block")
	blockIRI := BlockIRI(ref)

	// Verify parent -> block edge exists.
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the bucket initially references the block.
	if len(outgoing) != 1 || outgoing[0] != blockIRI {
		t.Fatalf("expected parent -> %s before rm, got %v", blockIRI, outgoing)
	}

	// RmBlock should remove parent -> block edge.
	if err := env.gcStore.RmBlock(env.ctx, ref); err != nil {
		t.Fatal(err.Error())
	}

	// Flush reference changes after removing the block.
	env.flush(t)

	// Read parent edges after removing the block.
	outgoing, err = env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the bucket has no remaining outgoing reference.
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing from parent after rm, got %d", len(outgoing))
	}
}

// TestGCStoreOps_ParentIRI_FlushPending tests that FlushPending
// works correctly with parentIRI mode.
func TestGCStoreOps_ParentIRI_FlushPending(t *testing.T) {
	// Create a GC store with bucket ownership.
	parent := BucketIRI("test-bucket")
	env := newGCTestEnvWithParent(t, parent)

	// Put two blocks without flushing.
	ex1 := block_mock.NewExample("flush-a")
	ref1, _, err := block.PutBlock(env.ctx, env.gcStore, ex1)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the second block without flushing its reference changes.
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

	// Verify parent edges remain buffered before flushing.
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

	// Verify flushing produces both parent edges.
	if len(outgoing) != 2 {
		t.Fatalf("expected 2 outgoing after flush, got %d", len(outgoing))
	}

	// Verify the parent edges target the two written blocks.
	sorted := sortedStrings(outgoing)
	iri1 := BlockIRI(ref1)
	iri2 := BlockIRI(ref2)
	expected := sortedStrings([]string{iri1, iri2})
	if sorted[0] != expected[0] || sorted[1] != expected[1] {
		t.Fatalf("expected %v, got %v", expected, sorted)
	}
}

// TestGCStoreOps_ParentIRI_DedupClearsStaleUnref tests that taking ownership
// of a block staged under unreferenced removes the stale staging edge.
func TestGCStoreOps_ParentIRI_DedupClearsStaleUnref(t *testing.T) {
	for _, tc := range []struct {
		name  string
		batch bool
	}{
		{name: "PutBlock"},
		{name: "PutBlockBatch", batch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Create a GC store for the selected write path.
			env := newGCTestEnv(t)

			// Stage the block through the selected write path.
			var ref *block.BlockRef
			if tc.batch {
				entry := buildBatchEntry(t, "parent-dedup-batch")
				if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{entry}); err != nil {
					t.Fatal(err.Error())
				}
				ref = entry.Ref
			} else {
				ex := block_mock.NewExample("parent-dedup-single")
				var err error
				ref, _, err = block.PutBlock(env.ctx, env.gcStore, ex)
				if err != nil {
					t.Fatal(err.Error())
				}
			}

			// Flush the initial staging edge.
			env.flush(t)

			// Read staged nodes before taking parent ownership.
			nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify the block initially belongs to the staging set.
			if !slices.Contains(nodes, BlockIRI(ref)) {
				t.Fatal("block should be staged under unreferenced")
			}

			// Write the existing block through a parent-owned store.
			parent := BucketIRI("parent-dedup")
			parentStore := NewGCStoreOpsWithParent(env.rawStore, env.refGraph, parent)
			if tc.batch {
				entry := buildBatchEntry(t, "parent-dedup-batch")
				if _, err := parentStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{entry}); err != nil {
					t.Fatal(err.Error())
				}
			} else {
				ex := block_mock.NewExample("parent-dedup-single")
				if _, _, err := block.PutBlock(env.ctx, parentStore, ex); err != nil {
					t.Fatal(err.Error())
				}
			}

			// Flush the new parent ownership edge.
			if err := parentStore.FlushPending(env.ctx); err != nil {
				t.Fatal(err.Error())
			}

			// Read staged nodes after taking parent ownership.
			nodes, err = env.refGraph.GetUnreferencedNodes(env.ctx)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify parent ownership removes the staging edge.
			if slices.Contains(nodes, BlockIRI(ref)) {
				t.Fatal("parent-owned block should not remain unreferenced")
			}

			// Collect orphan blocks after taking parent ownership.
			if _, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx); err != nil {
				t.Fatal(err.Error())
			}

			// Read the parent-owned block after collection.
			_, exists, err := parentStore.GetBlock(env.ctx, ref)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Verify the parent-owned block survives collection.
			if !exists {
				t.Fatal("parent-owned block should survive collection")
			}
		})
	}
}

// TestGCStoreOps_ParentIRI_FlushPendingRemovesConcurrentUnref tests that a
// staging edge created after reconciliation begins is removed by the same
// ref-graph batch as the parent edge.
func TestGCStoreOps_ParentIRI_FlushPendingRemovesConcurrentUnref(t *testing.T) {
	// Write and stage the block before concurrent ownership changes.
	env := newGCTestEnv(t)
	ex := block_mock.NewExample("concurrent-unref")
	ref, _, err := block.PutBlock(env.ctx, env.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Remove the initial staging edge from the graph.
	blockIRI := BlockIRI(ref)
	if err := env.refGraph.RemoveRef(env.ctx, NodeUnreferenced, blockIRI); err != nil {
		t.Fatal(err.Error())
	}

	// Create a parent store that injects staging during its graph batch.
	raceGraph := &injectUnrefRefGraph{
		RefGraphOps: env.refGraph,
		object:      blockIRI,
	}
	parentStore := NewGCStoreOpsWithParent(
		env.rawStore,
		raceGraph,
		BucketIRI("concurrent-unref"),
	)

	// Write the existing block through the parent store.
	_, existed, err := block.PutBlock(env.ctx, parentStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the parent write deduplicates the existing block.
	if !existed {
		t.Fatal("expected parent write to deduplicate the block")
	}

	// Flush parent ownership while staging is injected.
	if err := parentStore.FlushPending(env.ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the graph injected the concurrent staging edge.
	if !raceGraph.injected {
		t.Fatal("expected concurrent staging edge injection")
	}

	// Collect orphan blocks after the parent ownership batch.
	if _, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Verify the parent-owned block survives collection.
	if !env.blockExists(t, ref) {
		t.Fatal("parent-owned block should survive collection")
	}
}

// TestGCStoreOps_RemoveGCRefDoesNotReviveStagingAfterParentBatch verifies
// that orphan marking waits inside the ownership transition.
func TestGCStoreOps_RemoveGCRefDoesNotReviveStagingAfterParentBatch(t *testing.T) {
	// Give the block an old owner the remover will release.
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "orphan-transition")
	object := BlockIRI(ref)
	if err := env.gcStore.AddGCRef(env.ctx, "owner:old", object); err != nil {
		t.Fatal(err.Error())
	}

	// Start the removal and hold it inside the ownership transition.
	raceGraph := &orphanRaceRefGraph{
		RefGraphOps:    env.refGraph,
		removerStarted: make(chan struct{}),
		parentDone:     make(chan struct{}),
	}
	remover := NewGCStoreOps(env.rawStore, raceGraph)
	removerErr := make(chan error, 1)
	go func() {
		removerErr <- remover.RemoveGCRef(env.ctx, "owner:old", object)
	}()
	<-raceGraph.removerStarted

	// Flush a parent edge while the removal waits.
	parent := NewGCStoreOps(env.rawStore, raceGraph)
	parent.mu.Lock()
	parent.bufferRefEdgesLocked(ref, []*block.BlockRef{ref}, true)
	parent.mu.Unlock()
	if err := parent.FlushPending(env.ctx); err != nil {
		t.Fatal(err.Error())
	}
	if err := <-removerErr; err != nil {
		t.Fatal(err.Error())
	}

	// The parent keeps the block and its staging edge stays removed.
	if _, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx); err != nil {
		t.Fatal(err.Error())
	}
	if !env.blockExists(t, ref) {
		t.Fatal("parent-owned block should survive collection")
	}

	// Read staged nodes after the concurrent ownership transition.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify parent ownership leaves no stale staging edge.
	if slices.Contains(nodes, object) {
		t.Fatal("parent-owned block should not be staged as unreferenced")
	}
}

// buildBatchEntry creates a PutBatchEntry from a mock block message.
func buildBatchEntry(t *testing.T, msg string) *block.PutBatchEntry {
	// Encode the mock block payload for a batch entry.
	t.Helper()
	ex := block_mock.NewExample(msg)
	data, err := ex.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the batch reference from the encoded payload.
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return &block.PutBatchEntry{Ref: ref, Data: data}
}

// TestGCStoreOps_PutBlockBatch_DuplicateNoNewUnrefEdge tests that
// PutBlockBatch does not revive unreferenced edges for blocks that
// already exist in the store.
func TestGCStoreOps_PutBlockBatch_DuplicateNoNewUnrefEdge(t *testing.T) {
	// Create a GC store for the duplicate batch write.
	env := newGCTestEnv(t)

	// Put a block via single put and root it.
	ref := env.putBlock(t, "batch-dup")
	blockIRI := BlockIRI(ref)
	if err := env.gcStore.AddGCRef(env.ctx, "entity:root", blockIRI); err != nil {
		t.Fatal(err.Error())
	}

	// Block should not be unreferenced (it has a real ref).
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the rooted block begins outside the staging set.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced before batch, got %d", len(nodes))
	}

	// Re-write the same block via batch path.
	entry := buildBatchEntry(t, "batch-dup")
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{entry}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Should still have 0 unreferenced nodes. The batch path must
	// not revive the unreferenced edge for an already-existing block.
	nodes, err = env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the duplicate batch write preserves the empty staging set.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 unreferenced after batch dup, got %d", len(nodes))
	}
}

// TestGCStoreOps_PutBlockBatch_NewBlockAddsUnrefEdge tests that
// PutBlockBatch adds unreferenced edges for genuinely new blocks.
func TestGCStoreOps_PutBlockBatch_NewBlockAddsUnrefEdge(t *testing.T) {
	// Create a GC store for the new batch entries.
	env := newGCTestEnv(t)

	// Write two new blocks through the batch path.
	e1 := buildBatchEntry(t, "batch-new-a")
	e2 := buildBatchEntry(t, "batch-new-b")
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{e1, e2}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Read staged nodes after writing the new batch.
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify both new blocks are staged.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 unreferenced nodes from batch, got %d", len(nodes))
	}
}

// buildTreeEntry creates a batch entry for data that references children.
func buildTreeEntry(t *testing.T, data string, children ...*block.PutBatchEntry) *block.PutBatchEntry {
	t.Helper()

	// Build the entry's ref from its bytes.
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Point the entry at its children.
	entry := &block.PutBatchEntry{Ref: ref, Data: []byte(data)}
	for _, child := range children {
		entry.Refs = append(entry.Refs, child.Ref)
	}
	return entry
}

// sortedEdges returns edges ordered by subject and object.
func sortedEdges(edges []RefEdge) []RefEdge {
	sorted := slices.Clone(edges)
	slices.SortFunc(sorted, func(a, b RefEdge) int {
		return cmp.Or(cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Object, b.Object))
	})
	return sorted
}

// TestGCStoreOps_ParentIRI_PutBlockBatchExistingTreeBuffersOwnerOnly tests that
// writing a stored tree again buffers one owner edge and one staging removal per
// block, and no child edges.
func TestGCStoreOps_ParentIRI_PutBlockBatchExistingTreeBuffersOwnerOnly(t *testing.T) {
	// Build a three-block chain.
	ctx := context.Background()
	leaf := buildTreeEntry(t, "tree-leaf")
	mid := buildTreeEntry(t, "tree-mid", leaf)
	root := buildTreeEntry(t, "tree-root", mid)
	tree := []*block.PutBatchEntry{leaf, mid, root}

	// Write the chain once and flush its edges.
	parent := BucketIRI("tree-bucket")
	refGraph := &recordingRefGraph{}
	rawStore := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	gcStore := NewGCStoreOpsWithParent(rawStore, refGraph, parent)
	existed, err := gcStore.PutBlockBatch(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(existed, true) {
		t.Fatalf("first write reported existing blocks: %v", existed)
	}

	// A new tree records its owner edges and its child edges.
	if err := gcStore.FlushPending(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if len(refGraph.adds) != 5 {
		t.Fatalf("first write adds = %v, want 3 owner and 2 child edges", refGraph.adds)
	}

	// Write the same chain again.
	existed, err = gcStore.PutBlockBatch(ctx, tree)
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(existed, false) {
		t.Fatalf("second write reported new blocks: %v", existed)
	}
	if err := gcStore.FlushPending(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// Each block gets its owner edge and loses its staging edge, and nothing else.
	var wantAdds, wantRemoves []RefEdge
	for _, entry := range tree {
		iri := BlockIRI(entry.Ref)
		wantAdds = append(wantAdds, RefEdge{Subject: parent, Object: iri})
		wantRemoves = append(wantRemoves, RefEdge{Subject: NodeUnreferenced, Object: iri})
	}
	if !slices.Equal(sortedEdges(refGraph.adds), sortedEdges(wantAdds)) {
		t.Fatalf("adds = %v, want %v", refGraph.adds, wantAdds)
	}
	if !slices.Equal(sortedEdges(refGraph.removes), sortedEdges(wantRemoves)) {
		t.Fatalf("removes = %v, want %v", refGraph.removes, wantRemoves)
	}
}

// TestGCStoreOps_ParentIRI_SweepKeepsMixedNewAndReputWrites tests that a sweep
// keeps every reachable block after a batch that mixes new blocks with blocks
// stored earlier, once the bucket releases its direct edges to the children.
func TestGCStoreOps_ParentIRI_SweepKeepsMixedNewAndReputWrites(t *testing.T) {
	// Root the bucket.
	parent := BucketIRI("mixed-bucket")
	env := newGCTestEnvWithParent(t, parent)
	if err := env.refGraph.AddRef(env.ctx, NodeGCRoot, parent); err != nil {
		t.Fatal(err.Error())
	}

	// Build a chain and a sibling root sharing its leaf, and write the chain.
	leaf := buildTreeEntry(t, "mixed-leaf")
	mid := buildTreeEntry(t, "mixed-mid", leaf)
	root := buildTreeEntry(t, "mixed-root", mid)
	sibling := buildTreeEntry(t, "mixed-sibling", leaf)
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{leaf, mid, root}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// The second batch re-puts the chain and adds the sibling.
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{leaf, mid, root, sibling}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Release every direct edge except the two roots.
	for _, entry := range []*block.PutBatchEntry{leaf, mid} {
		if err := env.refGraph.RemoveRef(env.ctx, parent, BlockIRI(entry.Ref)); err != nil {
			t.Fatal(err.Error())
		}
	}

	// The sweep keeps all four blocks: the chain through its recorded edges.
	if _, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx); err != nil {
		t.Fatal(err.Error())
	}
	for _, entry := range []*block.PutBatchEntry{leaf, mid, root, sibling} {
		if !env.blockExists(t, entry.Ref) {
			t.Fatalf("reachable block %q was swept", entry.Data)
		}
	}
}

type recordingRefGraph struct {
	adds       []RefEdge
	removes    []RefEdge
	applyCount int
}

func (r *recordingRefGraph) AddRef(context.Context, string, string) error {
	return nil
}

func (r *recordingRefGraph) RemoveRef(context.Context, string, string) error {
	return nil
}

func (r *recordingRefGraph) ApplyRefBatch(_ context.Context, adds, removes []RefEdge) error {
	r.applyCount++
	r.adds = append([]RefEdge(nil), adds...)
	r.removes = append([]RefEdge(nil), removes...)
	return nil
}

type injectUnrefRefGraph struct {
	RefGraphOps
	object   string
	injected bool
}

func (r *injectUnrefRefGraph) ApplyRefBatch(
	ctx context.Context,
	adds, removes []RefEdge,
) error {
	if !r.injected {
		r.injected = true
		if err := r.AddRef(ctx, NodeUnreferenced, r.object); err != nil {
			return err
		}
	}
	return r.RefGraphOps.ApplyRefBatch(ctx, adds, removes)
}

type orphanRaceRefGraph struct {
	RefGraphOps
	removerStarted chan struct{}
	parentDone     chan struct{}
	startOnce      sync.Once
	parentOnce     sync.Once
}

func (r *orphanRaceRefGraph) signalRemoverStarted() {
	r.startOnce.Do(func() { close(r.removerStarted) })
}

func (r *orphanRaceRefGraph) signalParentDone() {
	r.parentOnce.Do(func() { close(r.parentDone) })
}

func (r *orphanRaceRefGraph) ApplyRefBatch(
	ctx context.Context,
	adds, removes []RefEdge,
) error {
	// Hold the removal batch until the parent ownership batch completes.
	removalOnly := len(adds) == 0 && len(removes) == 1 && removes[0].Subject != NodeUnreferenced
	if removalOnly {
		r.signalRemoverStarted()
		<-r.parentDone
	}

	// Apply the graph batch and signal completion of parent additions.
	err := r.RefGraphOps.ApplyRefBatch(ctx, adds, removes)
	if len(adds) != 0 {
		r.signalParentDone()
	}
	return err
}

func (r *orphanRaceRefGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	r.signalRemoverStarted()
	<-r.parentDone
	return false, nil
}

func (r *recordingRefGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, nil
}

func (r *recordingRefGraph) HasIncomingRefs(context.Context, string) (bool, error) {
	return false, nil
}

func (r *recordingRefGraph) HasIncomingRefsExcluding(context.Context, string, ...string) (bool, error) {
	return false, nil
}

func (r *recordingRefGraph) GetOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *recordingRefGraph) GetIncomingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *recordingRefGraph) GetUnreferencedNodes(context.Context) ([]string, error) {
	return nil, nil
}

func (r *recordingRefGraph) AddBlockRef(context.Context, *block.BlockRef, *block.BlockRef) error {
	return nil
}

func (r *recordingRefGraph) AddObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}

func (r *recordingRefGraph) RemoveObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}

func (r *recordingRefGraph) Close() error {
	return nil
}

// _ is a type assertion
var _ RefGraphOps = (*recordingRefGraph)(nil)

func TestGCStoreOpsGetStoredBlock(t *testing.T) {
	// Store a parent block that references a child block.
	env := newGCTestEnv(t)
	child := env.putBlock(t, "child")
	data := []byte("parent")
	parent, _, err := env.gcStore.PutBlock(env.ctx, data, &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Define checks for stored payloads and references at each flush stage.
	check := func(stage string) {
		// Read and verify the stored parent block payload.
		t.Helper()
		got, err := env.gcStore.GetStoredBlock(env.ctx, parent)
		if err != nil || got == nil || string(got.Data) != string(data) || !got.RefsKnown {
			t.Fatalf("%s: parent = %+v/%v", stage, got, err)
		}

		// Verify the stored parent references its child.
		if len(got.Refs) != 1 || !got.Refs[0].EqualsRef(child) {
			t.Fatalf("%s: parent refs = %v, want [%v]", stage, got.Refs, child)
		}

		// Verify the stored child is a known leaf block.
		got, err = env.gcStore.GetStoredBlock(env.ctx, child)
		if err != nil || got == nil || !got.RefsKnown || len(got.Refs) != 0 {
			t.Fatalf("%s: child = %+v/%v, want leaf", stage, got, err)
		}
	}

	// Check stored blocks before and after flushing references.
	check("buffered")
	env.flush(t)
	check("flushed")

	// Read the parent edges from the reference graph.
	targets, err := env.refGraph.GetOutgoingRefs(env.ctx, BlockIRI(parent))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the parent edge targets the stored child.
	if !slices.Equal(targets, []string{BlockIRI(child)}) {
		t.Fatalf("ref graph targets = %v, want [%s]", targets, BlockIRI(child))
	}

	// Build a reference for a block absent from the store.
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify reading the absent block returns no stored record.
	got, err := env.gcStore.GetStoredBlock(env.ctx, missing)
	if err != nil || got != nil {
		t.Fatalf("missing = %+v/%v, want not found", got, err)
	}
}

// flushingStore flushes each batch like a bounded buffered writer, so a copy
// spans many ownership flushes.
type flushingStore struct {
	*GCStoreOps
}

func (s flushingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Write the batch, then deliver its ownership changes.
	existed, err := s.GCStoreOps.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	return existed, s.FlushPending(ctx)
}

// TestGCStoreOps_ParentIRI_CacheFillTakesNoOwnership tests that a cache fill
// leaves ownership alone: an existing block keeps its owner, a new copy stays a
// garbage candidate, and the copy does not adopt a block the owner staged.
func TestGCStoreOps_ParentIRI_CacheFillTakesNoOwnership(t *testing.T) {
	// Stage a written block under the bucket.
	parent := BucketIRI("fill-bucket")
	env := newGCTestEnvWithParent(t, parent)
	put := func(data string, opts *block.PutOpts) *block.BlockRef {
		ref, _, err := env.gcStore.PutBlock(env.ctx, []byte(data), opts)
		if err != nil {
			t.Fatal(err.Error())
		}
		env.flush(t)
		return ref
	}
	written := put("written", nil)

	// Fill the written block again and a new block that references it.
	put("written", &block.PutOpts{CacheFill: true})
	cached := put("cached", &block.PutOpts{Refs: []*block.BlockRef{written}, CacheFill: true})

	// The bucket still owns only the written block.
	owned, err := env.refGraph.GetOutgoingRefs(env.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(owned, []string{BlockIRI(written)}) {
		t.Fatalf("bucket owns %v, want only %s", owned, BlockIRI(written))
	}
	nodes, err := env.refGraph.GetUnreferencedNodes(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(nodes, []string{BlockIRI(cached)}) {
		t.Fatalf("unreferenced nodes = %v, want only the cached copy", nodes)
	}

	// Collection sweeps the unreachable copy and keeps the written block.
	if _, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx); err != nil {
		t.Fatal(err.Error())
	}
	if env.blockExists(t, cached) || !env.blockExists(t, written) {
		t.Fatal("collection should sweep only the cached copy")
	}
}

// TestGCStoreOps_ParentIRI_CopyGraphHandsChildrenToParents tests that a graph
// copied into a parented store leaves the parent owning only the root, so
// replacing the root releases the whole copy.
func TestGCStoreOps_ParentIRI_CopyGraphHandsChildrenToParents(t *testing.T) {
	// Build a source graph with recorded edges.
	src := newGCTestEnv(t)
	put := func(data string, refs ...*block.BlockRef) *block.BlockRef {
		ref, _, err := src.gcStore.PutBlock(src.ctx, []byte(data), &block.PutOpts{Refs: refs})
		if err != nil {
			t.Fatal(err.Error())
		}
		return ref
	}
	leaf := put("leaf")
	mid := put("mid", leaf)
	root := put("root", mid, put("side"))
	src.flush(t)

	// Copy it into a bucket store that flushes after every write.
	parent := BucketIRI("copy-bucket")
	dst := newGCTestEnvWithParent(t, parent)
	if err := block.CopyGraph(src.ctx, src.gcStore, flushingStore{dst.gcStore}, root, nil); err != nil {
		t.Fatal(err.Error())
	}

	// Every copied child belongs to its parent block, not the bucket.
	owned, err := dst.refGraph.GetOutgoingRefs(dst.ctx, parent)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(owned, []string{BlockIRI(root)}) {
		t.Fatalf("parent owns %v, want only the root %s", owned, BlockIRI(root))
	}
}
