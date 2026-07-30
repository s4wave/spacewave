package world_block

import (
	"context"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestWorldStateCommitCarriesExactRefEdgeIndex holds the RefGraph exact-edge
// index equal to the durable GC graph across world commits. Commit hands the
// live index to the RefGraph it rebuilds instead of walking the graph, so a
// carried index that outruns or lags the committed edges would go unnoticed
// until orphan detection swept a referenced block.
func TestWorldStateCommitCarriesExactRefEdgeIndex(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	// The GC tree runs as an isolated block transaction, so its writes reach
	// the world cursor only through the commit below. That is what makes the
	// discarded leg of this test a real rollback rather than a replay.
	if !ws.gcTreeIsolated {
		t.Fatal("expected the GC tree to commit through an isolated block transaction")
	}

	verify := func(step string) {
		t.Helper()

		rg, ok := ws.refGraph.(*block_gc.RefGraph)
		if !ok {
			t.Fatalf("%s: refgraph is %T, want *block_gc.RefGraph", step, ws.refGraph)
		}
		if err := rg.VerifyEdgeIndex(ctx); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}

	verify("before any commit")

	for _, key := range []string{"carry/a", "carry/b"} {
		if _, err := BuildMockObject(ctx, ws, key); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	verify("after committing two added objects")

	deleted, err := ws.DeleteObject(ctx, "carry/a")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected carry/a to exist before deletion")
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	verify("after committing one removed object")

	// Mutate the ref graph and rebuild onto the last committed cursor without
	// committing. The isolated GC tree is discarded, so the rebuilt graph holds
	// the committed edges and the abandoned index must not survive.
	if _, err := BuildMockObject(ctx, ws, "carry/discarded"); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetBlockTransaction(ctx, ws.btx, ws.bcs); err != nil {
		t.Fatal(err.Error())
	}
	verify("after discarding a transaction")

	if _, err := BuildMockObject(ctx, ws, "carry/c"); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	verify("after committing past a discarded transaction")
}

// TestBuildGCTreeDropsCarriedIndexOnEmptyTree covers the one condition under
// which a carried index cannot describe the GC tree being opened. A world state
// that has never committed writes its gcroot edge into an isolated GC tree, so
// rebuilding from its cursor lands on an empty tree while the live index still
// holds that edge.
func TestBuildGCTreeDropsCarriedIndexOnEmptyTree(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	carried := ws.refGraph.TransferRefEdgeIndex()
	gcTree, rg, _, _, err := ws.buildGCTree(ctx, ws.bcs, carried)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer func() {
		_ = rg.Close()
		gcTree.Discard()
	}()

	size, err := gcTree.Size(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if size != 0 {
		t.Fatalf("gc tree size = %d, want 0 so the carried index has something to be wrong about", size)
	}
	if err := rg.VerifyEdgeIndex(ctx); err != nil {
		t.Fatal(err.Error())
	}
}
