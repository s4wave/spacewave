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
	id int
}

func (m *mockSRPCClient) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	return nil
}

func (m *mockSRPCClient) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	return nil, nil
}

// newTestClient creates a RemoteResourceClient for attached resource tests.
func newTestClient(t *testing.T) (*RemoteResourceClient, context.CancelFunc) {
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
		createdAt:     s.now(),
	}
	s.resourceIDCtr = 1
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.clients[1] = client
	})
	return client, cancel
}

func resourceServerWaitCh(s *ResourceServer) <-chan struct{} {
	locked := s.bcast.Lock()
	defer locked.Unlock()
	return locked.WaitCh()
}

func assertWaitChClosed(t *testing.T, waitCh <-chan struct{}) {
	t.Helper()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("resource client queue wait channel was not closed")
	}
}

func TestAddAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(42, "test-resource", func() {}, mc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := client.GetAttachedResource(42)
	if err != nil {
		t.Fatalf("unexpected error getting resource: %v", err)
	}
	if got != mc {
		t.Fatal("returned client does not match the one that was added")
	}
}

func TestAddAttachedResource_InitializesMap(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

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

func TestAddAttachedResource_ReleasedClient(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	// Mark the client as released.
	client.server.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		client.released = true
	})

	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(1, "label", func() {}, mc, nil)
	if err != resource.ErrClientReleased {
		t.Fatalf("got error %v, want %v", err, resource.ErrClientReleased)
	}
}

func TestRemoveAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

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

	client.RemoveAttachedResource(10)

	if !canceled {
		t.Fatal("cancel function was not called")
	}
	if !released {
		t.Fatal("release function was not called")
	}

	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

func TestReleaseResourceRemovesAttachedResource(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

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

	if !client.ReleaseResource(10) {
		t.Fatal("expected ReleaseResource to release attached resource")
	}
	if !released {
		t.Fatal("release function was not called")
	}
	if !canceled {
		t.Fatal("cancel function was not called")
	}

	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

func TestAddResourceValueWakesPendingScanner(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	waitCh := resourceServerWaitCh(client.server)
	if _, err := client.AddResourceValue(srpc.NewMux(), &mockSRPCClient{id: 2}, nil); err != nil {
		t.Fatalf("AddResourceValue: %v", err)
	}
	assertWaitChClosed(t, waitCh)
}

func TestReleaseResourceWakesClientQueue(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	id, err := client.AddResource(srpc.NewMux(), nil)
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	waitCh := resourceServerWaitCh(client.server)

	if !client.ReleaseResource(id) {
		t.Fatal("expected ReleaseResource to release server-owned resource")
	}
	assertWaitChClosed(t, waitCh)
}

func TestReleaseAttachedResourceWakesClientQueue(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	mc := &mockSRPCClient{id: 3}
	err := client.AddAttachedResource(12, "attached", func() {}, mc, func() {})
	if err != nil {
		t.Fatalf("AddAttachedResource: %v", err)
	}
	waitCh := resourceServerWaitCh(client.server)

	if !client.ReleaseResource(12) {
		t.Fatal("expected ReleaseResource to release attached resource")
	}
	assertWaitChClosed(t, waitCh)
	if len(client.txQueue) != 1 ||
		client.txQueue[0].GetResourceReleased().GetResourceId() != 12 {
		t.Fatalf("attached release notifications = %#v", client.txQueue)
	}
}

func TestRemoveAttachedResource_NotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	// Should not panic when removing a non-existent resource.
	client.RemoveAttachedResource(999)
}

func TestRemoveAttachedResourceDoesNotAffectOthers(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	canceled1 := false
	canceled2 := false
	mc1 := &mockSRPCClient{id: 1}
	mc2 := &mockSRPCClient{id: 2}

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

func TestGetAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	mc := &mockSRPCClient{id: 42}
	err := client.AddAttachedResource(5, "my-resource", func() {}, mc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

func TestGetAttachedResource_NotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	_, err := client.GetAttachedResource(999)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

func TestAddResourceValueAndGetResourceValue(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	want := &mockSRPCClient{id: 99}
	id, err := client.AddResourceValue(srpc.NewMux(), want, nil)
	if err != nil {
		t.Fatalf("unexpected error adding resource: %v", err)
	}

	got, err := client.GetResourceValue(id)
	if err != nil {
		t.Fatalf("unexpected error getting resource value: %v", err)
	}
	if got != want {
		t.Fatal("returned resource value does not match")
	}
}

func TestGetResourceValueNotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	_, err := client.GetResourceValue(404)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

func TestReleaseAllAttachedResources_CancelsAll(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	canceled := make(map[uint32]bool)
	for i := uint32(1); i <= 3; i++ {
		id := i
		mc := &mockSRPCClient{id: int(id)}
		err := client.AddAttachedResource(id, "res", func() { canceled[id] = true }, mc, nil)
		if err != nil {
			t.Fatalf("unexpected error adding resource %d: %v", id, err)
		}
	}

	client.releaseAllAttachedResources()

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
