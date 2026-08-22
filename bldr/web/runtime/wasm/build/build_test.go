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
	if testing.Short() {
		t.Skip("bundling test requires local tooling")
	}
	for _, tool := range []string{"tinygo", "bun"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available for real wrapper bundle test", tool)
		}
	}
	repoRoot := findRepoRoot(t)
	depsRoot := filepath.Join(repoRoot, "bldr", "dist", "deps")
	if _, err := os.Stat(
		filepath.Join(depsRoot, "node_modules", "rolldown", "dist", "index.mjs"),
	); err != nil {
		t.Skip("bldr/dist/deps dependencies not preinstalled; run bun install there")
	}

	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

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

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertCallableDefaultExport(t, outPath, data)
}

// assertCallableDefaultExport fails unless the built module carries a default
// export shape and importing it yields a function.
func assertCallableDefaultExport(t *testing.T, path string, data []byte) {
	t.Helper()

	defaultShape := regexp.MustCompile(`\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}\s*;?\s*$`)
	defaultStatement := regexp.MustCompile(`(?m)^\s*export\s+default\b|[,;{([]\s*export\s+default\b|^export\{[^}]*\bdefault\b`)
	if !defaultShape.Match(data) && !defaultStatement.Match(data) && !bytes.Contains(data, []byte("export default")) {
		const snippet = 600
		head := data
		if len(head) > snippet {
			head = head[:snippet]
		}
		tail := data
		if len(tail) > snippet {
			tail = tail[len(tail)-snippet:]
		}
		t.Fatalf(
			"built module %s has no default export shape\nhead:\n%s\ntail:\n%s",
			path, head, tail,
		)
	}

	bunPath, err := exec.LookPath("bun")
	if err != nil {
		t.Logf("bun not found; byte-shape check only")
		return
	}
	checkScript := filepath.Join(t.TempDir(), "check-default.ts")
	if err := os.WriteFile(checkScript, []byte(`const mod = await import(process.argv[2]);
console.log("default:" + typeof mod.default);
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bunPath, "run", checkScript, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dynamic import of %s failed: %v\noutput:\n%s", path, err, output)
	}
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
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		name := d.Name()
		if strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") || strings.HasPrefix(name, "tsconfig") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
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
