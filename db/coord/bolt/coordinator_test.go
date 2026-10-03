//go:build !js && !wasip1

package bolt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

const (
	multiprocessLeaseRoleEnv    = "SPACEWAVE_COORD_BOLT_LEASE_ROLE"
	multiprocessLeaseDBPathEnv  = "SPACEWAVE_COORD_BOLT_LEASE_DB_PATH"
	multiprocessLeaseHeldEnv    = "SPACEWAVE_COORD_BOLT_LEASE_HELD_PATH"
	multiprocessLeaseReleaseEnv = "SPACEWAVE_COORD_BOLT_LEASE_RELEASE_PATH"
)

func TestCoordinatorUsesBoltCommitGeneration(t *testing.T) {
	// Open a bbolt coordinator for the writer scope.
	ctx := context.Background()
	db := openTestDB(t)
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "writer",
	}

	// Read and verify the bbolt coordination capability.
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported {
		t.Fatal("expected bbolt coordinator capability to be supported")
	}
	if capability.Backend != coord.BackendKindBbolt {
		t.Fatalf("backend = %q, want bbolt", capability.Backend)
	}
	if capability.Generation != 0 {
		t.Fatalf("initial generation = %d, want 0", capability.Generation)
	}

	// Watch the writer scope for committed generation changes.
	watch, err := c.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Acquire the writer lease for the first commit.
	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}

	// Verify the lease starts at generation zero.
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("refresh generation before commit = %d, want 0", snapshot.Generation)
	}

	// Commit a value and verify the lease sees the new generation.
	writeBoltValue(t, db, "one")
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("refresh generation after commit = %d, want 1", snapshot.Generation)
	}

	// Verify the watch reports the committed generation.
	event := nextEvent(t, watch.Events())
	if event.Generation != 1 {
		t.Fatalf("commit watch generation = %d, want 1", event.Generation)
	}

	// Publish the key-prefix change at the committed generation.
	if snapshot, err := lease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("k/")}); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("publish generation = %d, want 1", snapshot.Generation)
	}

	// Release the writer lease after publishing.
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorIndependentHandles(t *testing.T) {
	// Open independent writer and reader coordinators over one database.
	ctx := context.Background()
	db := openTestDB(t)
	inner := coord_inmem.NewCoordinator()
	writerC := NewCoordinator(db, inner)
	readerC := NewCoordinator(db, inner)
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

	// Watch the reader scope for writer changes.
	watch, err := readerC.Watch(ctx, reader, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Acquire the writer lease while the reader watch remains active.
	lease, ok, err := writerC.TryAcquireWriteLease(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("writer lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	// Verify the writer lease starts at generation zero.
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("refresh generation before commit = %d, want 0", snapshot.Generation)
	}

	// Commit a value and verify the writer generation advances.
	writeBoltValue(t, db, "two")
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("refresh generation after commit = %d, want 1", snapshot.Generation)
	}

	// Publish the new root and key prefix through the writer lease.
	root := &bucket.ObjectRef{BucketId: "bucket-b"}
	snapshot, err := lease.Publish(ctx, coord.Event{
		RootChanged:      root,
		KeyPrefixChanged: []byte("k/"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 {
		t.Fatalf("publish generation = %d, want 1", snapshot.Generation)
	}
	if !snapshot.Root.EqualsRef(root) {
		t.Fatalf("publish root = %#v, want %#v", snapshot.Root, root)
	}

	// Find the root and key-prefix notification among the watch events.
	foundRootPrefixEvent := false
	for range 2 {
		event := nextEvent(t, watch.Events())
		if event.RootChanged.EqualsRef(root) && string(event.KeyPrefixChanged) == "k/" {
			foundRootPrefixEvent = true
		}
	}
	if !foundRootPrefixEvent {
		t.Fatal("watch did not receive root/key-prefix event")
	}

	// Verify the independent reader recovers the committed root and generation.
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
}

func TestCoordinatorLeaseWaitsForRelease(t *testing.T) {
	// Open two coordinators for competing writer scopes.
	ctx := context.Background()
	db := openTestDB(t)
	inner := coord_inmem.NewCoordinator()
	firstC := NewCoordinator(db, inner)
	secondC := NewCoordinator(db, inner)
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

	// Watch the second writer for lock demand.
	watch, err := secondC.Watch(ctx, second, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Acquire the first writer lease before starting the contender.
	leaseA, ok, err := firstC.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Start the second writer and report its eventual release result.
	waitErr := make(chan error, 1)
	go func() {
		leaseB, err := secondC.WaitAcquireWriteLease(ctx, second)
		if err == nil {
			err = leaseB.Release(ctx)
		}
		waitErr <- err
	}()

	// Verify lock demand and release the first writer.
	if event := nextEvent(t, watch.Events()); !event.WantLock {
		t.Fatalf("expected want-lock event, got %#v", event)
	}
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorWaitAcquireWakesOnLocalRelease(t *testing.T) {
	// Open one coordinator for competing local writers.
	ctx := context.Background()
	db := openTestDB(t)
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
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

	// Acquire the first local writer lease.
	leaseA, ok, err := c.TryAcquireWriteLease(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Start a local contender and report its lease result.
	started := make(chan struct{})
	waitErr := make(chan error, 1)
	go func() {
		close(started)
		leaseB, err := c.WaitAcquireWriteLease(ctx, second)
		if err == nil {
			err = leaseB.Release(ctx)
		}
		waitErr <- err
	}()

	// Verify the local contender remains blocked while the lease is held.
	<-started
	select {
	case err := <-waitErr:
		t.Fatalf("wait returned before release: %v", err)
	default:
	}

	// Release the first local writer and await the contender.
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for local release wake")
	}
}

func TestCoordinatorRetriesOwnPostRefreshCommitAndRejectsReleasedWriter(t *testing.T) {
	// Open two coordinators for released and fresh writer leases.
	ctx := context.Background()
	db := openTestDB(t)
	inner := coord_inmem.NewCoordinator()
	writerA := NewCoordinator(db, inner)
	writerB := NewCoordinator(db, inner)
	scopeA := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "writer-a",
	}
	scopeB := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "writer-b",
	}

	// Acquire the first writer lease.
	staleLease, ok, err := writerA.TryAcquireWriteLease(ctx, scopeA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}

	// Refresh the first lease before committing its value.
	if snapshot, err := staleLease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("first refresh generation = %d, want 0", snapshot.Generation)
	}

	// Publish the first writer commit at its durable generation.
	writeBoltValue(t, db, "two")
	if snapshot, err := staleLease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("owned/")}); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("post-refresh publish generation = %d, want 1", snapshot.Generation)
	}

	// Release the first writer before testing its stale lease.
	if err := staleLease.Release(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify a released lease cannot publish a later commit.
	writeBoltValue(t, db, "three")
	if _, err := staleLease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("released/")}); !errors.Is(err, coord.ErrLeaseReleased) {
		t.Fatalf("released publish error = %v, want ErrLeaseReleased", err)
	}

	// Acquire a fresh writer lease after the later commit.
	freshLease, ok, err := writerB.TryAcquireWriteLease(ctx, scopeB)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("second lease unexpectedly busy")
	}

	// Verify the fresh writer sees the later generation.
	if snapshot, err := freshLease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 2 {
		t.Fatalf("second refresh generation = %d, want 2", snapshot.Generation)
	}

	// Publish the fresh writer root and key-prefix event.
	if _, err := freshLease.Publish(ctx, coord.Event{
		RootChanged:      &bucket.ObjectRef{BucketId: "bucket-b"},
		KeyPrefixChanged: []byte("k/"),
	}); err != nil {
		t.Fatal(err)
	}

	// Release the fresh writer lease after publishing.
	if err := freshLease.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorMultiprocessWriteLeaseExcludesContenders(t *testing.T) {
	// Run the selected child-process role when invoked by the parent test.
	if role := os.Getenv(multiprocessLeaseRoleEnv); role != "" {
		runMultiprocessLeaseRole(t, role)
		return
	}

	// Prepare database and synchronization paths for the competing processes.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	heldPath := filepath.Join(dir, "held")
	releasePath := filepath.Join(dir, "release")

	// Create and close the shared database before starting child processes.
	if db, err := bdb.Open(dbPath, 0o600, nil); err != nil {
		t.Fatal(err)
	} else if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Start the holder process and register its cleanup.
	holder := leaseRoleCommand(t, "holder", dbPath, heldPath, releasePath)
	var holderOut bytes.Buffer
	holder.Stdout = &holderOut
	holder.Stderr = &holderOut
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(releasePath, []byte("release"), 0o600)
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})

	// Wait for the holder process to acquire its write lease.
	waitForFile(t, heldPath)

	// Verify a competing process cannot acquire the held write lease.
	if output, err := leaseRoleCommand(t, "contender-busy", dbPath, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-busy failed: %v\n%s", err, output)
	}

	// Release the holder process and verify it exits successfully.
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder failed: %v\n%s", err, holderOut.String())
	}

	// Verify a competing process acquires the released write lease.
	if output, err := leaseRoleCommand(t, "contender-acquire", dbPath, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-acquire failed: %v\n%s", err, output)
	}
}

func openTestDB(t *testing.T) *bdb.DB {
	// Mark database setup failures at the calling test.
	t.Helper()

	// Open a temporary bbolt database and register its cleanup.
	db, err := bdb.Open(filepath.Join(t.TempDir(), "test.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func leaseRoleCommand(t *testing.T, role, dbPath, heldPath, releasePath string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCoordinatorMultiprocessWriteLeaseExcludesContenders$", "-test.v") //nolint:gosec
	cmd.Env = append(os.Environ(),
		multiprocessLeaseRoleEnv+"="+role,
		multiprocessLeaseDBPathEnv+"="+dbPath,
		multiprocessLeaseHeldEnv+"="+heldPath,
		multiprocessLeaseReleaseEnv+"="+releasePath,
	)
	return cmd
}

func runMultiprocessLeaseRole(t *testing.T, role string) {
	// Mark child-process failures at the calling test.
	t.Helper()

	// Bound the child-process lease operation lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open the shared database for the child-process role.
	db, err := bdb.Open(os.Getenv(multiprocessLeaseDBPathEnv), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create the child-process coordinator and participant scope.
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: role,
	}

	// Exercise the selected holder or contender lease operation.
	switch role {
	case "holder":
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("holder lease unexpectedly busy")
		}
		if err := os.WriteFile(os.Getenv(multiprocessLeaseHeldEnv), []byte("held"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitForFile(t, os.Getenv(multiprocessLeaseReleaseEnv))
		if err := lease.Release(ctx); err != nil {
			t.Fatal(err)
		}
	case "contender-busy":
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			_ = lease.Release(ctx)
			t.Fatal("contender acquired write lease while holder process still owned it")
		}
	case "contender-acquire":
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("contender lease busy after holder released")
		}
		if err := lease.Release(ctx); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown role %q", role)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeBoltValue(t *testing.T, db *bdb.DB, value string) {
	t.Helper()

	if err := db.Update(func(tx *bdb.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("coord-test"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("key"), []byte(value))
	}); err != nil {
		t.Fatal(err)
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
