//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris || windows

package filelock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

const (
	multiprocessRoleEnv    = "SPACEWAVE_COORD_FILELOCK_ROLE"
	multiprocessDirEnv     = "SPACEWAVE_COORD_FILELOCK_DIR"
	multiprocessHeldEnv    = "SPACEWAVE_COORD_FILELOCK_HELD_PATH"
	multiprocessReleaseEnv = "SPACEWAVE_COORD_FILELOCK_RELEASE_PATH"
)

func keyedScope(participant, key string) coord.Scope {
	return coord.Scope{
		VolumeID:      "volume-a",
		ParticipantID: participant,
		Key:           key,
	}
}

func TestKeyedLeaseNamespacesBackingStore(t *testing.T) {
	// Create file-lock coordinators for two distinct backing stores.
	ctx := context.Background()
	dir := t.TempDir()
	coordA := NewCoordinator(dir, filepath.Join(dir, "volume-a.db"), coord_inmem.NewCoordinator())
	coordB := NewCoordinator(dir, filepath.Join(dir, "volume-b.db"), coord_inmem.NewCoordinator())

	// Acquire the shared key in the first backing store.
	leaseA, ok, err := coordA.TryAcquireWriteLease(ctx, keyedScope("a", "shared-object"))
	if err != nil || !ok {
		t.Fatalf("acquire lease for volume A: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = leaseA.Release(ctx) })

	// Verify the same key can be leased independently in the second store.
	leaseB, ok, err := coordB.TryAcquireWriteLease(ctx, keyedScope("b", "shared-object"))
	if err != nil || !ok {
		t.Fatalf("acquire lease for volume B: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = leaseB.Release(ctx) })
}

func TestKeyedLeaseCanonicalizesBackingStorePath(t *testing.T) {
	// Create coordinators using relative and absolute paths to the same store.
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(dir)
	relativePath := "volume.db"
	absolutePath := filepath.Join(dir, relativePath)
	coordA := NewCoordinator(dir, relativePath, coord_inmem.NewCoordinator())
	coordB := NewCoordinator(dir, absolutePath, coord_inmem.NewCoordinator())

	// Acquire the key using the relative backing-store path.
	lease, ok, err := coordA.TryAcquireWriteLease(ctx, keyedScope("a", "shared-object"))
	if err != nil || !ok {
		t.Fatalf("acquire relative-path lease: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = lease.Release(ctx) })

	// Verify the absolute store path identifies the same held lease.
	if _, ok, err := coordB.TryAcquireWriteLease(ctx, keyedScope("b", "shared-object")); err != nil || ok {
		t.Fatalf("absolute-path lease = ok=%v err=%v, want held", ok, err)
	}
}

func TestKeyedLeaseExcludesAndReacquires(t *testing.T) {
	// Create two coordinators for keyed exclusion and reacquisition.
	ctx := context.Background()
	dir := t.TempDir()
	storeID := filepath.Join(dir, "volume.db")
	coordA := NewCoordinator(dir, storeID, coord_inmem.NewCoordinator())
	coordB := NewCoordinator(dir, storeID, coord_inmem.NewCoordinator())

	// Acquire the first keyed lease before testing competing handles.
	leaseA, ok, err := coordA.TryAcquireWriteLease(ctx, keyedScope("a", "world-1"))
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}

	// Verify the held key excludes both handles while another key remains available.
	if _, ok, err := coordB.TryAcquireWriteLease(ctx, keyedScope("b", "world-1")); err != nil || ok {
		t.Fatalf("second handle acquired held key: ok=%v err=%v", ok, err)
	}
	if _, ok, err := coordA.TryAcquireWriteLease(ctx, keyedScope("a2", "world-1")); err != nil || ok {
		t.Fatalf("same handle acquired held key: ok=%v err=%v", ok, err)
	}
	leaseOther, ok, err := coordB.TryAcquireWriteLease(ctx, keyedScope("b", "world-2"))
	if err != nil || !ok {
		t.Fatalf("distinct key acquire: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() { _ = leaseOther.Release(ctx) })

	// Start a contender waiting for the held keyed lease.
	waitErr := make(chan error, 1)
	go func() {
		lease, err := coordB.WaitAcquireWriteLease(ctx, keyedScope("b", "world-1"))
		if err == nil {
			err = lease.Release(ctx)
		}
		waitErr <- err
	}()

	// Release the held lease and verify the waiting contender acquires it.
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waitErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not acquire after release")
	}
}

func TestKeyedLeaseReleaseCompletesUnderCanceledContext(t *testing.T) {
	// Create a file-lock coordinator for release under cancellation.
	dir := t.TempDir()
	c := NewCoordinator(dir, filepath.Join(dir, "volume.db"), coord_inmem.NewCoordinator())

	// Acquire the lease whose cleanup must survive context cancellation.
	lease, ok, err := c.TryAcquireWriteLease(context.Background(), keyedScope("a", "world-1"))
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}

	// Release under a canceled context and verify clean lease completion.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lease.Release(canceled); err != nil {
		t.Fatalf("release under canceled context: %v", err)
	}
	select {
	case <-lease.Done():
	default:
		t.Fatal("Done not closed after release")
	}
	if err := lease.Err(); err != nil {
		t.Fatalf("Err after clean release = %v, want nil", err)
	}

	// Verify the released key can be acquired and released again.
	again, ok, err := c.TryAcquireWriteLease(context.Background(), keyedScope("a", "world-1"))
	if err != nil || !ok {
		t.Fatalf("reacquire after release: ok=%v err=%v", ok, err)
	}
	if err := again.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestKeyedWaitFollowsContextAndReleasesAbandonedLock checks that a wait
// blocked on another holder's file lock ends with its context, and that the
// abandoned wait gives the lock and keyed lease back once the holder unlocks.
func TestKeyedWaitFollowsContextAndReleasesAbandonedLock(t *testing.T) {
	// Hold the scope's lock file through a separate descriptor.
	dir := t.TempDir()
	c := NewCoordinator(dir, filepath.Join(dir, "volume.db"), coord_inmem.NewCoordinator())
	scope := keyedScope("a", "world-1")
	holder, err := c.openLockFile(t.Context(), scope)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if locked, err := tryLockFile(holder); err != nil || !locked {
		t.Fatalf("hold lock file: locked=%v err=%v", locked, err)
	}

	// Wait until the context ends while the holder keeps the lock.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.WaitAcquireWriteLease(ctx, scope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v, want deadline exceeded", err)
	}

	// Unlock and verify a new wait acquires the scope.
	if err := unlockFile(holder); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lease, err := c.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatalf("wait after unlock: %v", err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestKeyedCapabilityWithoutGenerations(t *testing.T) {
	// Create a keyed scope for file-lock capability checks.
	ctx := context.Background()
	dir := t.TempDir()
	c := NewCoordinator(dir, filepath.Join(dir, "volume.db"), coord_inmem.NewCoordinator())
	scope := keyedScope("a", "world-1")

	// Verify keyed leases expose the file-lock backend without generations.
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported || capability.Backend != coord.BackendKindFileLock {
		t.Fatalf("unexpected capability: %#v", capability)
	}
	if capability.Generations || capability.DetectsLoss {
		t.Fatalf("keyed capability declares generations or loss detection: %#v", capability)
	}

	// Verify keyed snapshots and watches report unsupported operations.
	if _, err := c.Snapshot(ctx, scope); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Snapshot error = %v, want ErrUnsupported", err)
	}
	if _, err := c.Watch(ctx, scope, 0); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Watch error = %v, want ErrUnsupported", err)
	}

	// Acquire a keyed lease and verify refresh and publication are unsupported.
	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer lease.Release(ctx)
	if _, err := lease.Refresh(ctx); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Refresh error = %v, want ErrUnsupported", err)
	}
	if _, err := lease.Publish(ctx, coord.Event{}); !errors.Is(err, coord.ErrUnsupported) {
		t.Fatalf("keyed Publish error = %v, want ErrUnsupported", err)
	}
}

func TestObjectStoreScopeDelegatesToInner(t *testing.T) {
	// Create an object-store scope backed by an inner memory coordinator.
	ctx := context.Background()
	dir := t.TempDir()
	inner := coord_inmem.NewCoordinator()
	c := NewCoordinator(dir, filepath.Join(dir, "volume.db"), inner)
	scope := coord.Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "a",
	}

	// Verify object-store capability delegates to the memory coordinator.
	capability, err := c.Capability(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if capability.Backend != coord.BackendKindInMemory || !capability.Generations {
		t.Fatalf("delegated capability = %#v", capability)
	}

	// Acquire the object-store scope through the file-lock coordinator.
	lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer lease.Release(ctx)

	// Verify the delegated lease excludes acquisition through the inner coordinator.
	if _, ok, err := inner.TryAcquireWriteLease(ctx, scope); err != nil || ok {
		t.Fatalf("inner acquired scope held through delegation: ok=%v err=%v", ok, err)
	}
}

func TestMultiprocessKeyedLeaseExcludesContenders(t *testing.T) {
	// Dispatch child processes to their keyed lease test roles.
	if role := os.Getenv(multiprocessRoleEnv); role != "" {
		runMultiprocessRole(t, role)
		return
	}

	// Prepare coordination files for the holder and contender processes.
	dir := t.TempDir()
	heldPath := filepath.Join(dir, "held")
	releasePath := filepath.Join(dir, "release")

	// Start the holder process and arrange cleanup for test failure.
	holder := roleCommand(t, "holder", dir, heldPath, releasePath)
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = holder.Process.Kill()
		_, _ = holder.Process.Wait()
	})

	// Wait for the holder process to publish its acquired lease.
	waitForFile(t, heldPath)

	// Verify a contender cannot acquire the key while the holder runs.
	if output, err := roleCommand(t, "contender-busy", dir, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-busy failed: %v\n%s", err, output)
	}

	// Kill the holder without releasing: the kernel drops the advisory lock
	// with the process.
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = holder.Process.Wait()

	// Verify the key becomes available after the holder process dies.
	if output, err := roleCommand(t, "contender-acquire", dir, heldPath, releasePath).CombinedOutput(); err != nil {
		t.Fatalf("contender-acquire failed: %v\n%s", err, output)
	}
}

func roleCommand(t *testing.T, role, dir, heldPath, releasePath string) *exec.Cmd {
	// Construct the keyed lease subprocess with its role and coordination paths.
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMultiprocessKeyedLeaseExcludesContenders$", "-test.v") //nolint:gosec
	cmd.Env = append(os.Environ(),
		multiprocessRoleEnv+"="+role,
		multiprocessDirEnv+"="+dir,
		multiprocessHeldEnv+"="+heldPath,
		multiprocessReleaseEnv+"="+releasePath,
	)
	return cmd
}

func runMultiprocessRole(t *testing.T, role string) {
	// Bound the keyed lease subprocess lifetime.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create the subprocess coordinator and its keyed lease scope.
	dir := os.Getenv(multiprocessDirEnv)
	c := NewCoordinator(dir, filepath.Join(dir, "volume.db"), coord_inmem.NewCoordinator())
	scope := keyedScope(role, "world-1")

	// Exercise the holder or contender contract for this subprocess.
	switch role {
	case "holder":
		_, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("holder lease unexpectedly busy")
		}
		if err := os.WriteFile(os.Getenv(multiprocessHeldEnv), []byte("held"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitForFile(t, os.Getenv(multiprocessReleaseEnv))
	case "contender-busy":
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			_ = lease.Release(ctx)
			t.Fatal("contender acquired keyed lease while holder process still owned it")
		}
	case "contender-acquire":
		lease, ok, err := c.TryAcquireWriteLease(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("contender lease busy after holder died")
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
