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
func (e *testEnv) flush(t *testing.T) {
	t.Helper()
	if err := e.gcStore.FlushPending(e.ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func (e *testEnv) recordRefs(source *block.BlockRef, targets []*block.BlockRef) {
	e.gcStore.bufferBlockRefs(source, targets)
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
	env := newTestEnv(t)

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept, got %d", stats.NodesSwept)
	}
}

// TestCollector_AllRooted tests that all rooted blocks survive.
func TestCollector_AllRooted(t *testing.T) {
	env := newTestEnv(t)

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

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept, got %d", stats.NodesSwept)
	}

	for _, ref := range []*block.BlockRef{a, b, c} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should still exist", ref.MarshalString())
		}
	}
}

// TestCollector_OrphanBlocks tests that orphan blocks are swept.
func TestCollector_OrphanBlocks(t *testing.T) {
	env := newTestEnv(t)

	rooted := env.putBlock(t, "rooted")
	orphan := env.putBlock(t, "orphan")

	// Root the first block.
	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(rooted))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	if !env.blockExists(t, rooted) {
		t.Fatal("rooted block should still exist")
	}
	if env.blockExists(t, orphan) {
		t.Fatal("orphan block should have been swept")
	}
}

// TestCollector_CascadingOrphans tests that removing a root cascades
// through the reference chain, orphaning and sweeping all descendants.
func TestCollector_CascadingOrphans(t *testing.T) {
	env := newTestEnv(t)

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

	stats, err = env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 3 {
		t.Fatalf("expected 3 swept after cascading, got %d", stats.NodesSwept)
	}

	for _, ref := range []*block.BlockRef{a, b, c} {
		if env.blockExists(t, ref) {
			t.Fatalf("block %s should have been swept", ref.MarshalString())
		}
	}
}

// TestCollector_DiamondDAG tests that a shared block in a diamond survives.
func TestCollector_DiamondDAG(t *testing.T) {
	env := newTestEnv(t)

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

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept in diamond DAG, got %d", stats.NodesSwept)
	}

	for _, ref := range []*block.BlockRef{root, b, c, d} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should survive diamond DAG", ref.MarshalString())
		}
	}
}

// TestCollector_DiamondPartialRemove tests that removing one path in a
// diamond keeps the shared child alive via the other path.
func TestCollector_DiamondPartialRemove(t *testing.T) {
	env := newTestEnv(t)

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
	env := newTestEnv(t)

	a := env.putBlock(t, "root-a")
	b := env.putBlock(t, "child-of-a")
	c := env.putBlock(t, "root-c")
	d := env.putBlock(t, "child-of-c")
	orphan := env.putBlock(t, "orphan")

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

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 1 {
		t.Fatalf("expected 1 swept, got %d", stats.NodesSwept)
	}

	for _, ref := range []*block.BlockRef{a, b, c, d} {
		if !env.blockExists(t, ref) {
			t.Fatalf("block %s should survive", ref.MarshalString())
		}
	}
	if env.blockExists(t, orphan) {
		t.Fatal("orphan should have been swept")
	}
}

// TestCollector_Idempotent tests that a second GC run sweeps nothing.
func TestCollector_Idempotent(t *testing.T) {
	env := newTestEnv(t)

	a := env.putBlock(t, "rooted")
	env.putBlock(t, "orphan")

	err := env.gcStore.AddGCRef(env.ctx, NodeGCRoot, BlockIRI(a))
	if err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	stats1, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats1.NodesSwept != 1 {
		t.Fatalf("first run: expected 1 swept, got %d", stats1.NodesSwept)
	}

	stats2, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats2.NodesSwept != 0 {
		t.Fatalf("second run: expected 0 swept, got %d", stats2.NodesSwept)
	}
}

// TestCollector_SweepCleansGraph tests that swept blocks have their
// graph edges removed.
func TestCollector_SweepCleansGraph(t *testing.T) {
	env := newTestEnv(t)

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

	orphanIRI := BlockIRI(orphan)
	outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, orphanIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(outgoing) != 0 {
		t.Fatalf("expected 0 outgoing refs from orphan after sweep, got %d", len(outgoing))
	}

	orphanChildIRI := BlockIRI(orphanChild)
	hasRefs, err := env.refGraph.HasIncomingRefs(env.ctx, orphanChildIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if hasRefs {
		t.Fatal("expected no incoming refs for orphanChild after sweep")
	}
}

// TestCollector_NoRoots tests that blocks with no real roots are all swept.
func TestCollector_NoRoots(t *testing.T) {
	env := newTestEnv(t)

	env.putBlock(t, "orphan-1")
	env.putBlock(t, "orphan-2")

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 2 {
		t.Fatalf("expected 2 swept, got %d", stats.NodesSwept)
	}
}

// TestCollector_EntityHierarchyCascade tests cascading through an
// entity hierarchy: gcroot -> plugin -> provider -> blockstore -> blocks.
func TestCollector_EntityHierarchyCascade(t *testing.T) {
	env := newTestEnv(t)

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
	env := newTestEnv(t)

	env.putBlock(t, "orphan-1")
	env.putBlock(t, "orphan-2")

	env.swept = nil
	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 2 {
		t.Fatalf("expected 2 swept, got %d", stats.NodesSwept)
	}
	if len(env.swept) != 2 {
		t.Fatalf("expected onSwept called 2 times, got %d", len(env.swept))
	}
}

// TestCollector_PermanentRootsNeverSwept tests that permanent root
// nodes are never swept even if they appear unreferenced.
func TestCollector_PermanentRootsNeverSwept(t *testing.T) {
	env := newTestEnv(t)

	// Add an unreferenced edge pointing to the gcroot node itself
	// (this shouldn't happen in practice but tests the safety check).
	err := env.refGraph.AddRef(env.ctx, NodeUnreferenced, NodeGCRoot)
	if err != nil {
		t.Fatal(err.Error())
	}

	stats, err := env.gc.Collect(env.ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept != 0 {
		t.Fatalf("expected 0 swept (permanent root), got %d", stats.NodesSwept)
	}
}
