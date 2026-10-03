package web_pkg

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveWebPkgRefsFromConfigUsesTSConfigPathRoot(t *testing.T) {
	// Create a local web package with a TypeScript path mapping and state entrypoint.
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

	// Resolve the state entrypoint through the TypeScript package mapping.
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

	// Verify the package reference uses the mapped root and state import.
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

func TestResolveWebPkgRefsFromConfigDirectoryEntrypointIncludesDirectFiles(t *testing.T) {
	// Create the command package directory and its root barrel.
	dir := t.TempDir()
	commandDir := filepath.Join(dir, "web", "command")
	if err := os.MkdirAll(filepath.Join(commandDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "package.json"), []byte(`{"name":"@s4wave/web"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "index.ts"), []byte("export const command = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Populate direct command files, a test file, and a nested entrypoint.
	if err := os.WriteFile(filepath.Join(commandDir, "useCommand.ts"), []byte("export const useCommand = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "CommandPalette.tsx"), []byte("export const CommandPalette = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "useCommand.test.tsx"), []byte("export const testOnly = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "child", "nested.ts"), []byte("export const nested = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Map the local web package through the TypeScript configuration.
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

	// Resolve the configured command directory entrypoint.
	refs, err := ResolveWebPkgRefsFromConfig(
		dir,
		[]WebPkgResolveConfig{{
			ID: "@s4wave/web",
			Entrypoints: []WebPkgEntrypointConfig{{
				Path: "./command",
			}},
		}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the command imports include direct files and exclude tests and children.
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	got := refs[0].GetImports()
	want := []string{"command/index.ts", "command/CommandPalette.tsx", "command/useCommand.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected command entrypoint imports: got %v want %v", got, want)
	}
}

func TestResolveWebPkgRefsFromConfigAddsNodeModuleRootWithExplicitEntrypoints(t *testing.T) {
	// Create a node module with a root export and an additional example file.
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

	// Resolve the configured example entrypoint alongside the node module root.
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

	// Verify the node module serves both its root export and explicit entrypoint.
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	got := refs[0].GetImports()
	want := []string{"build/foo.module.js", "examples/extra.js"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected imports: got %v want %v", got, want)
	}
}

func TestResolveWebPkgEntrypointsNodeModuleSubpathExports(t *testing.T) {
	// Create a package manifest with root and subpath exports.
	pkgRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(`{
  "name": "exported-subpaths",
  "type": "module",
  "exports": {
    ".": { "types": "./dist/index.d.mts", "default": "./dist/index.mjs" },
    "./langs": { "types": "./dist/langs.d.mts", "default": "./dist/langs.mjs" },
    "./worker": "./dist/worker.mjs",
    "./*": "./dist/*"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Resolve the package entrypoints without explicit subpaths.
	got, err := ResolveWebPkgEntrypoints(pkgRoot, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify default package imports contain only the root export.
	if want := []string{"dist/index.mjs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default imports: got %v want %v", got, want)
	}

	// Resolve the package entrypoints with the explicit language subpath.
	got, err = ResolveWebPkgEntrypoints(pkgRoot, []WebPkgEntrypointConfig{{Path: "./langs"}})
	if err != nil {
		t.Fatal(err)
	}

	// Verify explicit package imports include the root and language exports.
	if want := []string{"dist/index.mjs", "dist/langs.mjs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit imports: got %v want %v", got, want)
	}
}
