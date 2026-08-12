//go:build !js

package web_pkg_vite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedSourceRootUsesFilesystemIdentity(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(t.TempDir(), "case-equivalent-state")
	if err := os.Symlink(rootPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(aliasPath, "build", "input.js")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := newManagedSourceRoot("", rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Contains(sourcePath) {
		t.Fatalf("filesystem-equivalent source %q is outside %q", sourcePath, rootPath)
	}
}

func TestManagedSourceRootContainsNonexistentLexicalDescendant(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "state")
	root, err := newManagedSourceRoot("", rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if !root.Contains(filepath.Join(rootPath, "build", "missing.js")) {
		t.Fatal("nonexistent lexical descendant is outside managed root")
	}
	if root.Contains(filepath.Join(filepath.Dir(rootPath), "state-other", "missing.js")) {
		t.Fatal("sibling with matching prefix is inside managed root")
	}
}
