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

// CheckDetectedLoss validates the contract for a backend that declares
// involuntary lease-loss detection.
func CheckDetectedLoss(
	t *testing.T,
	capability *coord.Capability,
	lease coord.WriteLease,
	sever func(),
) {
	t.Helper()
	if capability == nil || !capability.DetectsLoss {
		t.Fatalf("capability does not declare loss detection: %#v", capability)
	}
	select {
	case <-lease.Done():
		t.Fatal("Done closed before the underlying hold was severed")
	default:
	}
	if err := lease.Err(); err != nil {
		t.Fatalf("held lease Err() = %v, want nil", err)
	}

	sever()
	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("Done did not close after the underlying hold was severed")
	}
	if err := lease.Err(); err == nil {
		t.Fatal("lost lease Err() = nil, want loss error")
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("lost lease Release() error = %v", err)
	}
}

// Check validates the shared coordinator contract.
func Check(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("generation root prefix and missed event recovery", func(t *testing.T) {
		checkGenerationRootPrefixAndMissedEventRecovery(t, factory)
	})
	t.Run("lease wait and release", func(t *testing.T) {
		checkLeaseWaitAndRelease(t, factory)
	})
	t.Run("release with canceled context", func(t *testing.T) {
		checkReleaseWithCanceledContext(t, factory)
	})
	t.Run("try while held not acquired", func(t *testing.T) {
		checkTryWhileHeldNotAcquired(t, factory)
	})
	t.Run("done and err after release", func(t *testing.T) {
		checkDoneAndErrAfterRelease(t, factory)
	})
	t.Run("keyed scope exclusion without collision", func(t *testing.T) {
		checkKeyedScopeExclusionWithoutCollision(t, factory)
	})
	t.Run("keyed scope without generations", func(t *testing.T) {
		checkKeyedScopeWithoutGenerations(t, factory)
	})
	t.Run("unsupported fallback", func(t *testing.T) {
		checkUnsupportedFallback(t)
	})
}

func checkTryWhileHeldNotAcquired(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, secondC := factory(t)
	first := coord.Scope{
		VolumeID:      "volume-try",
		ObjectStoreID: "objects-try",
		ParticipantID: "first",
	}
	second := coord.Scope{
		VolumeID:      "volume-try",
		ObjectStoreID: "objects-try",
		ParticipantID: "second",
	}

	lease, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	held, ok, err := secondC.TryAcquireWriteLease(ctx, second)
	if err != nil {
		t.Fatalf("contended TryAcquireWriteLease() error = %v, want nil", err)
	}
	if ok || held != nil {
		t.Fatalf("contended TryAcquireWriteLease() = (%v, %v), want (nil, false)", held, ok)
	}
}

func checkDoneAndErrAfterRelease(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, _ := factory(t)
	scope := coord.Scope{
		VolumeID:      "volume-done",
		ObjectStoreID: "objects-done",
		ParticipantID: "first",
	}

	lease, ok, err := firstC.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}

	select {
	case <-lease.Done():
		t.Fatal("Done closed while the lease was held")
	default:
	}
	if err := lease.Err(); err != nil {
		t.Fatalf("held lease Err() = %v, want nil", err)
	}

	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after release")
	}
	if err := lease.Err(); err != nil {
		t.Fatalf("cleanly released lease Err() = %v, want nil", err)
	}
}

// checkKeyedScopeExclusionWithoutCollision proves keyed scopes exclude one
// another per key without contending with the ObjectStore scope.
func checkKeyedScopeExclusionWithoutCollision(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, secondC := factory(t)
	keyed := coord.Scope{
		VolumeID:      "volume-keyed",
		ParticipantID: "first",
		Key:           "world-1",
	}

	capability, err := firstC.Capability(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported {
		t.Fatalf("keyed capability unsupported: %#v", capability)
	}

	keyedLease, ok, err := firstC.TryAcquireWriteLease(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("keyed lease unexpectedly busy")
	}

	contended, ok, err := secondC.TryAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      "volume-keyed",
		ParticipantID: "second",
		Key:           "world-1",
	})
	if err != nil {
		t.Fatalf("contended keyed TryAcquireWriteLease() error = %v, want nil", err)
	}
	if ok || contended != nil {
		t.Fatalf("second holder acquired held key: (%v, %v)", contended, ok)
	}

	otherKeyLease, ok, err := secondC.TryAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      "volume-keyed",
		ParticipantID: "second",
		Key:           "world-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("distinct key blocked by held key")
	}
	defer otherKeyLease.Release(ctx)

	storeLease, ok, err := secondC.TryAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      "volume-keyed",
		ObjectStoreID: "objects-keyed",
		ParticipantID: "second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ObjectStore scope blocked by held keyed scope")
	}
	defer storeLease.Release(ctx)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := keyedLease.Release(canceled); err != nil {
		t.Fatalf("keyed Release() with canceled context error = %v", err)
	}
	select {
	case <-keyedLease.Done():
	case <-time.After(time.Second):
		t.Fatal("keyed Done not closed after release")
	}
	if err := keyedLease.Err(); err != nil {
		t.Fatalf("cleanly released keyed lease Err() = %v, want nil", err)
	}

	reacquired, ok, err := secondC.TryAcquireWriteLease(ctx, coord.Scope{
		VolumeID:      "volume-keyed",
		ParticipantID: "second",
		Key:           "world-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("keyed scope still held after release")
	}
	if err := reacquired.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// checkKeyedScopeWithoutGenerations proves a pure exclusion scope declares
// Generations false and declines Refresh and Publish.
func checkKeyedScopeWithoutGenerations(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, _ := factory(t)
	keyed := coord.Scope{
		VolumeID:      "volume-keyed-generation",
		ParticipantID: "first",
		Key:           "world-1",
	}

	capability, err := firstC.Capability(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Generations {
		t.Fatalf("keyed capability declares generations: %#v", capability)
	}

	lease, ok, err := firstC.TryAcquireWriteLease(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("keyed lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	if _, err := lease.Refresh(ctx); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Refresh() error = %v, want ErrUnsupported", err)
	}
	if _, err := lease.Publish(ctx, coord.Event{}); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Publish() error = %v, want ErrUnsupported", err)
	}
}

func checkGenerationRootPrefixAndMissedEventRecovery(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	writerC, readerC := factory(t)
	writer := coord.Scope{
		VolumeID:      "volume-generation",
		ObjectStoreID: "objects-generation",
		ParticipantID: "writer",
	}
	reader := coord.Scope{
		VolumeID:      "volume-generation",
		ObjectStoreID: "objects-generation",
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
		VolumeID:      "volume-wait",
		ObjectStoreID: "objects-wait",
		ParticipantID: "first",
	}
	second := coord.Scope{
		VolumeID:      "volume-wait",
		ObjectStoreID: "objects-wait",
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

// checkReleaseWithCanceledContext proves a lease cannot be stranded by
// cancellation. Cleanup paths release with the same context that was just
// canceled, so a Release that returned early there would leave the scope locked
// forever and hang every later writer.
func checkReleaseWithCanceledContext(t *testing.T, factory Factory) {
	t.Helper()

	ctx := context.Background()
	firstC, secondC := factory(t)
	first := coord.Scope{
		VolumeID:      "volume-canceled",
		ObjectStoreID: "objects-canceled",
		ParticipantID: "first",
	}
	second := coord.Scope{
		VolumeID:      "volume-canceled",
		ObjectStoreID: "objects-canceled",
		ParticipantID: "second",
	}

	leaseA, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := leaseA.Release(canceled); err != nil {
		t.Fatalf("Release() with canceled context error = %v", err)
	}

	leaseB, ok, err := secondC.TryAcquireWriteLease(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease still held after release with a canceled context")
	}
	if err := leaseB.Release(ctx); err != nil {
		t.Fatal(err)
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
