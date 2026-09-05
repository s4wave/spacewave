//go:build !js

package bldr_plugin_compiler_go

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestStartupInputsExcludeGeneratedTree preserves real runtime dependencies
// while allowing generated modules to disappear after bundling.
func TestStartupInputsExcludeGeneratedTree(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, ".bldr-dist", "build", "js", "launcher")
	if err := os.MkdirAll(generated, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "bldr", "web", "runtime.ts")
	override := filepath.Join(root, "gs", "encoding", "binary", "index.ts")
	sibling := generated + "-source.ts"
	inputs := []string{source, filepath.Join(generated, "dist", "@goscript", "encoding", "binary", "index.ts"), filepath.Join(generated, "codegen", "entrypoint.ts"), override, sibling}
	got := filterPathsUnderBase(root, inputs, generated)
	want := []string{source, override, sibling}
	if !slices.Equal(got, want) {
		t.Fatalf("startup inputs = %v, want %v", got, want)
	}
}
