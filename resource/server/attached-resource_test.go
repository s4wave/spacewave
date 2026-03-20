package resource_server

import (
	"context"
	"testing"

	"github.com/aperturerobotics/bldr/resource"
	"github.com/aperturerobotics/starpc/srpc"
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

// newTestClient creates a RemoteResourceClient suitable for testing attached resource methods.
func newTestClient(t *testing.T) (*RemoteResourceClient, context.CancelFunc) {
	t.Helper()
	s := NewResourceServer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	client := &RemoteResourceClient{
		server:    s,
		clientID:  1,
		ctx:       ctx,
		resources: make(map[uint32]*trackedResource),
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.clients[1] = client
	})
	return client, cancel
}

func TestAddAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(42, "test-resource", func() {}, mc)
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

	if client.attachedResources != nil {
		t.Fatal("attachedResources should be nil before first AddAttachedResource")
	}

	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(1, "label", func() {}, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.attachedResources == nil {
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
	err := client.AddAttachedResource(1, "label", func() {}, mc)
	if err != resource.ErrClientReleased {
		t.Fatalf("got error %v, want %v", err, resource.ErrClientReleased)
	}
}

func TestRemoveAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	canceled := false
	mc := &mockSRPCClient{id: 1}
	err := client.AddAttachedResource(10, "label", func() { canceled = true }, mc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	client.RemoveAttachedResource(10)

	if !canceled {
		t.Fatal("cancel function was not called")
	}

	_, err = client.GetAttachedResource(10)
	if err != resource.ErrResourceNotFound {
		t.Fatalf("got error %v, want %v", err, resource.ErrResourceNotFound)
	}
}

func TestRemoveAttachedResource_NotFound(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	// Should not panic when removing a non-existent resource.
	client.RemoveAttachedResource(999)
}

func TestGetAttachedResource_Success(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	mc := &mockSRPCClient{id: 42}
	err := client.AddAttachedResource(5, "my-resource", func() {}, mc)
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

func TestReleaseAllAttachedResources_CancelsAll(t *testing.T) {
	client, cancel := newTestClient(t)
	defer cancel()

	canceled := make(map[uint32]bool)
	for i := uint32(1); i <= 3; i++ {
		id := i
		mc := &mockSRPCClient{id: int(id)}
		err := client.AddAttachedResource(id, "res", func() { canceled[id] = true }, mc)
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
