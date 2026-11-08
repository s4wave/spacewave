//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"testing"
)

// spacewaveReleaseLibrary is the release library Spacewave's bldr.star loads.
const spacewaveReleaseLibrary = "core/release/publisher/release.star"

// writeSpacewaveProject writes Spacewave's bldr.star followed by overlay into
// a temporary project beside the release library it loads, and returns the
// path of the written bldr.star.
func writeSpacewaveProject(t *testing.T, overlay []byte) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"bldr.star", spacewaveReleaseLibrary} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if name == "bldr.star" {
			data = append(append(data, '\n'), overlay...)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "bldr.star")
}
