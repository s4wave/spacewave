//go:build !js

package bldr

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

// TestDistDepsMatchRoot checks that the dist deps pin every dependency and
// override the root package.json also names at the root's version, as
// bldr:gen:deps writes them. A mismatch serves the dev web packages at another
// version than the generated code expects.
func TestDistDepsMatchRoot(t *testing.T) {
	// Read the root and the dist deps package files.
	read := func(path string) *fastjson.Value {
		// Parse the file whole.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := fastjson.ParseBytes(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return pkg
	}
	root := read("../package.json")
	dist := read("dist/deps/package.json")

	// Compare each dist pin the root also names.
	check := func(field string) {
		dist.GetObject(field).Visit(func(name []byte, version *fastjson.Value) {
			want := root.Get(field, string(name))
			if want != nil && want.String() != version.String() {
				t.Errorf("dist deps %s %s = %s, root = %s; run bun run bldr:gen:deps", field, name, version, want)
			}
		})
	}
	check("dependencies")
	check("overrides")
	check("patchedDependencies")

	// Both compiler installations must apply the same patch bytes to their pinned version.
	dist.GetObject("patchedDependencies").Visit(func(name []byte, patch *fastjson.Value) {
		// Resolve each package's patch relative to its own manifest.
		rootPath := string(root.GetStringBytes("patchedDependencies", string(name)))
		distPath := string(patch.GetStringBytes())
		rootBytes, err := os.ReadFile(filepath.Join("..", rootPath))
		if err != nil {
			t.Fatal(err)
		}
		distBytes, err := os.ReadFile(filepath.Join("dist/deps", distPath))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(rootBytes, distBytes) {
			t.Errorf("dist patch for %s differs from the root patch", name)
		}
	})
}
