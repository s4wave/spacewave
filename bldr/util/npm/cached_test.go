package npm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestReadSiblingBunLock(t *testing.T) {
	dir := t.TempDir()
	pkgPath := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if data, found, err := readSiblingBunLock(pkgPath); err != nil || found || data != nil {
		t.Fatalf("missing lock: data=%q found=%v err=%v", data, found, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "bun.lock"), []byte("lock-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, found, err := readSiblingBunLock(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected sibling bun.lock to be found")
	}
	if string(data) != "lock-data" {
		t.Fatalf("lock data=%q want lock-data", data)
	}
}

func TestBunInstallHashIncludesLockfile(t *testing.T) {
	pkg := []byte(`{"dependencies":{"react":"19.2.5"}}`)
	lockA := []byte("lock-a")
	lockB := []byte("lock-b")

	if got, want := bunInstallHash(pkg, nil), sha256Hex(pkg); got != want {
		t.Fatalf("package-only hash=%q want %q", got, want)
	}
	if bunInstallHash(pkg, lockA) == sha256Hex(pkg) {
		t.Fatal("lockfile hash matched package-only hash")
	}
	if bunInstallHash(pkg, lockA) == bunInstallHash(pkg, lockB) {
		t.Fatal("different lockfiles produced the same install hash")
	}
}

func TestBunMinimumReleaseAgeArg(t *testing.T) {
	t.Setenv("BLDR_BUN_MINIMUM_RELEASE_AGE", "")
	got := bunMinimumReleaseAgeArg()
	if len(got) != 1 || got[0] != "--minimum-release-age=0" {
		t.Fatalf("empty minimum age arg = %#v, want --minimum-release-age=0", got)
	}

	t.Setenv("BLDR_BUN_MINIMUM_RELEASE_AGE", "7d")
	got = bunMinimumReleaseAgeArg()
	if len(got) != 1 || got[0] != "--minimum-release-age=7d" {
		t.Fatalf("minimum age arg = %#v, want --minimum-release-age=7d", got)
	}
}

func TestSharedInstallDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", root)

	dir, ok := sharedInstallDir("abc123")
	if !ok || dir != filepath.Join(root, "abc123") {
		t.Fatalf("sharedInstallDir=%q ok=%v, want %q", dir, ok, filepath.Join(root, "abc123"))
	}

	// A cache root that cannot be created disables the shared cache.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", filepath.Join(blocker, "nested"))
	if _, ok := sharedInstallDir("abc123"); ok {
		t.Fatal("expected sharedInstallDir to report false for an unusable cache root")
	}
}

func TestEnsureSharedBunInstallSharesCache(t *testing.T) {
	le := logrus.NewEntry(logrus.New())

	pkgDir := t.TempDir()
	srcPackageJson := filepath.Join(pkgDir, "package.json")
	pkgManifest := []byte(`{"dependencies":{}}`)
	if err := os.WriteFile(srcPackageJson, pkgManifest, 0o644); err != nil {
		t.Fatal(err)
	}

	cacheRoot := filepath.Join(t.TempDir(), "cache")
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", cacheRoot)

	fallbackA := filepath.Join(t.TempDir(), "a")
	dirA, err := EnsureSharedBunInstall(t.Context(), le, pkgDir, srcPackageJson, fallbackA)
	if err != nil {
		t.Fatal(err)
	}
	if dirA == fallbackA || filepath.Dir(dirA) != cacheRoot {
		t.Fatalf("install root=%q, want a shared cache dir under %q", dirA, cacheRoot)
	}
	if data, err := os.ReadFile(filepath.Join(dirA, ".bldr-install-hash")); err != nil || string(data) != sha256Hex(pkgManifest) {
		t.Fatalf("install hash=%q err=%v, want %q", data, err, sha256Hex(pkgManifest))
	}

	// A second install of the same manifest reuses the shared directory.
	fallbackB := filepath.Join(t.TempDir(), "b")
	dirB, err := EnsureSharedBunInstall(t.Context(), le, pkgDir, srcPackageJson, fallbackB)
	if err != nil {
		t.Fatal(err)
	}
	if dirB != dirA {
		t.Fatalf("second install root=%q, want the shared %q", dirB, dirA)
	}

	// An unusable shared cache root falls back to the state directory.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", filepath.Join(blocker, "nested"))
	fallbackC := filepath.Join(t.TempDir(), "c")
	dirC, err := EnsureSharedBunInstall(t.Context(), le, pkgDir, srcPackageJson, fallbackC)
	if err != nil {
		t.Fatal(err)
	}
	if dirC != fallbackC {
		t.Fatalf("fallback install root=%q, want %q", dirC, fallbackC)
	}
	if _, err := os.Stat(filepath.Join(fallbackC, "node_modules")); err != nil {
		t.Fatalf("fallback node_modules missing: %v", err)
	}
}

func TestWithInstallLockSerializesTargetMutation(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "deps")
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- withInstallLock(t.Context(), targetDir, func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- withInstallLock(t.Context(), targetDir, func() error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("second target mutation entered while first held the lock")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second target mutation did not acquire the released lock")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := withInstallLock(ctx, targetDir, func() error {
		t.Fatal("canceled lock executed target mutation")
		return nil
	}); err == nil {
		t.Fatal("canceled lock returned no error")
	}
}
