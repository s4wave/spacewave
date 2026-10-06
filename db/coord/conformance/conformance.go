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
	// Require declared loss detection and verify the write lease remains held.
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

	// Sever the underlying lease hold and wait for its loss notification.
	sever()
	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("Done did not close after the underlying hold was severed")
	}

	// Verify lease loss reports an error and still permits release.
	if err := lease.Err(); err == nil {
		t.Fatal("lost lease Err() = nil, want loss error")
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("lost lease Release() error = %v", err)
	}
}

// Check validates the shared coordinator contract.
func Check(t *testing.T, factory Factory) {
	// Verify coordinator generations and recovery of missed root changes.
	t.Helper()
	t.Run("generation root prefix and missed event recovery", func(t *testing.T) {
		checkGenerationRootPrefixAndMissedEventRecovery(t, factory)
	})

	// Verify waiting writers acquire a lease after its release.
	t.Run("lease wait and release", func(t *testing.T) {
		checkLeaseWaitAndRelease(t, factory)
	})

	// Verify cancellation cannot strand a held write lease.
	t.Run("release with canceled context", func(t *testing.T) {
		checkReleaseWithCanceledContext(t, factory)
	})

	// Verify a contended lease attempt reports the scope as busy.
	t.Run("try while held not acquired", func(t *testing.T) {
		checkTryWhileHeldNotAcquired(t, factory)
	})

	// Verify lease release publishes completion without an error.
	t.Run("done and err after release", func(t *testing.T) {
		checkDoneAndErrAfterRelease(t, factory)
	})

	// Verify keyed leases exclude competing writers without colliding with store leases.
	t.Run("keyed scope exclusion without collision", func(t *testing.T) {
		checkKeyedScopeExclusionWithoutCollision(t, factory)
	})

	// Verify keyed leases decline generation tracking operations.
	t.Run("keyed scope without generations", func(t *testing.T) {
		checkKeyedScopeWithoutGenerations(t, factory)
	})

	// Verify unsupported coordinators expose their fallback contract.
	t.Run("unsupported fallback", func(t *testing.T) {
		checkUnsupportedFallback(t)
	})
}

func checkTryWhileHeldNotAcquired(t *testing.T, factory Factory) {
	// Create two coordinator participants for the same contended store scope.
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

	// Acquire the first participant write lease and release it after the check.
	lease, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	// Verify the second participant cannot acquire the held store scope.
	held, ok, err := secondC.TryAcquireWriteLease(ctx, second)
	if err != nil {
		t.Fatalf("contended TryAcquireWriteLease() error = %v, want nil", err)
	}
	if ok || held != nil {
		t.Fatalf("contended TryAcquireWriteLease() = (%v, %v), want (nil, false)", held, ok)
	}
}

func checkDoneAndErrAfterRelease(t *testing.T, factory Factory) {
	// Create a coordinator participant for lease completion checks.
	t.Helper()
	ctx := context.Background()
	firstC, _ := factory(t)
	scope := coord.Scope{
		VolumeID:      "volume-done",
		ObjectStoreID: "objects-done",
		ParticipantID: "first",
	}

	// Acquire the write lease whose completion state will be checked.
	lease, ok, err := firstC.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}

	// Verify the held write lease has no completion signal or error.
	select {
	case <-lease.Done():
		t.Fatal("Done closed while the lease was held")
	default:
	}
	if err := lease.Err(); err != nil {
		t.Fatalf("held lease Err() = %v, want nil", err)
	}

	// Release the write lease and wait for its completion signal.
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after release")
	}

	// Verify a clean lease release preserves a nil error.
	if err := lease.Err(); err != nil {
		t.Fatalf("cleanly released lease Err() = %v, want nil", err)
	}
}

// checkKeyedScopeExclusionWithoutCollision proves keyed scopes exclude one
// another per key without contending with the ObjectStore scope.
func checkKeyedScopeExclusionWithoutCollision(t *testing.T, factory Factory) {
	// Create coordinator participants for keyed scope exclusion checks.
	t.Helper()
	ctx := context.Background()
	firstC, secondC := factory(t)
	keyed := coord.Scope{
		VolumeID:      "volume-keyed",
		ParticipantID: "first",
		Key:           "world-1",
	}

	// Verify the coordinator supports write leases on keyed scopes.
	capability, err := firstC.Capability(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported {
		t.Fatalf("keyed capability unsupported: %#v", capability)
	}

	// Acquire the first keyed write lease for exclusion checks.
	keyedLease, ok, err := firstC.TryAcquireWriteLease(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("keyed lease unexpectedly busy")
	}

	// Verify another participant cannot acquire the same held key.
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

	// Verify a distinct keyed scope remains available while the first key is held.
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

	// Verify a store scope remains available while a keyed scope is held.
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

	// Release the keyed write lease using an already canceled context.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := keyedLease.Release(canceled); err != nil {
		t.Fatalf("keyed Release() with canceled context error = %v", err)
	}

	// Verify keyed lease completion closes without a release error.
	select {
	case <-keyedLease.Done():
	case <-time.After(time.Second):
		t.Fatal("keyed Done not closed after release")
	}
	if err := keyedLease.Err(); err != nil {
		t.Fatalf("cleanly released keyed lease Err() = %v, want nil", err)
	}

	// Verify another participant can acquire and release the freed key.
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
	// Create a keyed scope for coordinator generation capability checks.
	t.Helper()
	ctx := context.Background()
	firstC, _ := factory(t)
	keyed := coord.Scope{
		VolumeID:      "volume-keyed-generation",
		ParticipantID: "first",
		Key:           "world-1",
	}

	// Verify keyed scope capability declines generation tracking.
	capability, err := firstC.Capability(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Generations {
		t.Fatalf("keyed capability declares generations: %#v", capability)
	}

	// Acquire the keyed write lease and release it after the check.
	lease, ok, err := firstC.TryAcquireWriteLease(ctx, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("keyed lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	// Verify keyed write leases decline snapshot refresh and event publication.
	if _, err := lease.Refresh(ctx); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Refresh() error = %v, want ErrUnsupported", err)
	}
	if _, err := lease.Publish(ctx, coord.Event{}); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Publish() error = %v, want ErrUnsupported", err)
	}
}

func checkGenerationRootPrefixAndMissedEventRecovery(t *testing.T, factory Factory) {
	// Create writer and reader participants for generation recovery checks.
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

	// Read the generation before the change. A backend may count
	// publications or its own durable commits, so the publication itself need
	// not advance it.
	before, err := readerC.Snapshot(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}

	// Watch the reader scope before publishing a root change.
	root := &bucket.ObjectRef{BucketId: "bucket-a"}
	liveWatch, err := readerC.Watch(ctx, reader, before.Generation)
	if err != nil {
		t.Fatal(err)
	}
	defer liveWatch.Close()

	// Acquire the writer lease and release it after the check.
	lease, ok, err := writerC.TryAcquireWriteLease(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("writer lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	// Publish a root change and its affected key prefix.
	snapshot, err := lease.Publish(ctx, coord.Event{
		RootChanged:      root,
		KeyPrefixChanged: []byte("world-head/"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the published snapshot contains the new generation and root.
	generation := snapshot.Generation
	if generation < before.Generation {
		t.Fatalf("snapshot generation = %d, want at least %d", generation, before.Generation)
	}
	if !snapshot.Root.EqualsRef(root) {
		t.Fatalf("snapshot root = %#v, want %#v", snapshot.Root, root)
	}

	// Verify the live watch delivers the root change and affected key prefix.
	liveEvent := nextEvent(t, liveWatch.Events())
	if liveEvent.Generation != generation {
		t.Fatalf("live event generation = %d, want %d", liveEvent.Generation, generation)
	}
	if !liveEvent.RootChanged.EqualsRef(root) {
		t.Fatalf("live event root = %#v, want %#v", liveEvent.RootChanged, root)
	}
	if string(liveEvent.KeyPrefixChanged) != "world-head/" {
		t.Fatalf("live event prefix = %q, want world-head/", liveEvent.KeyPrefixChanged)
	}

	// Recover the current snapshot independently of the watch event.
	recovered, err := readerC.Snapshot(ctx, reader)
	if err != nil {
		t.Fatal(err)
	}

	// Verify snapshot recovery retains the published generation and root.
	if recovered.Generation != generation {
		t.Fatalf("recovered generation = %d, want %d", recovered.Generation, generation)
	}
	if !recovered.Root.EqualsRef(root) {
		t.Fatalf("recovered root = %#v, want %#v", recovered.Root, root)
	}

	// Start another watch from the generation preceding the root change.
	watch, err := readerC.Watch(ctx, reader, before.Generation)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Verify the new watch recovers the missed generation and root change.
	event := nextEvent(t, watch.Events())
	if event.Generation != generation {
		t.Fatalf("watch event generation = %d, want %d", event.Generation, generation)
	}
	if !event.RootChanged.EqualsRef(root) {
		t.Fatalf("watch event root = %#v, want %#v", event.RootChanged, root)
	}
}

func checkLeaseWaitAndRelease(t *testing.T, factory Factory) {
	// Create competing coordinator participants for lease wait checks.
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

	// Watch the waiting participant for lock demand events.
	watch, err := secondC.Watch(ctx, second, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Acquire the first write lease to hold the shared scope.
	leaseA, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Start a competing writer that releases its lease after acquisition.
	waitErr := make(chan error, 1)
	go func() {
		// Acquire and release the competing write lease when the scope becomes available.
		leaseB, err := secondC.WaitAcquireWriteLease(ctx, second)
		if err == nil {
			err = leaseB.Release(ctx)
		}

		// Deliver the competing writer result to the check.
		waitErr <- err
	}()

	// Verify the coordinator publishes the competing writer lock demand.
	wantLock := nextEvent(t, watch.Events())
	if !wantLock.WantLock {
		t.Fatalf("expected want-lock event, got %#v", wantLock)
	}

	// Release the first write lease and wait for the competing writer to complete.
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}

	// Verify a released write lease cannot refresh its snapshot.
	if _, err := leaseA.Refresh(ctx); !errors.Is(err, coord.ErrLeaseReleased) {
		t.Fatalf("released lease Refresh() error = %v, want ErrLeaseReleased", err)
	}
}

// checkReleaseWithCanceledContext proves a lease cannot be stranded by
// cancellation. Cleanup paths release with the same context that was just
// canceled, so a Release that returned early there would leave the scope locked
// forever and hang every later writer.
func checkReleaseWithCanceledContext(t *testing.T, factory Factory) {
	// Create competing coordinator participants for canceled lease release checks.
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

	// Acquire the first write lease to hold the shared scope.
	leaseA, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Verify an already canceled context still releases the held write lease.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := leaseA.Release(canceled); err != nil {
		t.Fatalf("Release() with canceled context error = %v", err)
	}

	// Verify the second participant can acquire and release the freed scope.
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
	// Create an unsupported coordinator and its store scope.
	t.Helper()
	ctx := context.Background()
	c := coord.NewUnsupportedCoordinator(coord.BackendKindUnsupported, coord.FallbackReasonUnsupported)
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "reader",
	}

	// Read the unsupported coordinator capability record.
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}

	// Verify unsupported capability reports its support status and fallback reason.
	if capability.Supported {
		t.Fatal("unsupported capability reported supported")
	}
	if capability.FallbackReason != coord.FallbackReasonUnsupported {
		t.Fatalf("fallback reason = %q, want unsupported", capability.FallbackReason)
	}

	// Verify the unsupported coordinator declines snapshot reads.
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
