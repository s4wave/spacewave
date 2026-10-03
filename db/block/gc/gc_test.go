package block_gc

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// testEnv holds the test environment for collector tests.
type testEnv struct {
	ctx      context.Context
	kvStore  *store_kvtx_inmem.Store
	rawStore block.StoreOps
	gcStore  *GCStoreOps
	refGraph *RefGraph
	gc       *Collector
	swept    []string
}

// newTestEnv creates a test environment with a GCStoreOps-wrapped store
// and a Collector that tracks swept nodes via onSwept callback.
func newTestEnv(t *testing.T) *testEnv {
	// Create an in-memory block store for collector tests.
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

	// Create a collector that records swept nodes.
	gcStore := NewGCStoreOps(rawStore, rg)
	env := &testEnv{
		ctx:      ctx,
		kvStore:  kvStore,
		rawStore: rawStore,
		gcStore:  gcStore,
		refGraph: rg,
	}
	env.gc = NewCollector(rg, rawStore, func(_ context.Context, iri string) error {
		env.swept = append(env.swept, iri)
		return nil
	})
	return env
}

// putBlock stores a mock block via GCStoreOps and returns its ref.
// Flushes pending gc operations so the ref graph is up to date.
func (e *testEnv) putBlock(t *testing.T, msg string) *block.BlockRef {
	// Store the mock block through GC reference tracking.
	t.Helper()
	ex := block_mock.NewExample(msg)
	ref, _, err := block.PutBlock(e.ctx, e.gcStore, ex)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Flush the new block reference changes into the graph.
	if err := e.gcStore.FlushPending(e.ctx); err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

// flush writes buffered GCStoreOps operations to the ref graph.
func (e *testEnv) flush(t *testing.T) {
	t.Helper()
	if err := e.gcStore.FlushPending(e.ctx); err != nil {
		t.Fatal(err.Error())
	}
}

// recordRefs buffers the edges a writer's put of source records.
func (e *testEnv) recordRefs(source *block.BlockRef, targets []*block.BlockRef) {
	e.gcStore.mu.Lock()
	e.gcStore.bufferRefEdgesLocked(source, targets, true)
	e.gcStore.mu.Unlock()
}

// blockExists checks if a block exists in the raw store.
func (e *testEnv) blockExists(t *testing.T, ref *block.BlockRef) bool {
	t.Helper()
	exists, err := e.rawStore.GetBlockExists(e.ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	return exists
}

// TestCollector_EmptyStore tests GC on an empty store.
func TestCollector_EmptyStore(t *testing.T) {
	// Create an empty collector test environment.
	env := newTestEnv(t)

	// Collect the empty block store.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the empty store produces no swept nodes.
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept, got %d", stats.NodesSwept)
	}
}

// TestCollector_AllRooted tests that all rooted blocks survive.
func TestCollector_AllRooted(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write the three blocks that form the rooted chain.
	a := env.putBlock(t, "block-a")
	b := env.putBlock(t, "block-b")
	c := env.putBlock(t, "block-c")

	// a -> b -> c
	env.recordRefs(a, []*block.BlockRef{b})
	env.recordRefs(b, []*block.BlockRef{c})

	// Root at a.
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(a))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Collect blocks after rooting the chain.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the collector preserves all rooted nodes.
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept, got %d", stats.NodesSwept)
	}

	// Verify every block in the rooted chain remains stored.
	for _, ref := range []*block.BlockRef{a, b, c} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should still exist", ref.MarshalString())
		}
	}
}

// TestCollector_OrphanBlocks tests that orphan blocks are swept.
func TestCollector_OrphanBlocks(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write a rooted block and an orphan block.
	rooted := env.putBlock(t, "rooted")
	orphan := env.putBlock(t, "orphan")

	// Root the first block.
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(rooted))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Collect blocks after rooting only the retained block.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the collector sweeps exactly one orphan.
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	// Verify the rooted block remains stored.
	if !env.blockExists(t, rooted) {
		t.Fatal("rooted block should still exist")
	}

	// Verify the orphan block is removed.
	if env.blockExists(t, orphan) {
		t.Fatal("orphan block should have been swept")
	}
}

func TestCollectorGraphOnlyPreservesOrphanBlocks(t *testing.T) {
	// Write an orphan block for graph-only collection.
	env := newTestEnv(t)
	orphan := env.putBlock(t, "orphan")

	// Collect graph records while preserving physical blocks.
	stats, err := env.gc.CollectGraphOnly(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify graph-only collection sweeps the node without deleting its block.
	if stats.NodesSwept != 1 {
		t.Fatalf("nodes swept = %d, want 1", stats.NodesSwept)
	}
	if stats.RemoveBlockCount != 0 {
		t.Fatalf("blocks removed = %d, want 0", stats.RemoveBlockCount)
	}

	// Verify the physical orphan block remains stored.
	if !env.blockExists(t, orphan) {
		t.Fatal("graph-only collection removed the physical block")
	}
}

// TestCollector_CascadingOrphans tests that removing a root cascades
// through the reference chain, orphaning and sweeping all descendants.
func TestCollector_CascadingOrphans(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write the three blocks that form the cascading chain.
	a := env.putBlock(t, "block-a")
	b := env.putBlock(t, "block-b")
	c := env.putBlock(t, "block-c")

	// a -> b -> c
	env.recordRefs(a, []*block.BlockRef{b})
	env.recordRefs(b, []*block.BlockRef{c})

	// Root at a.
	aIRI := BlockIRI(a)
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, aIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// All alive.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept before root removal, got %d", stats.NodesSwept)
	}

	// Remove the root. a becomes orphaned, cascading to b and c.
	err = env.gcStore.RemoveGCRef(env.ctx, NodeGCRoot, aIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Collect the orphaned chain after root removal.
	stats, err = env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the collector sweeps all three chain nodes.
	if stats.NodesSwept != 3 {
		t.Fatalf("expected 3 swept after cascading, got %d", stats.NodesSwept)
	}

	// Verify every block in the orphaned chain is removed.
	for _, ref := range []*block.BlockRef{a, b, c} {
		if env.blockExists(t, ref) {
			t.Fatalf("block %s should have been swept", ref.MarshalString())
		}
	}
}

// TestCollector_DiamondDAG tests that a shared block in a diamond survives.
func TestCollector_DiamondDAG(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write the four blocks that form the diamond graph.
	root := env.putBlock(t, "root")
	b := env.putBlock(t, "block-b")
	c := env.putBlock(t, "block-c")
	d := env.putBlock(t, "block-d")

	// root -> b, root -> c, b -> d, c -> d
	env.recordRefs(root, []*block.BlockRef{b, c})
	env.recordRefs(b, []*block.BlockRef{d})
	env.recordRefs(c, []*block.BlockRef{d})
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(root))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Collect blocks after rooting the diamond graph.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection preserves all diamond nodes.
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept in diamond DAG, got %d", stats.NodesSwept)
	}

	// Verify every block in the diamond remains stored.
	for _, ref := range []*block.BlockRef{root, b, c, d} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should survive diamond DAG", ref.MarshalString())
		}
	}
}

// TestCollector_DiamondPartialRemove tests that removing one path in a
// diamond keeps the shared child alive via the other path.
func TestCollector_DiamondPartialRemove(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write the four blocks that form the diamond graph.
	root := env.putBlock(t, "root")
	b := env.putBlock(t, "block-b")
	c := env.putBlock(t, "block-c")
	d := env.putBlock(t, "block-d")

	// root -> b, root -> c, b -> d, c -> d
	env.recordRefs(root, []*block.BlockRef{b, c})
	env.recordRefs(b, []*block.BlockRef{d})
	env.recordRefs(c, []*block.BlockRef{d})
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(root))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Remove b via GCStoreOps.RmBlock: d should survive via c's reference.
	err = env.gcStore.RmBlock(env.ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Collect the diamond after removing one intermediate block.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// b was already cleaned from the graph; no unreferenced blocks remain.
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept, got %d", stats.NodesSwept)
	}

	// d should still exist (c still references it).
	if !env.blockExists(t, d) {
		t.Fatal("d should survive via c's reference")
	}
}

// TestCollector_MultipleRoots tests that each root's DAG survives.
func TestCollector_MultipleRoots(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write two rooted chains and an independent orphan.
	a := env.putBlock(t, "root-a")
	b := env.putBlock(t, "child-of-a")
	c := env.putBlock(t, "root-c")
	d := env.putBlock(t, "child-of-c")
	orphan := env.putBlock(t, "orphan")

	// Register both rooted chains and flush their reference changes.
	env.recordRefs(a, []*block.BlockRef{b})
	env.recordRefs(c, []*block.BlockRef{d})
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(a))
	if err != nil {
		t.Fatal(err.Error())
	}
	err = env.gcStore.AddGCRef(env.ctx, "entity:pin", BlockIRI(c))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Collect blocks with two independent roots.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify only the independent orphan is swept.
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	// Verify both rooted chains remain stored.
	for _, ref := range []*block.BlockRef{a, b, c, d} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should survive", ref.MarshalString())
		}
	}

	// Verify the independent orphan is removed.
	if env.blockExists(t, orphan) {
		t.Fatal("orphan should have been swept")
	}
}

// TestCollector_Idempotent tests that a second GC run sweeps nothing.
func TestCollector_Idempotent(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write a retained block and an orphan block.
	a := env.putBlock(t, "rooted")
	env.putBlock(t, "orphan")

	// Root the retained block and flush its reference changes.
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(a))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Run the first collection over the orphan block.
	stats1, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the first collection sweeps exactly one orphan.
	if stats1.NodesSwept != 1 {
		t.Fatalf("first run: expected 1 swept, got %d", stats1.NodesSwept)
	}

	// Run collection again over the remaining rooted graph.
	stats2, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify repeated collection sweeps no additional nodes.
	if stats2.NodesSwept != 0 {
		t.Fatalf("second run: expected 0 swept, got %d", stats2.NodesSwept)
	}
}

// TestCollector_SweepCleansGraph tests that swept blocks have their
// graph edges removed.
func TestCollector_SweepCleansGraph(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write a retained block and an orphan parent-child pair.
	rooted := env.putBlock(t, "rooted")
	orphan := env.putBlock(t, "orphan")
	orphanChild := env.putBlock(t, "orphan-child")

	// orphan -> orphanChild
	env.recordRefs(orphan, []*block.BlockRef{orphanChild})
	env.flush(t)
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(rooted))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Collect the orphan parent-child pair.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// orphan has unreferenced edge, orphanChild doesn't (block refs removed it).
	// First iteration: delete orphan -> cascades orphanChild to unreferenced.
	// Second iteration: delete orphanChild.
	if stats.NodesSwept != 2 {
		t.Fatalf("expected 2 swept, got %d", stats.NodesSwept)
	}

	// Read outgoing edges of the swept orphan parent.
	orphanIRI := BlockIRI(orphan)
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, orphanIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection removes the orphan parent edges.
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing refs from orphan after sweep, got %d", len(outgoing))
	}

	// Read incoming references of the swept orphan child.
	orphanChildIRI := BlockIRI(orphanChild)
	hasRefs, err := env.refGraph.HasIncomingRefs(env.ctx, orphanChildIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection removes the orphan child references.
	if hasRefs {
		t.Fatal("expected no incoming refs for orphanChild after sweep")
	}
}

// TestCollector_NoRoots tests that blocks with no real roots are all swept.
func TestCollector_NoRoots(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write two blocks with no real root.
	env.putBlock(t, "orphan-1")
	env.putBlock(t, "orphan-2")

	// Collect the unrooted blocks.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection sweeps both unrooted blocks.
	if stats.NodesSwept != 2 {
		t.Fatalf("expected 2 swept, got %d", stats.NodesSwept)
	}
}

// TestCollector_EntityHierarchyCascade tests cascading through an
// entity hierarchy: gcroot -> plugin -> provider -> blockstore -> blocks.
func TestCollector_EntityHierarchyCascade(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write the block retained through the entity hierarchy.
	blk := env.putBlock(t, "data-block")
	blkIRI := BlockIRI(blk)

	// Build entity hierarchy with generic IRIs.
	pluginIRI := ObjectIRI("plugin:my-plugin")
	providerIRI := ObjectIRI("provider:my-provider")
	bstoreIRI := ObjectIRI("blockstore:my-bstore")

	// gcroot -> plugin -> provider -> blockstore -> block
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, pluginIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = env.gcStore.AddGCRef(env.ctx, pluginIRI, providerIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = env.gcStore.AddGCRef(env.ctx, providerIRI, bstoreIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = env.gcStore.AddGCRef(env.ctx, bstoreIRI, blkIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Nothing should be swept.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept with entity hierarchy, got %d", stats.NodesSwept)
	}
	if !env.blockExists(t, blk) {
		t.Fatal("block should exist under entity hierarchy")
	}

	// Remove root -> plugin. Everything cascades.
	err = env.gcStore.RemoveGCRef(env.ctx, NodeGCRoot, pluginIRI)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Collect the entity hierarchy after removing its root edge.
	env.swept = nil
	stats, err = env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// plugin, provider, blockstore, block = 4 nodes swept.
	if stats.NodesSwept != 4 {
		t.Fatalf("expected 4 swept after entity cascade, got %d", stats.NodesSwept)
	}
	if env.blockExists(t, blk) {
		t.Fatal("block should have been swept")
	}
}

// TestCollector_OnSweptCallback tests that the onSwept callback is
// called for each swept node.
func TestCollector_OnSweptCallback(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write two orphan blocks for callback verification.
	env.putBlock(t, "orphan-1")
	env.putBlock(t, "orphan-2")

	// Collect the orphan blocks while recording swept callbacks.
	env.swept = nil
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection sweeps both orphan nodes.
	if stats.NodesSwept != 2 {
		t.Fatalf("expected 2 swept, got %d", stats.NodesSwept)
	}

	// Verify each swept node produces one callback.
	if len(env.swept) != 2 {
		t.Fatalf("expected onSwept called 2 times, got %d", len(env.swept))
	}
}

// TestCollector_PermanentRootsNeverSwept tests that permanent root
// nodes are never swept even if they appear unreferenced.
func TestCollector_PermanentRootsNeverSwept(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Add an unreferenced edge pointing to the gcroot node itself
	// (this shouldn't happen in practice but tests the safety check).
	err := env.refGraph.AddRef(env.ctx, NodeUnreferenced, NodeGCRoot)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Collect the graph containing a staged permanent root.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the permanent root survives collection.
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept (permanent root), got %d", stats.NodesSwept)
	}
}

func TestCollector_PhaseStats(t *testing.T) {
	// Create the block store, reference graph, and collector for the test.
	env := newTestEnv(t)

	// Write a block and register its root edge.
	ref := env.putBlock(t, "phase-stats")
	if err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(ref)); err != nil {
		t.Fatal(err.Error())
	}

	// Remove the root edge to make the block collectible.
	if err := env.gcStore.RemoveGCRef(env.ctx, NodeGCRoot, BlockIRI(ref)); err != nil {
		t.Fatal(err.Error())
	}

	// Collect the block and capture phase statistics.
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify collection records one swept node.
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	// Verify collection records the orphan graph cleanup.
	if stats.UnreferencedNodeCount == 0 {
		t.Fatal("expected unreferenced node count")
	}
	if stats.RemoveNodeRefsCount != 1 {
		t.Fatalf("expected 1 node-ref removal, got %d", stats.RemoveNodeRefsCount)
	}
	if stats.RemoveUnreferencedEdgeCount != 1 {
		t.Fatalf("expected 1 unreferenced edge removal, got %d", stats.RemoveUnreferencedEdgeCount)
	}

	// Verify collection records the callback and physical deletion.
	if stats.OnSweptCount != 1 {
		t.Fatalf("expected 1 onSwept call, got %d", stats.OnSweptCount)
	}
	if stats.RemoveBlockCount != 1 {
		t.Fatalf("expected 1 physical block delete, got %d", stats.RemoveBlockCount)
	}
}
