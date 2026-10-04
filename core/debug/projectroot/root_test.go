package projectroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFromDirFindsBldrStar(t *testing.T) {
	// Create a Bldr project marker and a nested debug directory.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bldr.star"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "app", "debug")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	// Find the nearest project root from the nested debug path.
	got, err := FindFromDir(child, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("expected %s, got %s", root, got)
	}
}

func TestFindFromDirFindsBldrYaml(t *testing.T) {
	// Create a Bldr YAML project marker and a nested command directory.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bldr.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "cmd", "spacewave-debug")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	// Find the project root from the nested debug command path.
	got, err := FindFromDir(child, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("expected %s, got %s", root, got)
	}
}

func TestFindFromDirMissingRoot(t *testing.T) {
	root := t.TempDir()
	_, err := FindFromDir(root, 2)
	if err == nil {
		t.Fatal("expected error")
	}
}
