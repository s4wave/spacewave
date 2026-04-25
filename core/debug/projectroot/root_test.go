package projectroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFromDirFindsBldrStar(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bldr.star"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "app", "debug")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindFromDir(child, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("expected %s, got %s", root, got)
	}
}

func TestFindFromDirFindsBldrYaml(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bldr.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "cmd", "alpha-debug")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

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
