//go:build !js

package wasm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBldrDependencyUsesExplicitSpacewaveRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repoRoot, "go.mod"),
		[]byte("module github.com/s4wave/spacewave\n\ngo 1.24\n"),
		0o644,
	); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	version, sum, srcPath, err := resolveBldrDependency(repoRoot)
	if err != nil {
		t.Fatalf("resolveBldrDependency: %v", err)
	}
	if version != "" {
		t.Fatalf("expected local source dependency, got version %q", version)
	}
	if sum != "" {
		t.Fatalf("expected empty sum for local source dependency, got %q", sum)
	}
	if srcPath != repoRoot {
		t.Fatalf("expected explicit repo root %q, got %q", repoRoot, srcPath)
	}
}
