//go:build !js

package wasm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/testbed/reporoot"
)

// TestCompileTestScripts checks that the bundler emits ESM from TypeScript fixtures.
func TestCompileTestScripts(t *testing.T) {
	// Compile the package fixtures with source aliases from the module root.
	root := reporoot.Must(t)
	outDir := filepath.Join(t.TempDir(), "e2e")
	scripts, err := CompileTestScriptsFor(".", outDir, root, filepath.Join(root, "vendor"))
	if err != nil {
		t.Fatalf("CompileTestScriptsFor: %v", err)
	}

	// Require the fixture script URL.
	url, ok := scripts["compile-test-fixture.ts"]
	if !ok {
		t.Fatalf("expected compile-test-fixture.ts in compiled scripts, got keys: %v", scriptKeys(scripts))
	}

	// Require the served fixture path.
	if url != "/e2e/compile-test-fixture.mjs" {
		t.Fatalf("expected /e2e/compile-test-fixture.mjs, got %s", url)
	}

	// Read the compiled fixture.
	outPath := filepath.Join(outDir, "compile-test-fixture.mjs")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read compiled output: %v", err)
	}

	// Require a non-empty compiled script.
	js := string(data)
	if len(js) == 0 {
		t.Fatal("expected non-empty compiled output")
	}

	// Log the compiled fixture size.
	t.Logf("compiled %d scripts, fixture is %d bytes", len(scripts), len(js))
}

// scriptKeys returns the names of the compiled fixture scripts.
func scriptKeys(m CompiledScripts) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
