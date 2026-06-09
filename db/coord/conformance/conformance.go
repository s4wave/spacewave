package conformance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
)

// Factory constructs two handles for one coordinated scope.
type Factory func(t testing.TB) (coord.Coordinator, coord.Coordinator)

// Check validates the shared coordinator contract.
func Check(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("generation root prefix and missed event recovery", func(t *testing.T) {
		checkGenerationRootPrefixAndMissedEventRecovery(t, factory)
	})
	t.Run("lease wait and release", func(t *testing.T) {
		checkLeaseWaitAndRelease(t, factory)
	})
	t.Run("unsupported fallback", func(t *testing.T) {
		checkUnsupportedFallback(t)
	})
}

func checkGenerationRootPrefixAndMissedEventRecovery(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	writerC, readerC := factory(t)
	writer := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "writer",
	}
	reader := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "reader",
	}

	root := &bucket.ObjectRef{BucketId: "bucket-a"}
	liveWatch, err := readerC.Watch(ctx, reader, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer liveWatch.Close()

	lease, ok, err := writerC.TryAcquireWriteLease(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("writer lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	snapshot, err := lease.Publish(ctx, coord.Event{
		RootChanged:      root,
		KeyPrefixChanged: []byte("world-head/"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 {
		t.Fatalf("snapshot generation = %d, want 1", snapshot.Generation)
	}
	if !snapshot.Root.EqualsRef(root) {
		t.Fatalf("snapshot root = %#v, want %#v", snapshot.Root, root)
	}

	liveEvent := nextEvent(t, liveWatch.Events())
	if liveEvent.Generation != 1 {
		t.Fatalf("live event generation = %d, want 1", liveEvent.Generation)
	}
	if !liveEvent.RootChanged.EqualsRef(root) {
		t.Fatalf("live event root = %#v, want %#v", liveEvent.RootChanged, root)
	}
	if string(liveEvent.KeyPrefixChanged) != "world-head/" {
		t.Fatalf("live event prefix = %q, want world-head/", liveEvent.KeyPrefixChanged)
	}

	recovered, err := readerC.Snapshot(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Generation != 1 {
		t.Fatalf("recovered generation = %d, want 1", recovered.Generation)
	}
	if !recovered.Root.EqualsRef(root) {
		t.Fatalf("recovered root = %#v, want %#v", recovered.Root, root)
	}

	watch, err := readerC.Watch(ctx, reader, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	event := nextEvent(t, watch.Events())
	if event.Generation != 1 {
		t.Fatalf("watch event generation = %d, want 1", event.Generation)
	}
	if !event.RootChanged.EqualsRef(root) {
		t.Fatalf("watch event root = %#v, want %#v", event.RootChanged, root)
	}
}

func checkLeaseWaitAndRelease(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, secondC := factory(t)
	first := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "first",
	}
	second := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "second",
	}

	watch, err := secondC.Watch(ctx, second, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	leaseA, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	waitErr := make(chan error, 1)
	go func() {
		leaseB, err := secondC.WaitAcquireWriteLease(ctx, second)
		if err == nil {
			err = leaseB.Release(ctx)
		}
		waitErr <- err
	}()

	wantLock := nextEvent(t, watch.Events())
	if !wantLock.WantLock {
		t.Fatalf("expected want-lock event, got %#v", wantLock)
	}

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

func checkUnsupportedFallback(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	c := coord.NewUnsupportedCoordinator(coord.BackendKindUnsupported, coord.FallbackReasonUnsupported)
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "reader",
	}

	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Supported {
		t.Fatal("unsupported capability reported supported")
	}
	if capability.FallbackReason != coord.FallbackReasonUnsupported {
		t.Fatalf("fallback reason = %q, want unsupported", capability.FallbackReason)
	}
	if _, err := c.Snapshot(ctx, scope); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("Snapshot() error = %v, want ErrUnsupported", err)
	}
}

func nextEvent(t *testing.T, events <-chan coord.Event) coord.Event {
	t.Helper()

	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("watch closed before event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	return coord.Event{}
}
