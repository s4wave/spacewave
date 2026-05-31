package web_pkg_external

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestBldrDistWebPkgRefsIncludeQuickJSWASIReactor(t *testing.T) {
	const buildPkgsDir = "/build-pkgs"
	refs := GetBldrDistWebPkgRefs(buildPkgsDir, "/bldr")

	var found bool
	for _, ref := range refs {
		if ref.GetWebPkgId() != "quickjs-wasi-reactor" {
			continue
		}
		found = true
		if got, want := ref.GetWebPkgRoot(), filepath.Join(buildPkgsDir, "node_modules/quickjs-wasi-reactor/dist"); got != want {
			t.Fatalf("quickjs-wasi-reactor root=%q want %q", got, want)
		}
		if !slices.Contains(ref.GetImports(), "index.js") {
			t.Fatalf("quickjs-wasi-reactor imports=%v, want index.js", ref.GetImports())
		}
	}
	if !found {
		t.Fatal("quickjs-wasi-reactor missing from Bldr dist web package refs")
	}

	if !slices.Contains(BldrExternal, "quickjs-wasi-reactor") {
		t.Fatal("quickjs-wasi-reactor missing from BldrExternal")
	}
}
