//go:build !js

package releasewasm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/e2e/releasewasm/artifact"
)

func TestPrebuiltReleaseWasmDistDirsRejectsIdentitylessArtifact(t *testing.T) {
	repoRoot := t.TempDir()
	distDir := filepath.Join(repoRoot, ".tmp", "release-dist")
	prerenderDir := filepath.Join(repoRoot, ".tmp", "prerender")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(prerenderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "browser-release.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prerenderDir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(releaseWasmDistDirEnv, ".tmp/release-dist")
	t.Setenv(releaseWasmPrerenderDistEnv, ".tmp/prerender")

	dirs, ok, err := prebuiltReleaseWasmDistDirs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("prebuilt release-wasm dist dirs were not selected")
	}
	if dirs.releaseDist != distDir {
		t.Fatalf("release dist = %q, want %q", dirs.releaseDist, distDir)
	}
	if dirs.prerender != prerenderDir {
		t.Fatalf("prerender dist = %q, want %q", dirs.prerender, prerenderDir)
	}
	if err := artifact.Validate(dirs.releaseDist, dirs.prerender, &artifact.Identity{}); err == nil {
		t.Fatal("identityless prebuilt release-wasm artifact validated")
	}
}

func TestPrebuiltReleaseWasmDistDirsRequiresBothDirs(t *testing.T) {
	t.Setenv(releaseWasmDistDirEnv, t.TempDir())
	t.Setenv(releaseWasmPrerenderDistEnv, "")

	if _, _, err := prebuiltReleaseWasmDistDirs(t.TempDir()); err == nil {
		t.Fatal("expected partial prebuilt release-wasm dist env to fail")
	}
}
