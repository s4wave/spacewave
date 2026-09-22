//go:build !js

package bldr_plugin_compiler_go

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestModuleFilesRejectEscapingLinks preserves files outside the generated directory.
func TestModuleFilesRejectEscapingLinks(t *testing.T) {
	for _, name := range []string{"go.mod", "go.sum"} {
		t.Run(name, func(t *testing.T) {
			// A stale generated-file link must not redirect a build into another file.
			source := t.TempDir()
			generated := t.TempDir()
			outside := filepath.Join(t.TempDir(), "retained.txt")
			writeFile(t, source, "go.mod", "module example.com/source\n\ngo 1.27.0\n")
			writeFile(t, source, "go.sum", "source checksums\n")
			if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(generated, name)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			// Module generation rejects the escaping write and retains the linked file.
			compiler, err := NewModuleCompiler(logrus.NewEntry(logrus.New()), generated, "example")
			if err != nil {
				t.Fatal(err)
			}
			if err := compiler.writeModuleFiles(&Analysis{workDir: source}); err == nil {
				t.Fatal("accepted a generated module link outside its directory")
			}
			data, err := os.ReadFile(outside)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "keep" {
				t.Fatalf("changed the linked file: %q", data)
			}
		})
	}
}
