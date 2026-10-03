//go:build !js

package web_runtime_wasm_build

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestBuildWebWasmPluginScriptPreservesDefaultExport drives the real TinyGo
// plugin wrapper bundle over the real plugin-wasm.ts sources and asserts the
// emitted ES module keeps a callable default export. The plugin worker imports
// the built module and rejects it with "does not have a default export
// function" when the default export is missing.
//
// The test needs tinygo, bun, and a preinstalled bldr/dist/deps dependency
// root; it skips instead of downloading anything when they are absent so the
// default suite stays hermetic.
func TestBuildWebWasmPluginScriptPreservesDefaultExport(t *testing.T) {
	// Require the local tools needed to bundle the real TinyGo wrapper.
	if testing.Short() {
		t.Skip("bundling test requires local tooling")
	}
	for _, tool := range []string{"tinygo", "bun"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available for real wrapper bundle test", tool)
		}
	}

	// Require preinstalled Rolldown dependencies for the wrapper bundle.
	repoRoot := findRepoRoot(t)
	depsRoot := filepath.Join(repoRoot, "bldr", "dist", "deps")
	if _, err := os.Stat(
		filepath.Join(depsRoot, "node_modules", "rolldown", "dist", "index.mjs"),
	); err != nil {
		t.Skip("bldr/dist/deps dependencies not preinstalled; run bun install there")
	}

	// Prepare the context and logger for the real wrapper build.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Allocate an isolated distribution root for the wrapper sources.
	distRoot := t.TempDir()

	// The rolldown runner resolves its own module from the dist deps root;
	// link the preinstalled dependency root so nothing downloads.
	if err := os.MkdirAll(filepath.Join(distRoot, "dist", "deps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(
		filepath.Join(depsRoot, "node_modules"),
		filepath.Join(distRoot, "dist", "deps", "node_modules"),
	); err != nil {
		t.Fatal(err)
	}

	// Copy the dependency manifest into the isolated distribution root.
	installed, err := os.ReadFile(filepath.Join(depsRoot, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(distRoot, "dist", "deps", "package.json"),
		installed,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Copy the real wrapper sources so the bundle sees production inputs.
	// The synced dist root flattens the bldr module contents to the top
	// level: web/ carries the runtime and the rolldown runner, plugin/ and
	// manifest/ the protobuf modules the wrapper reaches through @go/
	// generated imports.
	copyTree(t, filepath.Join(repoRoot, "bldr", "web"), filepath.Join(distRoot, "web"))
	copyTree(t, filepath.Join(repoRoot, "bldr", "plugin"), filepath.Join(distRoot, "plugin"))
	copyTree(t, filepath.Join(repoRoot, "bldr", "manifest"), filepath.Join(distRoot, "manifest"))
	copyTree(t, filepath.Join(repoRoot, "bldr", "sdk"), filepath.Join(distRoot, "sdk"))
	for _, pkg := range []string{
		filepath.Join("db", "block"),
		filepath.Join("db", "bucket"),
		filepath.Join("db", "volume"),
		filepath.Join("net", "hash"),
	} {
		copyTree(t, filepath.Join(repoRoot, pkg), filepath.Join(distRoot, pkg))
	}
	copyTree(
		t,
		filepath.Join(repoRoot, "vendor", "github.com", "aperturerobotics", "controllerbus"),
		filepath.Join(distRoot, "vendor", "github.com", "aperturerobotics", "controllerbus"),
	)

	// Build the TinyGo wrapper as the plugin worker loads it.
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "spacewave-core.mjs")
	if _, err := BuildWebWasmPluginScript(
		ctx,
		le,
		distRoot,
		outPath,
		"spacewave-core.wasm",
		true, // useTinygo
		true, // minify matches dev serving
		false,
	); err != nil {
		t.Fatalf("build tinygo plugin script: %v", err)
	}

	// Verify the emitted plugin module has a callable default export.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertCallableDefaultExport(t, outPath, data)
}

// assertCallableDefaultExport fails unless the built module carries a default
// export shape and importing it yields a function.
func assertCallableDefaultExport(t *testing.T, path string, data []byte) {
	// Attribute default export failures to the calling test.
	t.Helper()

	// Recognize the bundled module default export syntax.
	defaultShape := regexp.MustCompile(`\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}\s*;?\s*$`)
	defaultStatement := regexp.MustCompile(`(?m)^\s*export\s+default\b|[,;{([]\s*export\s+default\b|^export\{[^}]*\bdefault\b`)
	if !defaultShape.Match(data) && !defaultStatement.Match(data) && !bytes.Contains(data, []byte("export default")) {
		// Capture the module head for a missing default export diagnostic.
		const snippet = 600
		head := data
		if len(head) > snippet {
			head = head[:snippet]
		}

		// Capture the module tail where Rolldown emits export declarations.
		tail := data
		if len(tail) > snippet {
			tail = tail[len(tail)-snippet:]
		}

		// Report the missing module export with both source excerpts.
		t.Fatalf(
			"built module %s has no default export shape\nhead:\n%s\ntail:\n%s",
			path, head, tail,
		)
	}

	// Find Bun for a runtime check of the module default export.
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		t.Logf("bun not found; byte-shape check only")
		return
	}

	// Write a module import that reports the default export type.
	checkScript := filepath.Join(t.TempDir(), "check-default.ts")
	if err := os.WriteFile(checkScript, []byte(`const mod = await import(process.argv[2]);
console.log("default:" + typeof mod.default);
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Import the bundled plugin module through Bun.
	cmd := exec.Command(bunPath, "run", checkScript, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dynamic import of %s failed: %v\noutput:\n%s", path, err, output)
	}

	// Require the imported plugin default export to be a function.
	if got := strings.TrimSpace(string(output)); !strings.HasSuffix(got, "default:function") {
		t.Fatalf("dynamic import of %s: default export type = %q, want function", path, got)
	}
}

// findRepoRoot walks up from the package directory to the go.mod root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above package directory")
		}
		dir = parent
	}
}

// copyTree copies a source directory, skipping test files.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		// Propagate source traversal failures before copying wrapper inputs.
		if walkErr != nil {
			return walkErr
		}

		// Resolve the source entry within the distribution destination.
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Create destination directories for the source tree.
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		// Exclude test sources and TypeScript configuration from wrapper inputs.
		name := d.Name()
		if strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") || strings.HasPrefix(name, "tsconfig") {
			return nil
		}

		// Require a regular source file before reading its contents.
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		// Read the wrapper input and prepare its destination directory.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode().Perm())
	}); err != nil {
		t.Fatal(err)
	}
}
