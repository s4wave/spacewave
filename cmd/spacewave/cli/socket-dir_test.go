//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"testing"
)

// shortSocketDir returns a fresh, symlink-resolved worktree-local directory
// short enough for Darwin's 104-byte Unix socket path limit.
func shortSocketDir(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "sw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
