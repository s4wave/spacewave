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
	ctx := context.Background()
	c := NewCoordinator()
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported || capability.Backend != coord.BackendKindInMemory {
		t.Fatalf("unexpected capability: %#v", capability)
	}

	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}

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

func TestCoordinatorLeaseWaitsForRelease(t *testing.T) {
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

	leaseA, ok, err := c.TryAcquireWriteLease(ctx, scopeA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	waitErr := make(chan error, 1)
	go func() {
		leaseB, err := c.WaitAcquireWriteLease(ctx, scopeB)
		if err == nil {
			err = leaseB.Release(ctx)
		}
		waitErr <- err
	}()

	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}

	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
	if _, err := leaseA.Refresh(ctx); !errors.Is(err, coord.ErrLeaseReleased) {
		t.Fatalf("released lease Refresh() error = %v, want ErrLeaseReleased", err)
	}
}

func TestCoordinatorWatchClosesOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCoordinator()
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	select {
	case _, ok := <-watch.Events():
		if ok {
			t.Fatal("watch delivered event after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not close after context cancellation")
	}

	c.mu.Lock()
	watcherCount := len(c.getScopeLocked(scope).watchers)
	c.mu.Unlock()
	if watcherCount != 0 {
		t.Fatalf("watcher count = %d, want 0", watcherCount)
	}
}
