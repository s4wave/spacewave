package block_gc_rpc_test

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_gc_rpc "github.com/s4wave/spacewave/db/block/gc/rpc"
	block_gc_rpc_client "github.com/s4wave/spacewave/db/block/gc/rpc/client"
	block_gc_rpc_server "github.com/s4wave/spacewave/db/block/gc/rpc/server"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// newTestRPCRefGraph creates a real RefGraph, wires it through SRPC, and
// returns a client-side RefGraphOps that talks over the pipe.
func newTestRPCRefGraph(t *testing.T) block_gc.RefGraphOps {
	t.Helper()
	ctx := context.Background()

	store := store_kvtx_inmem.NewStore()
	rg, err := block_gc.NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	mux := srpc.NewMux()
	if err := block_gc_rpc.SRPCRegisterRefGraph(mux, block_gc_rpc_server.NewRefGraph(rg)); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	openStream := srpc.NewServerPipe(server)
	client := srpc.NewClient(openStream)
	return block_gc_rpc_client.NewRefGraph(block_gc_rpc.NewSRPCRefGraphClient(client))
}

func testBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ht := hash.HashType_HashType_BLAKE3
	h, err := hash.Sum(ht, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}

// TestRPCRefGraph tests the RefGraph RPC client/server end to end.
func TestRPCRefGraph(t *testing.T) {
	ctx := context.Background()
	rg := newTestRPCRefGraph(t)

	// AddRef + GetOutgoingRefs
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	sorted := slices.Clone(refs)
	slices.Sort(sorted)
	if len(sorted) != 2 || sorted[0] != "b" || sorted[1] != "c" {
		t.Fatalf("expected [b c], got %v", sorted)
	}

	// GetIncomingRefs
	if err := rg.AddRef(ctx, "x", "d"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "y", "d"); err != nil {
		t.Fatal(err)
	}
	sources, err := rg.GetIncomingRefs(ctx, "d")
	if err != nil {
		t.Fatal(err)
	}
	sorted = slices.Clone(sources)
	slices.Sort(sorted)
	if len(sorted) != 2 || sorted[0] != "x" || sorted[1] != "y" {
		t.Fatalf("expected [x y], got %v", sorted)
	}

	// HasIncomingRefs
	has, err := rg.HasIncomingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected b to have incoming refs")
	}
	has, err = rg.HasIncomingRefs(ctx, "z-nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected z-nonexistent to have no incoming refs")
	}

	// RemoveRef
	if err := rg.RemoveRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	refs, err = rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "c" {
		t.Fatalf("expected [c], got %v", refs)
	}

	// RemoveNodeRefs
	if err := rg.AddRef(ctx, "n", "p"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "n", "q"); err != nil {
		t.Fatal(err)
	}
	targets, err := rg.RemoveNodeRefs(ctx, "n", false)
	if err != nil {
		t.Fatal(err)
	}
	sorted = slices.Clone(targets)
	slices.Sort(sorted)
	if len(sorted) != 2 || sorted[0] != "p" || sorted[1] != "q" {
		t.Fatalf("expected [p q], got %v", sorted)
	}
	refs, err = rg.GetOutgoingRefs(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no outgoing refs after RemoveNodeRefs, got %v", refs)
	}

	// GetUnreferencedNodes
	if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, "orphan1"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, "orphan2"); err != nil {
		t.Fatal(err)
	}
	nodes, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sorted = slices.Clone(nodes)
	slices.Sort(sorted)
	if len(sorted) != 2 || sorted[0] != "orphan1" || sorted[1] != "orphan2" {
		t.Fatalf("expected [orphan1 orphan2], got %v", sorted)
	}

	// AddBlockRef + AddObjectRoot + RemoveObjectRoot
	src := testBlockRef(t, "source")
	tgt := testBlockRef(t, "target")
	if err := rg.AddBlockRef(ctx, src, tgt); err != nil {
		t.Fatal(err)
	}
	refs, err = rg.GetOutgoingRefs(ctx, block_gc.BlockIRI(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != block_gc.BlockIRI(tgt) {
		t.Fatalf("expected [%s], got %v", block_gc.BlockIRI(tgt), refs)
	}

	objRef := testBlockRef(t, "obj-block")
	if err := rg.AddObjectRoot(ctx, "myobj", objRef); err != nil {
		t.Fatal(err)
	}
	refs, err = rg.GetOutgoingRefs(ctx, block_gc.ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != block_gc.BlockIRI(objRef) {
		t.Fatalf("expected [%s], got %v", block_gc.BlockIRI(objRef), refs)
	}

	if err := rg.RemoveObjectRoot(ctx, "myobj", objRef); err != nil {
		t.Fatal(err)
	}
	refs, err = rg.GetOutgoingRefs(ctx, block_gc.ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs after RemoveObjectRoot, got %v", refs)
	}
}
