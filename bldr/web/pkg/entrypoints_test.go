package web_pkg

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveWebPkgRefsFromConfigUsesTSConfigPathRoot(t *testing.T) {
	dir := t.TempDir()
	webDir := filepath.Join(dir, "web", "state")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "package.json"), []byte(`{"name":"@s4wave/web"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.tsx"), []byte("export const ok = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{
  "compilerOptions": {
    "paths": {
      "@s4wave/web": ["./web"],
      "@s4wave/web/*": ["./web/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := ResolveWebPkgRefsFromConfig(
		dir,
		[]WebPkgResolveConfig{{
			ID: "@s4wave/web",
			Entrypoints: []WebPkgEntrypointConfig{{
				Path: "./state",
			}},
		}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].GetWebPkgRoot() != filepath.Join(dir, "web") {
		t.Fatalf("expected tsconfig path root, got %q", refs[0].GetWebPkgRoot())
	}
	if got := refs[0].GetImports(); !reflect.DeepEqual(got, []string{"state/index.tsx"}) {
		t.Fatalf("expected state entrypoint import, got %v", got)
	}
}

func TestResolveWebPkgRefsFromConfigAddsNodeModuleRootWithExplicitEntrypoints(t *testing.T) {
	dir := t.TempDir()
	pkgRoot := filepath.Join(dir, "node_modules", "non-index-root")
	if err := os.MkdirAll(filepath.Join(pkgRoot, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgRoot, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(`{
  "name": "non-index-root",
  "type": "module",
  "exports": {
    ".": {
      "import": "./build/foo.module.js"
    }
  },
  "module": "./build/foo.module.js"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "build", "foo.module.js"), []byte("export const root = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "examples", "extra.js"), []byte("export const extra = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := ResolveWebPkgRefsFromConfig(
		dir,
		[]WebPkgResolveConfig{{
			ID: "non-index-root",
			Entrypoints: []WebPkgEntrypointConfig{{
				Path: "./examples/extra",
			}},
		}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	got := refs[0].GetImports()
	want := []string{"build/foo.module.js", "examples/extra.js"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected imports: got %v want %v", got, want)
	}
}
