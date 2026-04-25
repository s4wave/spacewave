//go:build !js

package wasm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCompileTestScripts validates the esbuild compilation pipeline produces
// ESM modules from TS fixtures.
func TestCompileTestScripts(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "e2e")
	scripts, err := CompileTestScripts(".", outDir)
	if err != nil {
		t.Fatalf("CompileTestScripts: %v", err)
	}

	url, ok := scripts["compile-test-fixture.ts"]
	if !ok {
		t.Fatalf("expected compile-test-fixture.ts in compiled scripts, got keys: %v", scriptKeys(scripts))
	}

	if url != "/e2e/compile-test-fixture.mjs" {
		t.Fatalf("expected /e2e/compile-test-fixture.mjs, got %s", url)
	}

	outPath := filepath.Join(outDir, "compile-test-fixture.mjs")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read compiled output: %v", err)
	}

	js := string(data)
	if len(js) == 0 {
		t.Fatal("expected non-empty compiled output")
	}

	t.Logf("compiled %d scripts, fixture is %d bytes", len(scripts), len(js))
}

func scriptKeys(m CompiledScripts) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
