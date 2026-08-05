package bldr_plugin_compiler_js

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativeEntrypointOutputPathPreservesAbsoluteOutput(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "source")
	outputRoot := filepath.Join(t.TempDir(), "dist")
	outputPath := filepath.Join(outputRoot, "plugin-HASH.mjs")

	relFromSource, err := filepath.Rel(sourceRoot, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if relFromSource == "." || !strings.HasPrefix(relFromSource, ".."+string(filepath.Separator)) {
		t.Fatalf("test output path %q is not outside source root %q", outputPath, sourceRoot)
	}

	got, err := relativeEntrypointOutputPath(sourceRoot, outputRoot, outputPath)
	if err != nil {
		t.Fatalf("relativeEntrypointOutputPath() error = %v", err)
	}
	const want = "plugin-HASH.mjs"
	if got != want {
		t.Fatalf("relativeEntrypointOutputPath(%q, %q) = %q, want %q", outputRoot, outputPath, got, want)
	}
}

func TestRelativeEntrypointOutputPathResolvesRelativeOutputFromWorkingDir(t *testing.T) {
	baseDir := t.TempDir()
	workingRoot := filepath.Join(baseDir, "repo", "source", "root")
	outputRoot := filepath.Join(baseDir, "dist")
	outputPath := filepath.Join(outputRoot, "plugin-HASH.mjs")

	outputFromWorkingRoot, err := filepath.Rel(workingRoot, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := relativeEntrypointOutputPath(workingRoot, outputRoot, outputFromWorkingRoot)
	if err != nil {
		t.Fatalf("relativeEntrypointOutputPath() error = %v", err)
	}
	const want = "plugin-HASH.mjs"
	if got != want {
		t.Fatalf("relativeEntrypointOutputPath(%q, %q) = %q, want %q", outputRoot, outputFromWorkingRoot, got, want)
	}
}
