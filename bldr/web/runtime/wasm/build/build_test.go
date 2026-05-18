//go:build !js

package web_runtime_wasm_build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildInputPathsFromMetafileResolvesExistingInputs(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "plugin-wasm.ts"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	nestedDir := filepath.Join(workDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nestedDir, "dep.ts")
	if err := os.WriteFile(nestedFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := buildInputPathsFromMetafile(workDir, `{
		"inputs": {
			"plugin-wasm.ts": {},
			"nested/dep.ts": {},
			"<runtime>": {},
			"missing.ts": {}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(workDir, "nested", "dep.ts"),
		filepath.Join(workDir, "plugin-wasm.ts"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths len = %d want %d: %v", len(paths), len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q want %q", i, paths[i], want[i])
		}
	}
}
