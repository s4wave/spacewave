package resource_server

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// ownershipTestClient seeds a live resource generation for tree-lifetime checks.
func ownershipTestClient(t *testing.T) (*ResourceServer, *RemoteResourceClient) {
	// Build a server and one registered client with a seeded root resource.
	t.Helper()
	server := NewResourceServer(nil)
	client := &RemoteResourceClient{
		server:         server,
		clientID:       1,
		rootResourceID: 1,
		ctx:            t.Context(),
		resources:      make(map[uint32]*trackedResource),
		children:       make(map[uint32]map[uint32]struct{}),
		tombstones:     make(map[uint32]struct{}),
	}
	server.clients[1] = client
	server.resourceIDCtr = 1
	client.resources[1] = &trackedResource{
		mux:           srpc.NewMux(),
		ownerClientID: 1,
	}
	return server, client
}

// TestPendingChildrenReleasePostorderAndRootRetention checks pending tree cleanup.
func TestPendingChildrenReleasePostorderAndRootRetention(t *testing.T) {
	// Add a pending child and grandchild invocation resource with release recorders.
	_, client := ownershipTestClient(t)
	var order []uint32
	releaseChild := func() { order = append(order, 2) }
	child, err := newResourceRPCContext(client, 1).AddResource(srpc.NewMux(), releaseChild)
	if err != nil || child != 2 {
		t.Fatalf("child = %d/%v", child, err)
	}

	// Register a pending grandchild beneath the child.
	releaseGrandchild := func() { order = append(order, 3) }
	grandchild, err := newResourceRPCContext(client, child).AddResource(srpc.NewMux(), releaseGrandchild)
	if err != nil || grandchild != 3 {
		t.Fatalf("grandchild = %d/%v", grandchild, err)
	}

	// Release the root and verify descendants drain postorder while it stays.
	if _, err := client.releaseClientControl(1); err != nil {
		t.Fatalf("root release: %v", err)
	}
	if client.resources[1] == nil {
		t.Fatal("root was released")
	}
	if client.resources[2] != nil || client.resources[3] != nil {
		t.Fatal("pending descendants survived root release")
	}
	if len(order) != 2 || order[0] != 3 || order[1] != 2 {
		t.Fatalf("release order = %v", order)
	}
	if len(client.txQueue) != 2 {
		t.Fatalf("notifications = %d, want 2", len(client.txQueue))
	}
}

// TestGenerationCleanupReleasesAdoptedTreeChildFirst checks disconnect cleanup.
func TestGenerationCleanupReleasesAdoptedTreeChildFirst(t *testing.T) {
	// Add and adopt a pending child invocation resource.
	_, client := ownershipTestClient(t)
	var order []uint32
	releaseChild := func() { order = append(order, 2) }
	child, err := newResourceRPCContext(client, 1).AddResource(srpc.NewMux(), releaseChild)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(child) {
		t.Fatal("adopt rejected")
	}

	// Add and adopt a grandchild under the adopted child.
	releaseGrandchild := func() { order = append(order, 3) }
	grandchild, err := newResourceRPCContext(client, child).AddResource(srpc.NewMux(), releaseGrandchild)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(grandchild) {
		t.Fatal("grandchild adopt rejected")
	}

	// Run generation cleanup and verify the child releases before the root.
	var releaseFns []func()
	client.releaseAllChildrenLocked(1, &releaseFns)
	for _, releaseFn := range releaseFns {
		releaseFn()
	}
	if len(order) != 2 || order[0] != 3 || order[1] != 2 {
		t.Fatalf("cleanup order = %v", order)
	}
	if len(client.resources) != 0 {
		t.Fatalf("resources after cleanup = %d", len(client.resources))
	}
}

// TestAdoptedChildSurvivesParentReleaseAndTombstoneNotifies checks independent adoption.
func TestAdoptedChildSurvivesParentReleaseAndTombstoneNotifies(t *testing.T) {
	// Add and adopt a pending child invocation resource.
	_, client := ownershipTestClient(t)
	child, err := newResourceRPCContext(client, 1).AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(child) {
		t.Fatal("adopt rejected")
	}

	// Release the root and verify the adopted child survives it.
	if _, err := client.releaseClientControl(1); err != nil {
		t.Fatal(err)
	}
	if client.resources[child] == nil {
		t.Fatal("adopted child was released with parent")
	}

	// Release the child server-side and verify the tombstone notification.
	if !client.ReleaseResource(child) {
		t.Fatal("server release rejected")
	}
	if len(client.txQueue) != 1 {
		t.Fatalf("server release notifications = %d, want 1", len(client.txQueue))
	}

	// Re-adopt the tombstoned child and verify the stale notification.
	client.txQueue = nil
	if !client.adoptResource(child) {
		t.Fatal("stale adopt did not succeed")
	}
	if len(client.txQueue) != 1 {
		t.Fatalf("stale adopt notifications = %d, want 1", len(client.txQueue))
	}
}

// TestChildAfterParentReleaseRejected checks registration cannot revive a released tree.
func TestChildAfterParentReleaseRejected(t *testing.T) {
	// Add a pending child, then release it as a parent whose invocation runs on.
	_, client := ownershipTestClient(t)
	parent, err := newResourceRPCContext(client, 1).AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !client.ReleaseResource(parent) {
		t.Fatal("parent release rejected")
	}

	// Register the late child and verify it is rejected without allocation.
	var released bool
	releaseChild := func() { released = true }
	child, err := newResourceRPCContext(client, parent).AddResource(srpc.NewMux(), releaseChild)
	if err != resource.ErrResourceNotFound || child != 0 {
		t.Fatalf("late child = %d/%v, want ErrResourceNotFound", child, err)
	}
	if released {
		t.Fatal("rejected registration ran the caller's release")
	}
	if len(client.resources) != 1 || len(client.children) != 0 {
		t.Fatalf("resources = %d, children = %d after rejection", len(client.resources), len(client.children))
	}
}

// TestForeignAndNeverAllocatedControlsTerminate checks unknown controls are refused.
func TestForeignAndNeverAllocatedControlsTerminate(t *testing.T) {
	_, client := ownershipTestClient(t)
	if _, err := client.releaseClientControl(999); err != resource.ErrResourceNotFound {
		t.Fatalf("unknown release error = %v", err)
	}
	if client.adoptResource(999) {
		t.Fatal("unknown adopt accepted")
	}
}

// TestUnpublishedCleanupPreservesAdoptedResource checks adoption wins cleanup.
func TestUnpublishedCleanupPreservesAdoptedResource(t *testing.T) {
	// Adopt a child while its creating handler still retains publication state.
	_, client := ownershipTestClient(t)
	invocation := newResourceRPCContext(client, 1)
	releases := 0
	id, err := invocation.AddResource(srpc.NewMux(), func() { releases++ })
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(id) {
		t.Fatal("adoption failed")
	}

	// Complete the invocation and preserve the independently adopted child.
	invocation.releaseUnpublished()
	if client.resources[id] == nil || releases != 0 {
		t.Fatal("invocation cleanup released an adopted child")
	}
	if _, err := invocation.AddResource(srpc.NewMux(), nil); err != context.Canceled {
		t.Fatalf("late registration error = %v, want context.Canceled", err)
	}

	// Release the child's own lifetime exactly once.
	client.ReleaseResource(id)
	if releases != 1 {
		t.Fatalf("release callbacks = %d, want 1", releases)
	}
}

// TestAbandonPreservesAdoptedResource checks that adoption wins route abandonment.
func TestAbandonPreservesAdoptedResource(t *testing.T) {
	// Retain one adopted child beside a pending child from the same invocation.
	_, client := ownershipTestClient(t)
	invocation := newResourceRPCContext(client, 1)
	releases := 0
	adoptedID, err := invocation.AddResource(srpc.NewMux(), func() { releases++ })
	if err != nil {
		t.Fatal(err)
	}
	pendingID, err := invocation.AddResource(srpc.NewMux(), func() { releases++ })
	if err != nil {
		t.Fatal(err)
	}
	if !client.adoptResource(adoptedID) {
		t.Fatal("adoption failed")
	}

	// Abandonment removes only the pending child and rejects later registration.
	invocation.abandon()
	invocation.releaseUnpublished()
	if client.resources[adoptedID] == nil || client.resources[pendingID] != nil || releases != 1 {
		t.Fatal("abandonment did not preserve the adopted child and release the pending child")
	}
	if _, err := invocation.AddResource(srpc.NewMux(), nil); err != context.Canceled {
		t.Fatalf("late registration error = %v, want context.Canceled", err)
	}

	// The adopted child keeps its independently controlled release lifetime.
	client.ReleaseResource(adoptedID)
	if releases != 2 {
		t.Fatalf("release callbacks = %d, want 2", releases)
	}
}
