package npm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/sirupsen/logrus"
)

func TestReadSiblingBunLock(t *testing.T) {
	// Write a package manifest without a sibling lock file.
	dir := t.TempDir()
	pkgPath := filepath.Join(dir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify the missing lock is reported as not found.
	if data, found, err := readSiblingBunLock(pkgPath); err != nil || found || data != nil {
		t.Fatalf("missing lock: data=%q found=%v err=%v", data, found, err)
	}

	// Write the sibling lock file and verify it is read back.
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
	// Verify the lockfile changes the install hash.
	pkg := []byte(`{"dependencies":{"react":"19.2.5"}}`)
	lockA := []byte("lock-a")
	lockB := []byte("lock-b")

	// Verify the package-only hash matches the plain digest.
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

// TestWithoutRootScripts proves the seeded manifest drops only the scripts.
func TestWithoutRootScripts(t *testing.T) {
	// Strip the scripts from a manifest that also has dependencies.
	pkg := []byte(`{"name":"app","scripts":{"prepare":"go mod vendor"},"dependencies":{"react":"19.2.5"}}`)
	got, err := withoutRootScripts(pkg)
	if err != nil {
		t.Fatal(err)
	}

	// Check the scripts are gone and the dependencies remain.
	var parser fastjson.Parser
	fields, err := parser.ParseBytes(got)
	if err != nil {
		t.Fatal(err)
	}
	if fields.Get("scripts") != nil {
		t.Fatalf("scripts kept in %s", got)
	}
	if fields.Get("dependencies") == nil {
		t.Fatalf("dependencies dropped from %s", got)
	}

	// Check a manifest without scripts is returned unchanged.
	plain := []byte(`{"dependencies":{}}`)
	if got, err := withoutRootScripts(plain); err != nil || string(got) != string(plain) {
		t.Fatalf("manifest without scripts changed: %s, %v", got, err)
	}
}

func TestBunMinimumReleaseAgeArg(t *testing.T) {
	// Verify the empty environment falls back to zero.
	t.Setenv("BLDR_BUN_MINIMUM_RELEASE_AGE", "")
	got := bunMinimumReleaseAgeArg()
	if len(got) != 1 || got[0] != "--minimum-release-age=0" {
		t.Fatalf("empty minimum age arg = %#v, want --minimum-release-age=0", got)
	}

	// Verify the configured age is passed through.
	t.Setenv("BLDR_BUN_MINIMUM_RELEASE_AGE", "7d")
	got = bunMinimumReleaseAgeArg()
	if len(got) != 1 || got[0] != "--minimum-release-age=7d" {
		t.Fatalf("minimum age arg = %#v, want --minimum-release-age=7d", got)
	}
}

func TestSharedInstallDir(t *testing.T) {
	// Configure a usable shared cache root.
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", root)

	// Verify the install dir resolves under the cache root.
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
	// Write a source manifest and configure a shared cache root.
	le := logrus.NewEntry(logrus.New())

	// Write the source package manifest into its own directory.
	pkgDir := t.TempDir()
	srcPackageJson := filepath.Join(pkgDir, "package.json")
	pkgManifest := []byte(`{"dependencies":{}}`)
	if err := os.WriteFile(srcPackageJson, pkgManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", cacheRoot)

	// Install once and verify the shared cache directory is used.
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
	// Hold the target lock with the first mutation in the background.
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

	// Start the second mutation while the first holds the lock.
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- withInstallLock(t.Context(), targetDir, func() error {
			close(secondEntered)
			return nil
		})
	}()

	// Verify the second mutation waits and then acquires the released lock.
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

	// Verify a canceled context refuses to run the mutation.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := withInstallLock(ctx, targetDir, func() error {
		t.Fatal("canceled lock executed target mutation")
		return nil
	}); err == nil {
		t.Fatal("canceled lock returned no error")
	}
}
