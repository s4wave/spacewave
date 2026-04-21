//go:build !js

package bldr_web_bundler_esbuild_build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
)

// TestGoVendorTsResolverPlugin verifies that @go/foo/bar.js imports are
// resolved to vendor/foo/bar.ts when only the .ts file exists.
func TestGoVendorTsResolverPlugin(t *testing.T) {
	projectRoot := t.TempDir()

	// Create vendor/example/mod.ts
	vendorDir := filepath.Join(projectRoot, "vendor", "example")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(vendorDir, "mod.ts"),
		[]byte(`export const greeting = "hello"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create an entry file that imports @go/example/mod.js
	entryDir := filepath.Join(projectRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { greeting } from "@go/example/mod.js"; console.log(greeting);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
}

// TestGoVendorTsResolverPlugin_JS verifies that @go/foo/bar.js resolves
// to the .js file when it exists.
func TestGoVendorTsResolverPlugin_JS(t *testing.T) {
	projectRoot := t.TempDir()

	vendorDir := filepath.Join(projectRoot, "vendor", "example")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a .js file (not .ts)
	if err := os.WriteFile(
		filepath.Join(vendorDir, "mod.js"),
		[]byte(`export const val = 42`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	entryDir := filepath.Join(projectRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { val } from "@go/example/mod.js"; console.log(val);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "42") {
		t.Fatalf("output does not contain expected value: %s", out)
	}
}

// TestGoVendorTsResolverPlugin_Missing verifies that unresolvable @go/
// imports produce an esbuild error.
func TestGoVendorTsResolverPlugin_Missing(t *testing.T) {
	projectRoot := t.TempDir()

	// No vendor files created.
	entryDir := filepath.Join(projectRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { x } from "@go/nonexistent/mod.js"; console.log(x);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outdir:      filepath.Join(projectRoot, "out"),
		Bundle:      true,
		Write:       false,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot),
		},
	})
	if len(result.Errors) == 0 {
		t.Fatal("expected esbuild error for missing @go/ import, got none")
	}
}

// TestGoVendorTsResolverPlugin_Local verifies that monorepo-local @go imports
// resolve from the repo root instead of vendor/.
func TestGoVendorTsResolverPlugin_Local(t *testing.T) {
	projectRoot := t.TempDir()

	localDir := filepath.Join(projectRoot, "db", "volume")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(localDir, "volume.pb.ts"),
		[]byte(`export const volume = "ok"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	entryDir := filepath.Join(projectRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { volume } from "@go/github.com/s4wave/spacewave/db/volume/volume.pb.js"; console.log(volume);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
}
