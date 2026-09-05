package bldr_plugin_compiler_go

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// TestGoModuleInputsTracked keeps dependency selection in the same input
// manifest that controls reuse of compiled plugin binaries.
func TestGoModuleInputsTracked(t *testing.T) {
	// Materialize the module inputs discovered by the native compiler.
	root := t.TempDir()
	want := []string{"go.mod", "go.sum", "vendor/modules.txt"}
	for _, name := range want {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("dependency selection\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Use the compiler's discovery and filtering path, not hand-built absolute
	// test inputs that bypass the source-relative discovery boundary.
	inputs := &builder.InputManifest{}
	if err := appendInputManifestFiles(inputs, root, InputFileKind_InputFileKind_GO, existingSourceFiles(root, want...)); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, file := range inputs.GetFiles() {
		got = append(got, filepath.ToSlash(file.GetPath()))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("module dependency inputs = %v, want %v", got, want)
	}
}
