//go:build !js

package bldr_cli_compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestAnalyzeCliImports preserves signature validation for functions and inferred values.
func TestAnalyzeCliImports(t *testing.T) {
	// Build small independent packages so invalid signatures remain legal Go inputs.
	root := t.TempDir()
	writeCliFixture(t, root, "go.mod", "module github.com/s4wave/spacewave\n\ngo 1.26.2\n")
	writeCliFixture(t, root, "bldr/web/bundler/output.go", "package bundler\ntype WebBundlerOutput struct{}\n")
	tests := []struct {
		// name identifies the command package.
		name string
		// source defines the command builder declaration.
		source string
		// wantError is the existing diagnostic for an invalid builder.
		wantError string
	}{
		{name: "ordinary", source: "func NewCliCommands(int) {}"},
		{name: "inferred", source: "var NewCliCommands = func(int) {}"},
		{name: "alias", source: "type Builder = func(int)\nvar NewCliCommands Builder", wantError: "is not a function"},
		{name: "variadic", source: "func NewCliCommands(...int) {}"},
		{name: "grouped", source: "func NewCliCommands(a, b int) {}", wantError: "must take only getBus"},
		{name: "empty", source: "func NewCliCommands() {}", wantError: "must take only getBus"},
		{name: "object", source: "var NewCliCommands = 1", wantError: "is not a function"},
		{name: "method", source: "type T struct{}\nfunc (T) NewCliCommands(int) {}", wantError: "does not export NewCliCommands"},
	}
	for _, tc := range tests {
		writeCliFixture(t, root, tc.name+"/cli.go", "package "+tc.name+"\n"+tc.source+"\n")
	}

	// Exercise the CLI adapter rather than duplicating its validation in the test.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkgPath := "./" + tc.name
			imports, err := AnalyzeCliImports(t.Context(), logrus.NewEntry(logrus.New()), root, []string{pkgPath}, "linux", "amd64")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("got %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if imports[pkgPath].Alias != tc.name {
				t.Fatalf("import alias: got %q, want %q", imports[pkgPath].Alias, tc.name)
			}
		})
	}
}

// writeCliFixture creates a module fixture file under the test's temporary directory.
func writeCliFixture(t *testing.T, root, name, source string) {
	t.Helper()

	// Create parent directories before writing the complete source file.
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
