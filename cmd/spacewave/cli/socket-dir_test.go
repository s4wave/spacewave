//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"testing"
)

// shortSocketDir returns a fresh, symlink-resolved directory short enough to
// hold Unix sockets. Darwin limits sun_path to 104 bytes, which t.TempDir and
// checkout-relative paths exceed once the test name or checkout path is long.
func shortSocketDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "sw-")
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
