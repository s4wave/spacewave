//go:build !skip_e2e && !js

package wasm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	bldr_statepath "github.com/s4wave/spacewave/bldr/statepath"
)

const harnessStateRootDeadPID = 999999999

func TestHarnessStateRootOwnerMarkerRoundTrip(t *testing.T) {
	stateRoot := t.TempDir()
	owner := harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Unix(1700000000, 123).UnixNano(),
		token:           "0123456789abcdef0123456789abcdef",
	}

	if err := writeHarnessStateRootOwner(stateRoot, owner); err != nil {
		t.Fatal(err)
	}
	assertHarnessStateRootPathExists(t, filepath.Join(stateRoot, harnessStateRootOwnerName))

	got, err := readHarnessStateRootOwner(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got != owner {
		t.Fatalf("owner marker round trip = %+v, want %+v", got, owner)
	}
}

func TestHarnessStateRootConcurrentOwnersUseDistinctRoots(t *testing.T) {
	repoRoot := t.TempDir()
	stableStateRoot, err := buildHarnessStateRoot(repoRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	privateStateRoot, err := buildHarnessStateRoot(repoRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	if stableStateRoot == privateStateRoot {
		t.Fatalf("stable and private state roots both resolved to %s", stableStateRoot)
	}

	stableLock := acquireTestHarnessStateRootLock(t, stableStateRoot)
	t.Cleanup(func() {
		closeTestHarnessStateRootLock(t, stableLock)
	})
	assertHarnessStateRootPathMissing(t, filepath.Join(stableStateRoot, harnessStateRootOwnerName))

	stableSentinel := filepath.Join(stableStateRoot, "src", "live")
	if err := os.MkdirAll(filepath.Dir(stableSentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableSentinel, []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}

	contendedLock, acquired, err := acquireHarnessStateRootLock(stableStateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		closeTestHarnessStateRootLock(t, contendedLock)
		t.Fatal("second owner acquired the live stable state root")
	}

	privateLock := acquireTestHarnessStateRootLock(t, privateStateRoot)
	privateOwner, err := newHarnessStateRootOwner()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeHarnessStateRootOwner(privateStateRoot, privateOwner); err != nil {
		t.Fatal(err)
	}
	privateSentinel := filepath.Join(privateStateRoot, "src", "private")
	if err := os.MkdirAll(filepath.Dir(privateSentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privateSentinel, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	(&Harness{
		stateRoot:                 privateStateRoot,
		preserveStartupBuildCache: false,
		stateRootOwner:            privateOwner,
		stateRootLock:             privateLock,
	}).Release()

	assertHarnessStateRootPathMissing(t, privateStateRoot)
	assertHarnessStateRootPathExists(t, stableSentinel)
	assertHarnessStateRootPathMissing(t, filepath.Join(stableStateRoot, harnessStateRootOwnerName))
}

func TestHarnessStateRootSerialOwnersReuseStableRoot(t *testing.T) {
	repoRoot := t.TempDir()
	stableStateRoot, err := buildHarnessStateRoot(repoRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	firstLock := acquireTestHarnessStateRootLock(t, stableStateRoot)
	assertHarnessStateRootPathMissing(t, filepath.Join(stableStateRoot, harnessStateRootOwnerName))

	durable := filepath.Join(stableStateRoot, "build", "cached")
	transient := filepath.Join(stableStateRoot, "src", "stale")
	for _, path := range []string{durable, transient} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	closeTestHarnessStateRootLock(t, firstLock)

	secondLock := acquireTestHarnessStateRootLock(t, stableStateRoot)
	if err := bldr_statepath.ClearBuildState(stableStateRoot, true); err != nil {
		t.Fatal(err)
	}
	assertHarnessStateRootPathExists(t, durable)
	assertHarnessStateRootPathMissing(t, transient)
	assertHarnessStateRootPathExists(t, filepath.Join(stableStateRoot, harnessStateRootLockName))

	(&Harness{
		stateRoot:                 stableStateRoot,
		preserveStartupBuildCache: true,
		stateRootLock:             secondLock,
	}).Release()

	assertHarnessStateRootPathExists(t, stableStateRoot)
	assertHarnessStateRootPathMissing(t, filepath.Join(stableStateRoot, harnessStateRootOwnerName))
	thirdLock := acquireTestHarnessStateRootLock(t, stableStateRoot)
	closeTestHarnessStateRootLock(t, thirdLock)
}

func TestHarnessStateRootReleaseStopsConsumersBeforeUnlock(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "wasm-stable")
	lock := acquireTestHarnessStateRootLock(t, stateRoot)
	h := &Harness{
		stateRoot:                 stateRoot,
		preserveStartupBuildCache: true,
		stateRootLock:             lock,
		cloudEndpointClose: func() {
			consumerLock, acquired, err := acquireHarnessStateRootLock(stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if acquired {
				closeTestHarnessStateRootLock(t, consumerLock)
				t.Fatal("state root unlocked before consumer cleanup")
			}
		},
	}

	h.Release()

	reacquiredLock := acquireTestHarnessStateRootLock(t, stateRoot)
	closeTestHarnessStateRootLock(t, reacquiredLock)
}

func TestReapHarnessCacheOffStateRootsDeletesDeadPIDMarker(t *testing.T) {
	parent := t.TempDir()
	stateRoot := makeHarnessStateRootDir(t, parent, "wasm-00000001")
	owner := harnessStateRootOwner{
		pid:             harnessStateRootDeadPID,
		createdUnixNano: time.Now().Add(-time.Hour).UnixNano(),
		token:           "11111111111111111111111111111111",
	}
	if err := writeHarnessStateRootOwner(stateRoot, owner); err != nil {
		t.Fatal(err)
	}

	reapHarnessCacheOffStateRoots(nil, parent, filepath.Join(parent, "wasm-00000002"), filepath.Join(parent, "wasm-00000003"), harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().UnixNano(),
		token:           "22222222222222222222222222222222",
	})

	assertHarnessStateRootPathMissing(t, stateRoot)
}

func TestReapHarnessCacheOffStateRootsPreservesLivePIDMarker(t *testing.T) {
	parent := t.TempDir()
	stateRoot := makeHarnessStateRootDir(t, parent, "wasm-00000004")
	owner := harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().Add(-time.Hour).UnixNano(),
		token:           "33333333333333333333333333333333",
	}
	if err := writeHarnessStateRootOwner(stateRoot, owner); err != nil {
		t.Fatal(err)
	}

	reapHarnessCacheOffStateRoots(nil, parent, filepath.Join(parent, "wasm-00000005"), filepath.Join(parent, "wasm-00000006"), harnessStateRootOwner{
		pid:             harnessStateRootDeadPID,
		createdUnixNano: time.Now().UnixNano(),
		token:           "44444444444444444444444444444444",
	})

	assertHarnessStateRootPathExists(t, stateRoot)
}

func TestReapHarnessCacheOffStateRootsPreservesStableStateRoot(t *testing.T) {
	parent := t.TempDir()
	stableStateRoot := makeHarnessStateRootDir(t, parent, "wasm-0badcafe")
	old := time.Now().Add(-(harnessMarkerlessStateRootMaxAge + time.Hour))
	if err := os.Chtimes(stableStateRoot, old, old); err != nil {
		t.Fatal(err)
	}

	reapHarnessCacheOffStateRoots(nil, parent, filepath.Join(parent, "wasm-00000007"), stableStateRoot, harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().UnixNano(),
		token:           "55555555555555555555555555555555",
	})

	assertHarnessStateRootPathExists(t, stableStateRoot)
}

func TestReapHarnessCacheOffStateRootsDeletesOldMarkerlessStateRoot(t *testing.T) {
	parent := t.TempDir()
	stateRoot := makeHarnessStateRootDir(t, parent, "wasm-00000008")
	old := time.Now().Add(-(harnessMarkerlessStateRootMaxAge + time.Hour))
	if err := os.Chtimes(stateRoot, old, old); err != nil {
		t.Fatal(err)
	}

	reapHarnessCacheOffStateRoots(nil, parent, filepath.Join(parent, "wasm-00000009"), filepath.Join(parent, "wasm-0000000a"), harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().UnixNano(),
		token:           "66666666666666666666666666666666",
	})

	assertHarnessStateRootPathMissing(t, stateRoot)
}

func TestReapHarnessCacheOffStateRootsPreservesYoungMarkerlessStateRoot(t *testing.T) {
	parent := t.TempDir()
	stateRoot := makeHarnessStateRootDir(t, parent, "wasm-0000000b")
	young := time.Now().Add(-time.Minute)
	if err := os.Chtimes(stateRoot, young, young); err != nil {
		t.Fatal(err)
	}

	reapHarnessCacheOffStateRoots(nil, parent, filepath.Join(parent, "wasm-0000000c"), filepath.Join(parent, "wasm-0000000d"), harnessStateRootOwner{
		pid:             os.Getpid(),
		createdUnixNano: time.Now().UnixNano(),
		token:           "77777777777777777777777777777777",
	})

	assertHarnessStateRootPathExists(t, stateRoot)
}

func acquireTestHarnessStateRootLock(t *testing.T, stateRoot string) *os.File {
	t.Helper()
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	lock, acquired, err := acquireHarnessStateRootLock(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatalf("state root lock is contended: %s", stateRoot)
	}
	return lock
}

func closeTestHarnessStateRootLock(t *testing.T, lock *os.File) {
	t.Helper()
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

func makeHarnessStateRootDir(t *testing.T, parent, name string) string {
	t.Helper()
	stateRoot := filepath.Join(parent, name)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return stateRoot
}

func assertHarnessStateRootPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertHarnessStateRootPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, err=%v", path, err)
	}
}
