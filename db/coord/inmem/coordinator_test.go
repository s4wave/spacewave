package inmem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
)

func TestCoordinatorPublishesGenerationRootAndPrefixEvents(t *testing.T) {
	// Create a coordinator and the test scope.
	ctx := context.Background()
	c := NewCoordinator()
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	// Verify the coordinator reports in-memory capability for the scope.
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported || capability.Backend != coord.BackendKindInMemory {
		t.Fatalf("unexpected capability: %#v", capability)
	}

	// Start a watch and acquire the write lease.
	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Acquire the write lease for the scope.
	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}

	// Publish a generation change and check the returned snapshot.
	root := &bucket.ObjectRef{BucketId: "bucket-a"}
	snapshot, err := lease.Publish(ctx, coord.Event{
		RootChanged:      root,
		KeyPrefixChanged: []byte("world-head/"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 {
		t.Fatalf("unexpected snapshot generation: %d", snapshot.Generation)
	}
	if !snapshot.Root.EqualsRef(root) {
		t.Fatalf("unexpected snapshot root: %#v", snapshot.Root)
	}

	// Verify the watch delivers the published event.
	event := <-watch.Events()
	if event.ProcessID != "process-a" {
		t.Fatalf("unexpected process id: %q", event.ProcessID)
	}
	if event.VolumeID != "volume-a" || event.ObjectStoreID != "objects" {
		t.Fatalf("unexpected event scope: %#v", event)
	}
	if event.Generation != 1 {
		t.Fatalf("unexpected event generation: %d", event.Generation)
	}
	if !event.RootChanged.EqualsRef(root) {
		t.Fatalf("unexpected root event: %#v", event.RootChanged)
	}
	if string(event.KeyPrefixChanged) != "world-head/" {
		t.Fatalf("unexpected prefix event: %q", event.KeyPrefixChanged)
	}
}

// TestCoordinatorReplayCarriesMissedKeyPrefix verifies that a watcher attaching
// after a publish receives the missed key prefix in its replayed event.
func TestCoordinatorReplayCarriesMissedKeyPrefix(t *testing.T) {
	// Create a coordinator and the test scope.
	ctx := context.Background()
	c := NewCoordinator()
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	// Publish an event before any watcher attaches.
	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}
	root := &bucket.ObjectRef{BucketId: "bucket-a"}
	if _, err := lease.Publish(ctx, coord.Event{
		RootChanged:      root,
		KeyPrefixChanged: []byte("world-head/"),
	}); err != nil {
		t.Fatal(err)
	}

	// Attach a watcher from generation 0 and read the replayed event.
	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Attach a watcher from generation 0 and read the replayed event.
	event := <-watch.Events()
	if event.Generation != 1 {
		t.Fatalf("unexpected replay generation: %d", event.Generation)
	}
	if !event.RootChanged.EqualsRef(root) {
		t.Fatalf("unexpected replay root: %#v", event.RootChanged)
	}
	if string(event.KeyPrefixChanged) != "world-head/" {
		t.Fatalf("unexpected replay prefix: %q", event.KeyPrefixChanged)
	}
}

func TestCoordinatorLeaseWaitsForRelease(t *testing.T) {
	// Create a coordinator and two participant scopes.
	ctx := context.Background()
	c := NewCoordinator()
	scopeA := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}
	scopeB := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-b",
	}

	// Acquire the write lease for the first scope.
	leaseA, ok, err := c.TryAcquireWriteLease(ctx, scopeA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Wait for the second lease in a goroutine.
	waitErr := make(chan error, 1)
	go func() {
		leaseB, err := c.WaitAcquireWriteLease(ctx, scopeB)
		if err == nil {
			err = leaseB.Release(ctx)
		}
		waitErr <- err
	}()

	// Release the first lease and let the waiter proceed.
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the waiter acquired and the released lease is invalid.
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
	if _, err := leaseA.Refresh(ctx); !errors.Is(err, coord.ErrLeaseReleased) {
		t.Fatalf("released lease Refresh() error = %v, want ErrLeaseReleased", err)
	}
}

func TestCoordinatorWatchClosesOnContextCancel(t *testing.T) {
	// Create a coordinator, scope, and cancelable context.
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCoordinator()
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	// Start a watch on the scope.
	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Cancel the watch context.
	cancel()

	// Verify the watch closes and the watcher is removed.
	select {
	case _, ok := <-watch.Events():
		if ok {
			t.Fatal("watch delivered event after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not close after context cancellation")
	}

	// Verify the watcher count returns to zero.
	c.mu.Lock()
	watcherCount := len(c.getScopeLocked(scope).watchers)
	c.mu.Unlock()
	if watcherCount != 0 {
		t.Fatalf("watcher count = %d, want 0", watcherCount)
	}
}
