package unixfs_checkout

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPublishPreservesPrevious tries to replace a complete tree with missing
// staging, then a complete replacement, through real directory renames.
func TestPublishPreservesPrevious(t *testing.T) {
	// Keep an old checkout whose bytes must survive a failed publication.
	root := t.TempDir()
	destination := filepath.Join(root, "release")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "data"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publish(filepath.Join(root, "missing"), destination); err == nil {
		t.Fatal("missing staging published")
	}
	if data, err := os.ReadFile(filepath.Join(destination, "data")); err != nil || string(data) != "old" {
		t.Fatal("failed publication lost the previous checkout", string(data), err)
	}

	// Install complete staging, retaining a complete tree at every successful return.
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "data"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publish(staging, destination); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "data")); err != nil || string(data) != "new" {
		t.Fatal(string(data), err)
	}
}
