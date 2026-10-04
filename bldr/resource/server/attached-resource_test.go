package resource_server

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// mockSRPCClient implements srpc.Client for testing.
type mockSRPCClient struct {
	// id distinguishes clients in attachment lookup assertions.
	id int
}

// ExecCall accepts unused test calls without executing a handler.
func (m *mockSRPCClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	return nil
}

// NewStream accepts unused stream calls without opening a transport.
func (m *mockSRPCClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	return nil, nil
}

// newTestClient creates a RemoteResourceClient for attached resource tests.
func newTestClient(t *testing.T) (*RemoteResourceClient, context.CancelFunc) {
	// Build a client with a seeded root resource and register it with the server.
	t.Helper()
	s := NewResourceServer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	client := &RemoteResourceClient{
		server:         s,
		clientID:       1,
		rootResourceID: 1,
		ctx:            ctx,
		resources:      make(map[uint32]*trackedResource),
		children:       make(map[uint32]map[uint32]struct{}),
		tombstones:     make(map[uint32]struct{}),
	}
	client.resources[1] = &trackedResource{
		mux:           srpc.NewMux(),
		ownerClientID: 1,
	}
	s.resourceIDCtr = 1
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.clients[1] = client
	})
	return client, cancel
}

// resourceServerWaitCh observes the next server resource-state transition.
func resourceServerWaitCh(s *ResourceServer) <-chan struct{} {
	locked := s.bcast.Lock()
	defer locked.Unlock()
	return locked.WaitCh()
}

// assertWaitChClosed requires a previously captured state transition to fire.
func assertWaitChClosed(t *testing.T, waitCh <-chan struct{}) {
	t.Helper()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("resource client queue wait channel was not closed")
	}
}

// TestAddAttachedResource_Success checks published resource lookup.
func TestAddAttachedResource_Success(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a mock resource and require success.
	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(42, "test-resource", func() {}, mc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fetch the resource and compare it to the added mock.
	got, err := client.GetAttachedResource(42)
	if err != nil {
		t.Fatalf("unexpected error getting resource: %v", err)
	}
	if got != mc {
		t.Fatal("returned client does not match the one that was added")
	}
}

// TestAddAttachedResource_InitializesMap checks the first attachment allocates its map.
func TestAddAttachedResource_InitializesMap(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a resource and require the attached map to initialize.
	if len(client.attachedResources) != 0 {
		t.Fatal("attachedResources should be empty before first AddAttachedResource")
	}
	mc := &mockSRPCClient{id: 1}
	if err := client.AddAttachedResource(1, "label", func() {}, mc, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.attachedResources) == 0 {
		t.Fatal("attachedResources should be initialized after AddAttachedResource")
	}
}

// TestAddAttachedResource_ReleasedClient checks retired generations reject publication.
func TestAddAttachedResource_ReleasedClient(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Mark the client as released.
	client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		client.released = true
	})

	// Require the released-client rejection.
	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(1, "label", func() {}, mc, nil)
	if err != resource.ErrClientReleased {
		t.Fatalf("got error %v, want %v", err, resource.ErrClientReleased)
	}
}

// TestRemoveAttachedResource_Success checks attachment removal runs both callbacks.
func TestRemoveAttachedResource_Success(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a tracked resource and require success.
	canceled := false
	released := false
	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(
		10,
		"label",
		func() { canceled = true },
		mc,
		func() { released = true },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remove the attached resource.
	client.RemoveAttachedResource(10)

	// Assert both callbacks ran.
	if !canceled {
		t.Fatal("cancel function was not called")
	}
	if !released {
		t.Fatal("release function was not called")
	}

	// Require the resource lookup to fail.
	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

// TestReleaseResourceRemovesAttachedResource checks the shared release interface.
func TestReleaseResourceRemovesAttachedResource(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a tracked resource and require success.
	canceled := false
	released := false
	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(
		10,
		"label",
		func() { canceled = true },
		mc,
		func() { released = true },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Release the resource and require both callbacks.
	if !client.ReleaseResource(10) {
		t.Fatal("expected ReleaseResource to release attached resource")
	}
	if !released {
		t.Fatal("release function was not called")
	}
	if !canceled {
		t.Fatal("cancel function was not called")
	}

	// Require the resource lookup to fail.
	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

// TestAddResourceValueWakesCountWaiters checks registration publishes a count change.
func TestAddResourceValueWakesCountWaiters(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Add a resource value and require the resource-count waiter's wake.
	waitCh := resourceServerWaitCh(client.server)
	if _, err := client.AddResourceValue(srpc.NewMux(), &mockSRPCClient{id: 2}, nil); err != nil {
		t.Fatalf("AddResourceValue: %v", err)
	}
	assertWaitChClosed(t, waitCh)
}

// TestReleaseResourceWakesClientQueue checks owned-resource release wakes waiters.
func TestReleaseResourceWakesClientQueue(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Add a server-owned resource and capture the server wait channel.
	id, err := client.AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	waitCh := resourceServerWaitCh(client.server)

	// Release the resource and require the client queue wake.
	if !client.ReleaseResource(id) {
		t.Fatal("expected ReleaseResource to release server-owned resource")
	}
	assertWaitChClosed(t, waitCh)
}

// TestReleaseAttachedResourceWakesClientQueue checks release notification delivery.
func TestReleaseAttachedResourceWakesClientQueue(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a mock resource and capture the server wait channel.
	mc := &mockSRPCClient{id: 3}
	err := client.AddAttachedResource(12, "attached", func() {}, mc, func() {})
	if err != nil {
		t.Fatalf("AddAttachedResource: %v", err)
	}
	waitCh := resourceServerWaitCh(client.server)

	// Release the attached resource and require the queued release notification.
	if !client.ReleaseResource(12) {
		t.Fatal("expected ReleaseResource to release attached resource")
	}
	assertWaitChClosed(t, waitCh)
	if len(client.txQueue) != 1 ||
		client.txQueue[0].GetResourceReleased().GetResourceId() != 12 {
		t.Fatalf("attached release notifications = %#v", client.txQueue)
	}
}

// TestRemoveAttachedResource_NotFound checks removal is idempotent for unknown IDs.
func TestRemoveAttachedResource_NotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	// Should not panic when removing a non-existent resource.
	client.RemoveAttachedResource(999)
}

// TestRemoveAttachedResourceDoesNotAffectOthers checks independent attachment lifetimes.
func TestRemoveAttachedResourceDoesNotAffectOthers(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Track two resources with separate cancel callbacks and clients.
	canceled1 := false
	canceled2 := false
	mc1 := &mockSRPCClient{id: 1}
	mc2 := &mockSRPCClient{id: 2}

	// Attach both mock resources and require success.
	err := client.AddAttachedResource(10, "res-1", func() { canceled1 = true }, mc1, nil)
	if err != nil {
		t.Fatalf("unexpected error adding resource 1: %v", err)
	}
	err = client.AddAttachedResource(20, "res-2", func() { canceled2 = true }, mc2, nil)
	if err != nil {
		t.Fatalf("unexpected error adding resource 2: %v", err)
	}

	// Remove resource 1 only.
	client.RemoveAttachedResource(10)

	// Assert only resource 1 was canceled.
	if !canceled1 {
		t.Fatal("cancel for resource 1 was not called")
	}
	if canceled2 {
		t.Fatal("cancel for resource 2 was called unexpectedly")
	}

	// Resource 1 should be gone.
	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("resource 1: got error %v, want %v", err, resource.ErrResourceNotFound)
	}

	// Resource 2 should still be accessible.
	got, err := client.GetAttachedResource(20)
	if err != nil {
		t.Fatalf("resource 2: unexpected error: %v", err)
	}
	if got != mc2 {
		t.Fatal("resource 2: returned client does not match")
	}
}

// TestGetAttachedResource_Success checks attachment identity survives lookup.
func TestGetAttachedResource_Success(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach a mock resource and require success.
	mc := &mockSRPCClient{id: 42}
	err := client.AddAttachedResource(5, "my-resource", func() {}, mc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fetch the resource and assert the mock identity.
	got, err := client.GetAttachedResource(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mock, ok := got.(*mockSRPCClient)
	if !ok {
		t.Fatal("returned client has wrong type")
	}
	if mock.id != 42 {
		t.Fatalf("got id %d, want 42", mock.id)
	}
}

// TestGetAttachedResource_NotFound checks lookup reports an absent resource.
func TestGetAttachedResource_NotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	_, err := client.GetAttachedResource(999)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

// TestAddResourceValueAndGetResourceValue checks retained in-process resource values.
func TestAddResourceValueAndGetResourceValue(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Add a mock resource value and require success.
	want := &mockSRPCClient{id: 99}
	id, err := client.AddResourceValue(srpc.NewMux(), want, nil)
	if err != nil {
		t.Fatalf("unexpected error adding resource: %v", err)
	}

	// Fetch the value and compare it to the added mock.
	got, err := client.GetResourceValue(id)
	if err != nil {
		t.Fatalf("unexpected error getting resource value: %v", err)
	}
	if got != want {
		t.Fatal("returned resource value does not match")
	}
}

// TestGetResourceValueNotFound checks lookup reports an absent value.
func TestGetResourceValueNotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	_, err := client.GetResourceValue(404)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

// TestReleaseAllAttachedResources_CancelsAll checks disconnect retires every attachment.
func TestReleaseAllAttachedResources_CancelsAll(t *testing.T) {
	// Start a test client with a seeded root resource.
	client, cancel := newTestClient(t)
	defer cancel()

	// Attach three resources with tracked cancel callbacks.
	canceled := make(map[uint32]bool)
	for i := uint32(1); i <= 3; i++ {
		id := i
		mc := &mockSRPCClient{id: int(id)}
		err := client.AddAttachedResource(id, "res", func() { canceled[id] = true }, mc, nil)
		if err != nil {
			t.Fatalf("unexpected error adding resource %d: %v", id, err)
		}
	}

	// Release all attached resources.
	client.releaseAllAttachedResources()

	// Assert every cancel callback ran.
	for i := uint32(1); i <= 3; i++ {
		if !canceled[i] {
			t.Fatalf("cancel for resource %d was not called", i)
		}
	}

	// All resources should be removed.
	for i := uint32(1); i <= 3; i++ {
		_, err := client.GetAttachedResource(i)
		if err != resource.ErrResourceNotFound {
			t.Fatalf("resource %d: got error %v, want %v", i, err, resource.ErrResourceNotFound)
		}
	}
}
