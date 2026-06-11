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
	ctx := context.Background()
	db := openTestDB(t)
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "writer",
	}

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

	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("refresh generation before commit = %d, want 0", snapshot.Generation)
	}

	writeBoltValue(t, db, "one")
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("refresh generation after commit = %d, want 1", snapshot.Generation)
	}

	event := nextEvent(t, watch.Events())
	if event.Generation != 1 {
		t.Fatalf("commit watch generation = %d, want 1", event.Generation)
	}

	if snapshot, err := lease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("k/")}); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("publish generation = %d, want 1", snapshot.Generation)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorIndependentHandles(t *testing.T) {
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

	watch, err := readerC.Watch(ctx, reader, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	lease, ok, err := writerC.TryAcquireWriteLease(ctx, writer)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("writer lease unexpectedly busy")
	}
	defer lease.Release(ctx)

	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("refresh generation before commit = %d, want 0", snapshot.Generation)
	}

	writeBoltValue(t, db, "two")
	if snapshot, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("refresh generation after commit = %d, want 1", snapshot.Generation)
	}

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

func TestCoordinatorRetriesOwnPostRefreshCommitAndRejectsReleasedWriter(t *testing.T) {
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

	staleLease, ok, err := writerA.TryAcquireWriteLease(ctx, scopeA)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first lease unexpectedly busy")
	}
	if snapshot, err := staleLease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 0 {
		t.Fatalf("first refresh generation = %d, want 0", snapshot.Generation)
	}
	writeBoltValue(t, db, "two")
	if snapshot, err := staleLease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("owned/")}); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 1 {
		t.Fatalf("post-refresh publish generation = %d, want 1", snapshot.Generation)
	}
	if err := staleLease.Release(ctx); err != nil {
		t.Fatal(err)
	}

	writeBoltValue(t, db, "three")
	if _, err := staleLease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("released/")}); !errors.Is(err, coord.ErrLeaseReleased) {
		t.Fatalf("released publish error = %v, want ErrLeaseReleased", err)
	}

	freshLease, ok, err := writerB.TryAcquireWriteLease(ctx, scopeB)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("second lease unexpectedly busy")
	}
	if snapshot, err := freshLease.Refresh(ctx); err != nil {
		t.Fatal(err)
	} else if snapshot.Generation != 2 {
		t.Fatalf("second refresh generation = %d, want 2", snapshot.Generation)
	}
	if _, err := freshLease.Publish(ctx, coord.Event{
		RootChanged:      &bucket.ObjectRef{BucketId: "bucket-b"},
		KeyPrefixChanged: []byte("k/"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := freshLease.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinatorMultiprocessWriteLeaseExcludesContenders(t *testing.T) {
	if role := os.Getenv(multiprocessLeaseRoleEnv); role != "" {
		runMultiprocessLeaseRole(t, role)
		return
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	heldPath := filepath.Join(dir, "held")
	releasePath := filepath.Join(dir, "release")

	if db, err := bdb.Open(dbPath, 0o600, nil); err != nil {
		t.Fatal(err)
	} else if err := db.Close(); err != nil {
		t.Fatal(err)
	}

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

	waitForFile(t, heldPath)

	if output, err := leaseRoleCommand(t, "contender-busy", dbPath, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-busy failed: %v\n%s", err, output)
	}

	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder failed: %v\n%s", err, holderOut.String())
	}

	if output, err := leaseRoleCommand(t, "contender-acquire", dbPath, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-acquire failed: %v\n%s", err, output)
	}
}

func openTestDB(t *testing.T) *bdb.DB {
	t.Helper()

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
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := bdb.Open(os.Getenv(multiprocessLeaseDBPathEnv), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: role,
	}

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
