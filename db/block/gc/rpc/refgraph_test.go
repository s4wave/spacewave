package block_gc_rpc_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
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

type rpcRefGraphTestbed struct {
	client   block_gc.RefGraphOps
	observed *observedRefGraph
}

type observedRefGraph struct {
	block_gc.RefGraphOps

	mu              sync.Mutex
	applyCalls      int
	individualCalls int
	adds            []block_gc.RefEdge
	removes         []block_gc.RefEdge
}

func (r *observedRefGraph) AddRef(ctx context.Context, subject, object string) error {
	r.mu.Lock()
	r.individualCalls++
	r.mu.Unlock()
	return r.RefGraphOps.AddRef(ctx, subject, object)
}

func (r *observedRefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	r.mu.Lock()
	r.individualCalls++
	r.mu.Unlock()
	return r.RefGraphOps.RemoveRef(ctx, subject, object)
}

func (r *observedRefGraph) ApplyRefBatch(
	ctx context.Context,
	adds, removes []block_gc.RefEdge,
) error {
	// Record the batch before forwarding it to the wrapped graph.
	r.mu.Lock()
	r.applyCalls++
	r.adds = slices.Clone(adds)
	r.removes = slices.Clone(removes)
	r.mu.Unlock()
	return r.RefGraphOps.ApplyRefBatch(ctx, adds, removes)
}

func (r *observedRefGraph) reset() {
	// Clear the recorded calls and edges between assertions.
	r.mu.Lock()
	r.applyCalls = 0
	r.individualCalls = 0
	r.adds = nil
	r.removes = nil
	r.mu.Unlock()
}

func (r *observedRefGraph) snapshot() (int, int, []block_gc.RefEdge, []block_gc.RefEdge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.applyCalls, r.individualCalls, slices.Clone(r.adds), slices.Clone(r.removes)
}

// newTestRPCRefGraph creates a real RefGraph, wires it through SRPC, and
// returns a client-side RefGraphOps that talks over the pipe.
func newTestRPCRefGraph(t *testing.T) block_gc.RefGraphOps {
	t.Helper()
	return newRPCRefGraphTestbed(t, nil).client
}

func newRPCRefGraphTestbed(
	t *testing.T,
	wrapOwner func(block_gc.RefGraphOps) block_gc.RefGraphOps,
) *rpcRefGraphTestbed {
	// Prepare a fresh real graph and its SRPC testbed.
	t.Helper()
	ctx := context.Background()

	// Open a graph over an in-memory store and release it with the test.
	store := store_kvtx_inmem.NewStore()
	rg, err := block_gc.NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })

	// Optionally wrap the graph owner before recording server calls.
	var owner block_gc.RefGraphOps = rg
	if wrapOwner != nil {
		owner = wrapOwner(owner)
	}
	observed := &observedRefGraph{RefGraphOps: owner}
	mux := srpc.NewMux()
	if err := block_gc_rpc.SRPCRegisterRefGraph(mux, block_gc_rpc_server.NewRefGraph(observed)); err != nil {
		t.Fatal(err)
	}

	// Connect the registered service to an in-process RPC client.
	server := srpc.NewServer(mux)
	openStream := srpc.NewServerPipe(server)
	client := srpc.NewClient(openStream)
	return &rpcRefGraphTestbed{
		client:   block_gc_rpc_client.NewRefGraph(block_gc_rpc.NewSRPCRefGraphClient(client)),
		observed: observed,
	}
}

func testBlockRef(t *testing.T, data string) *block.BlockRef {
	// Hash test data into a block reference.
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
	// Exercise graph operations through the in-process SRPC client.
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

	// Remove every edge owned by the node and capture its targets.
	targets, err := rg.RemoveNodeRefs(ctx, "n", false)
	if err != nil {
		t.Fatal(err)
	}
	sorted = slices.Clone(targets)
	slices.Sort(sorted)
	if len(sorted) != 2 || sorted[0] != "p" || sorted[1] != "q" {
		t.Fatalf("expected [p q], got %v", sorted)
	}

	// Verify node removal cleared its outgoing edges.
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

	// Add a block edge and verify its encoded graph relationship.
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

	// Remove the object's root edge and verify the relationship is gone.
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

func assertRPCOwnerTransition(
	t *testing.T,
	testbed *rpcRefGraphTestbed,
	adds, removes []block_gc.RefEdge,
) {
	// Assert that the server applies the complete batch through its owner.
	t.Helper()
	applyCalls, individualCalls, gotAdds, gotRemoves := testbed.observed.snapshot()
	if applyCalls != 1 {
		t.Fatalf("server owner ApplyRefBatch calls = %d, want 1", applyCalls)
	}
	if individualCalls != 0 {
		t.Fatalf("server owner individual edge calls = %d, want 0", individualCalls)
	}
	if !slices.Equal(gotAdds, adds) {
		t.Fatalf("server owner adds = %v, want %v", gotAdds, adds)
	}
	if !slices.Equal(gotRemoves, removes) {
		t.Fatalf("server owner removes = %v, want %v", gotRemoves, removes)
	}
}

func sortedRefs(refs []string) []string {
	out := slices.Clone(refs)
	slices.Sort(out)
	return out
}

func TestRPCApplyRefBatchMissingRemovalPreservesGraph(t *testing.T) {
	// Verify missing batch removals preserve all graph indexes.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	rg := testbed.client

	// Seed existing owner and orphan edges before capturing the graph state.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, "staged"); err != nil {
		t.Fatal(err)
	}

	// Capture the graph indexes before the missing removal request.
	beforeOutgoing, err := rg.GetOutgoingRefs(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	beforeIncoming, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	beforeUnreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	removes := []block_gc.RefEdge{
		{Subject: "missing-owner", Object: "target"},
		{Subject: "missing-owner", Object: "never-seen"},
	}

	// Apply missing removals and confirm the server receives the batch.
	testbed.observed.reset()
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		t.Fatal(err)
	}
	assertRPCOwnerTransition(t, testbed, nil, removes)

	// Verify the missing removal leaves forward edges unchanged.
	afterOutgoing, err := rg.GetOutgoingRefs(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedRefs(afterOutgoing), sortedRefs(beforeOutgoing)) {
		t.Fatalf("missing removal changed forward refs: got %v, want %v", afterOutgoing, beforeOutgoing)
	}

	// Verify the missing removal leaves reverse edges unchanged.
	afterIncoming, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedRefs(afterIncoming), sortedRefs(beforeIncoming)) {
		t.Fatalf("missing removal changed reverse refs: got %v, want %v", afterIncoming, beforeIncoming)
	}

	// Verify the missing removal leaves orphan marks unchanged.
	afterUnreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedRefs(afterUnreferenced), sortedRefs(beforeUnreferenced)) {
		t.Fatalf(
			"missing removal changed orphan marks: got %v, want %v",
			afterUnreferenced,
			beforeUnreferenced,
		)
	}
}

func TestRPCApplyRefBatchExistingRemovalUpdatesForwardEdge(t *testing.T) {
	// Verify removing an existing edge updates the owner's forward index.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	rg := testbed.client

	// Seed the edge to remove and an edge that must remain.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "owner", "retained"); err != nil {
		t.Fatal(err)
	}
	removes := []block_gc.RefEdge{{Subject: "owner", Object: "target"}}

	// Apply the removal as one server-owned batch.
	testbed.observed.reset()
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		t.Fatal(err)
	}
	assertRPCOwnerTransition(t, testbed, nil, removes)

	// Confirm only the retained edge remains.
	outgoing, err := rg.GetOutgoingRefs(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(outgoing, []string{"retained"}) {
		t.Fatalf("existing removal forward refs = %v, want [retained]", outgoing)
	}
}

func TestRPCApplyRefBatchCollidingAddAndRemoveOrphans(t *testing.T) {
	// Verify an edge present in both sets is removed and marked orphaned.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	rg := testbed.client
	edge := block_gc.RefEdge{Subject: "owner", Object: "target"}
	adds := []block_gc.RefEdge{edge}
	removes := []block_gc.RefEdge{edge}

	// Submit the colliding add and remove together.
	testbed.observed.reset()
	if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
		t.Fatal(err)
	}
	assertRPCOwnerTransition(t, testbed, adds, removes)

	// Confirm the edge is absent and its target is orphaned.
	outgoing, err := rg.GetOutgoingRefs(ctx, edge.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(outgoing, edge.Object) {
		t.Fatalf("colliding edge remains after add-before-remove: %v", outgoing)
	}
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unreferenced, edge.Object) {
		t.Fatalf("colliding transition orphan marks = %v, want target", unreferenced)
	}
}

// TestRPCApplyRefBatchSharedOwnerRemovalKeepsObjectReachable verifies a shared object remains reachable after one owner is removed.
func TestRPCApplyRefBatchSharedOwnerRemovalKeepsObjectReachable(t *testing.T) {
	// Verify removing one of two owners keeps the shared object reachable.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	rg := testbed.client

	// Seed two owners for the same object.
	for _, owner := range []string{"owner-a", "owner-b"} {
		if err := rg.AddRef(ctx, owner, "shared"); err != nil {
			t.Fatal(err)
		}
	}
	removes := []block_gc.RefEdge{{Subject: "owner-a", Object: "shared"}}

	// Remove one owner through a single batch.
	testbed.observed.reset()
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		t.Fatal(err)
	}
	assertRPCOwnerTransition(t, testbed, nil, removes)

	// Confirm the remaining owner still reaches the shared object.
	incoming, err := rg.GetIncomingRefs(ctx, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(incoming, []string{"owner-b"}) {
		t.Fatalf("shared-owner reverse refs = %v, want [owner-b]", incoming)
	}

	// Confirm the shared object is not marked unreferenced.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(unreferenced, "shared") {
		t.Fatalf("shared-owner removal orphaned shared object: %v", unreferenced)
	}
}

// TestRPCApplyRefBatchLastOwnerRemovalOrphansObject verifies the final owner removal marks its target as orphaned.
func TestRPCApplyRefBatchLastOwnerRemovalOrphansObject(t *testing.T) {
	// Verify removing the last owner marks the target as orphaned.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	rg := testbed.client

	// Seed the target with its only owner.
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	removes := []block_gc.RefEdge{{Subject: "owner", Object: "target"}}

	// Remove the last owner through a single batch.
	testbed.observed.reset()
	if err := rg.ApplyRefBatch(ctx, nil, removes); err != nil {
		t.Fatal(err)
	}
	assertRPCOwnerTransition(t, testbed, nil, removes)

	// Confirm the target is now unreferenced.
	incoming, err := rg.GetIncomingRefs(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(incoming, []string{block_gc.NodeUnreferenced}) {
		t.Fatalf("last-owner reverse refs = %v, want [unreferenced]", incoming)
	}

	// Confirm the target appears in the orphan index.
	unreferenced, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(unreferenced, "target") {
		t.Fatalf("last-owner orphan marks = %v, want target", unreferenced)
	}
}

var errInjectedRPCBatch = errors.New("injected RPC owner batch failure")

type remainderRefGraph struct {
	block_gc.RefGraphOps
}

func (r *remainderRefGraph) ApplyRefBatch(
	ctx context.Context,
	adds, removes []block_gc.RefEdge,
) error {
	if err := r.RefGraphOps.ApplyRefBatch(ctx, adds[:1], nil); err != nil {
		return err
	}
	return &rpcBatchError{
		err:     errInjectedRPCBatch,
		adds:    slices.Clone(adds[1:]),
		removes: slices.Clone(removes),
	}
}

type rpcBatchError struct {
	err     error
	adds    []block_gc.RefEdge
	removes []block_gc.RefEdge
}

func (e *rpcBatchError) Error() string {
	return e.err.Error()
}

func (e *rpcBatchError) Unwrap() error {
	return e.err
}

func (e *rpcBatchError) RefBatchRemainder() ([]block_gc.RefEdge, []block_gc.RefEdge) {
	return e.adds, e.removes
}

// TestRPCApplyRefBatchPreservesRemainder verifies a failed remote batch reports its uncommitted remainder.
func TestRPCApplyRefBatchPreservesRemainder(t *testing.T) {
	// Verify a failed remote batch reports the uncommitted remainder.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, func(rg block_gc.RefGraphOps) block_gc.RefGraphOps {
		return &remainderRefGraph{RefGraphOps: rg}
	})
	adds := []block_gc.RefEdge{
		{Subject: "owner", Object: "committed"},
		{Subject: "owner", Object: "pending-a"},
		{Subject: "owner", Object: "pending-b"},
	}
	removes := []block_gc.RefEdge{{Subject: "owner", Object: "pending-remove"}}

	// Reset observations and send a batch whose owner commits only a prefix.
	testbed.observed.reset()

	// Check the injected error and the server-side batch call.
	err := testbed.client.ApplyRefBatch(ctx, adds, removes)
	if err == nil || err.Error() != errInjectedRPCBatch.Error() {
		t.Fatalf("RPC batch error = %v, want %q", err, errInjectedRPCBatch)
	}

	// Read the uncommitted suffix and verify its exact contents.
	assertRPCOwnerTransition(t, testbed, adds, removes)
	remainderAdds, remainderRemoves, ok := block_gc.RefBatchRemainder(err)
	if !ok {
		t.Fatal("RPC batch error did not carry a remainder")
	}
	if !slices.Equal(remainderAdds, adds[1:]) {
		t.Fatalf("RPC remainder adds = %v, want %v", remainderAdds, adds[1:])
	}
	if !slices.Equal(remainderRemoves, removes) {
		t.Fatalf("RPC remainder removes = %v, want %v", remainderRemoves, removes)
	}

	// Confirm the committed prefix is visible through the client.
	committed, getErr := testbed.client.GetOutgoingRefs(ctx, "owner")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if !slices.Equal(committed, []string{"committed"}) {
		t.Fatalf("RPC committed prefix = %v, want [committed]", committed)
	}
}

var (
	errInjectedRPCAddRef        = errors.New("injected RPC AddRef failure")
	errInjectedRPCRemoveRef     = errors.New("injected RPC RemoveRef failure")
	errInjectedRPCRemoveNodeRef = errors.New("injected RPC RemoveNodeRefs failure")
)

type rpcErrorRefGraph struct {
	block_gc.RefGraphOps
	addRefErr         error
	removeRefErr      error
	removeNodeRefsErr error
}

func (r *rpcErrorRefGraph) AddRef(context.Context, string, string) error {
	return r.addRefErr
}

func (r *rpcErrorRefGraph) RemoveRef(context.Context, string, string) error {
	return r.removeRefErr
}

func (r *rpcErrorRefGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, r.removeNodeRefsErr
}

func TestRPCAddRefSurfacesServerError(t *testing.T) {
	testbed := newRPCRefGraphTestbed(t, func(rg block_gc.RefGraphOps) block_gc.RefGraphOps {
		return &rpcErrorRefGraph{
			RefGraphOps: rg,
			addRefErr:   errInjectedRPCAddRef,
		}
	})

	err := testbed.client.AddRef(context.Background(), "subject", "object")
	if err == nil || err.Error() != errInjectedRPCAddRef.Error() {
		t.Fatalf("RPC AddRef error = %v, want %q", err, errInjectedRPCAddRef)
	}
}

func TestRPCRemoveRefSurfacesServerError(t *testing.T) {
	testbed := newRPCRefGraphTestbed(t, func(rg block_gc.RefGraphOps) block_gc.RefGraphOps {
		return &rpcErrorRefGraph{
			RefGraphOps:  rg,
			removeRefErr: errInjectedRPCRemoveRef,
		}
	})

	err := testbed.client.RemoveRef(context.Background(), "subject", "object")
	if err == nil || err.Error() != errInjectedRPCRemoveRef.Error() {
		t.Fatalf("RPC RemoveRef error = %v, want %q", err, errInjectedRPCRemoveRef)
	}
}

func TestRPCRemoveNodeRefsSurfacesServerError(t *testing.T) {
	testbed := newRPCRefGraphTestbed(t, func(rg block_gc.RefGraphOps) block_gc.RefGraphOps {
		return &rpcErrorRefGraph{
			RefGraphOps:       rg,
			removeNodeRefsErr: errInjectedRPCRemoveNodeRef,
		}
	})

	targets, err := testbed.client.RemoveNodeRefs(context.Background(), "node", true)
	if err == nil || err.Error() != errInjectedRPCRemoveNodeRef.Error() {
		t.Fatalf("RPC RemoveNodeRefs error = %v, want %q", err, errInjectedRPCRemoveNodeRef)
	}
	if targets != nil {
		t.Fatalf("RPC RemoveNodeRefs targets = %v, want nil", targets)
	}
}

// TestRPCApplyRefBatchSplitsLargeBatch sends a batch larger than one RPC
// message and expects every edge to commit across several requests.
func TestRPCApplyRefBatchSplitsLargeBatch(t *testing.T) {
	// Build adds that encode to more than one RPC message.
	ctx := context.Background()
	testbed := newRPCRefGraphTestbed(t, nil)
	pad := strings.Repeat("x", 1024)
	adds := make([]block_gc.RefEdge, 12_000)
	for i := range adds {
		adds[i] = block_gc.RefEdge{Subject: "owner", Object: fmt.Sprintf("object-%d-%s", i, pad)}
	}
	removes := []block_gc.RefEdge{adds[0]}

	// Apply the batch and expect it to arrive in several requests.
	testbed.observed.reset()
	if err := testbed.client.ApplyRefBatch(ctx, adds, removes); err != nil {
		t.Fatal(err)
	}
	if applyCalls, _, _, _ := testbed.observed.snapshot(); applyCalls < 3 {
		t.Fatalf("server ApplyRefBatch calls = %d, want at least 3", applyCalls)
	}

	// The removal commits after the additions on either side of it.
	for _, edge := range []block_gc.RefEdge{adds[0], adds[1], adds[len(adds)-1]} {
		owned, err := testbed.client.HasIncomingRefs(ctx, edge.Object)
		if err != nil {
			t.Fatal(err)
		}
		if want := edge != adds[0]; owned != want {
			t.Fatalf("%.16s owned = %v, want %v", edge.Object, owned, want)
		}
	}
}
