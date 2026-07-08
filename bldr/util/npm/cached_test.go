package npm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureNodeModulesLinkCreatesSymlinkToInstallNodeModules(t *testing.T) {
	parentDir := t.TempDir()
	installDir := t.TempDir()
	installNodeModules := filepath.Join(installDir, "node_modules")
	if err := os.Mkdir(installNodeModules, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := EnsureNodeModulesLink(parentDir, installDir); err != nil {
		t.Fatal(err)
	}

	assertNodeModulesSymlinkTarget(t, parentDir, installNodeModules)
}

func TestEnsureNodeModulesLinkToleratesExistingSymlinkToInstallNodeModules(t *testing.T) {
	parentDir := t.TempDir()
	installDir := t.TempDir()
	installNodeModules := filepath.Join(installDir, "node_modules")
	if err := os.Mkdir(installNodeModules, 0o755); err != nil {
		t.Fatal(err)
	}

	target, err := filepath.Abs(installNodeModules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(parentDir, "node_modules")); err != nil {
		t.Fatal(err)
	}

	if err := EnsureNodeModulesLink(parentDir, installDir); err != nil {
		t.Fatal(err)
	}

	assertNodeModulesSymlinkTarget(t, parentDir, installNodeModules)
}

func assertNodeModulesSymlinkTarget(t *testing.T, parentDir, installNodeModules string) {
	t.Helper()

	linkPath := filepath.Join(parentDir, "node_modules")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is mode %v, want symlink", linkPath, info.Mode())
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(parentDir, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(installNodeModules)
	if err != nil {
		t.Fatal(err)
	}
	if target != want {
		t.Fatalf("node_modules symlink target=%q want %q", target, want)
	}
}

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
	if got := bunMinimumReleaseAgeArg(); got != nil {
		t.Fatalf("empty minimum age arg = %#v, want nil", got)
	}

	t.Setenv("BLDR_BUN_MINIMUM_RELEASE_AGE", "0")
	got := bunMinimumReleaseAgeArg()
	if len(got) != 1 || got[0] != "--minimum-release-age=0" {
		t.Fatalf("minimum age arg = %#v, want --minimum-release-age=0", got)
	}
}
