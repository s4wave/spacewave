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
	// Prepare a project root for resolving a vendored TypeScript module.
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

	// Bundle the entry with the Go vendor resolver and require a successful build.
	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot, projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	// Verify the bundle includes the vendored TypeScript export.
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
	// Prepare a project root for resolving a vendored JavaScript module.
	projectRoot := t.TempDir()

	// Create the vendor directory for the JavaScript module.
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

	// Create an entry importing the vendored JavaScript module.
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

	// Bundle the JavaScript module with the Go vendor resolver.
	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot, projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	// Verify the bundle includes the JavaScript export.
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
	// Prepare a project root with no vendored module.
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

	// Bundle the missing Go import and require a resolution error.
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outdir:      filepath.Join(projectRoot, "out"),
		Bundle:      true,
		Write:       false,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot, projectRoot),
		},
	})
	if len(result.Errors) == 0 {
		t.Fatal("expected esbuild error for missing @go/ import, got none")
	}
}

// TestGoVendorTsResolverPlugin_Local verifies that monorepo-local @go imports
// resolve from the repo root instead of vendor/.
func TestGoVendorTsResolverPlugin_Local(t *testing.T) {
	// Declare the project module for local Go import resolution.
	projectRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(projectRoot, "go.mod"),
		[]byte("module github.com/s4wave/spacewave\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create the local module source outside the vendor tree.
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

	// Create an entry importing the local module through its Go path.
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

	// Bundle the local Go import with the vendor resolver.
	outFile := filepath.Join(projectRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(projectRoot, projectRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	// Verify the bundle includes the local module export.
	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
}

// TestGoVendorTsResolverPlugin_EscapedRelative verifies that a relative import
// escaping the flattened Bldr source root resolves through the dist vendor tree.
func TestGoVendorTsResolverPlugin_EscapedRelative(t *testing.T) {
	// Declare an external application with a separate dist source root.
	sourceRoot := t.TempDir()
	distSourceRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(sourceRoot, "go.mod"),
		[]byte("module github.com/example/app\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create the vendored source targeted by the escaped relative import.
	vendorDir := filepath.Join(
		distSourceRoot,
		"vendor",
		"github.com",
		"s4wave",
		"spacewave",
		"sdk",
		"objecttype",
		"registry",
	)
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(vendorDir, "registry.pb.ts"),
		[]byte(`export const registry = "escaped-relative-vendor"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create a flattened host source whose relative import escapes the dist root.
	hostDir := filepath.Join(distSourceRoot, "sdk", "plugin", "host")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(hostDir, "host.pb.ts"),
		[]byte(`import { registry } from "../../../../sdk/objecttype/registry/registry.pb.js";
export { registry };`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create an entry importing the flattened host source.
	entryDir := filepath.Join(distSourceRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { registry } from "sdk/plugin/host/host.pb.js"; console.log(registry);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Bundle the escaped relative import with the Go vendor resolver.
	outFile := filepath.Join(distSourceRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(sourceRoot, distSourceRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	// Verify the bundle includes the source from the dist vendor tree.
	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "escaped-relative-vendor") {
		t.Fatalf("output does not contain expected vendor string: %s", out)
	}
}

// TestGoVendorTsResolverPlugin_DistVendor verifies that external apps can use
// the Bldr-generated dist source vendor tree instead of an app-root vendor
// mirror.
func TestGoVendorTsResolverPlugin_DistVendor(t *testing.T) {
	// Declare an external application with a separate dist vendor tree.
	sourceRoot := t.TempDir()
	distSourceRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(sourceRoot, "go.mod"),
		[]byte("module github.com/example/app\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create a utility module in the dist vendor tree.
	vendorDir := filepath.Join(distSourceRoot, "vendor", "github.com", "aperturerobotics", "util", "pipesock")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(vendorDir, "pipesock.ts"),
		[]byte(`export const pipe = "dist-vendor"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create the Spacewave module in the dist vendor tree.
	spacewaveVendorDir := filepath.Join(distSourceRoot, "vendor", "github.com", "s4wave", "spacewave", "db", "volume")
	if err := os.MkdirAll(spacewaveVendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(spacewaveVendorDir, "volume.pb.ts"),
		[]byte(`export const volume = "spacewave-vendor"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create the plugin module in the flattened dist source tree.
	distPluginDir := filepath.Join(distSourceRoot, "web", "plugin")
	if err := os.MkdirAll(distPluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(distPluginDir, "plugin.pb.ts"),
		[]byte(`export const plugin = "dist-source"`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create an entry importing both vendored modules and the dist plugin.
	entryDir := filepath.Join(distSourceRoot, "src")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(entryDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { pipe } from "@go/github.com/aperturerobotics/util/pipesock/pipesock.js";
import { volume } from "@go/github.com/s4wave/spacewave/db/volume/volume.pb.js";
import { plugin } from "web/plugin/plugin.pb.js";
console.log(pipe, volume, plugin);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Bundle the entry using the separate source and dist roots.
	outFile := filepath.Join(distSourceRoot, "out.js")
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entryFile},
		Outfile:     outFile,
		Bundle:      true,
		Write:       true,
		Format:      esbuild.FormatESModule,
		Platform:    esbuild.PlatformNode,
		Plugins: []esbuild.Plugin{
			GoVendorTsResolverPlugin(sourceRoot, distSourceRoot),
		},
	})
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("esbuild error: %s", e.Text)
		}
		t.Fatal("esbuild build failed")
	}

	// Verify the bundle includes both vendored exports and the dist source export.
	out, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "dist-vendor") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
	if !strings.Contains(string(out), "spacewave-vendor") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
	if !strings.Contains(string(out), "dist-source") {
		t.Fatalf("output does not contain expected string: %s", out)
	}
}
