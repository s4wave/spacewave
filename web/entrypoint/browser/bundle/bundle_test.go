//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/fastjson"
)

func TestBrowserBuildOptsResolvesGoVendorImportsFromNestedDir(t *testing.T) {
	projectRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(projectRoot, "tsconfig.json"),
		[]byte(`{"compilerOptions":{"paths":{"@go/*":["./vendor/*"]}}}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

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

	workingDir := filepath.Join(projectRoot, "web", "entrypoint", "browser")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(workingDir, "entry.ts")
	if err := os.WriteFile(
		entryFile,
		[]byte(`import { greeting } from "@go/example/mod.js"; console.log(greeting);`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(projectRoot, "out.js")
	opts := BrowserBuildOpts(workingDir, false)
	opts.EntryPoints = []string{"entry.ts"}
	opts.Outfile = outFile
	opts.Write = true

	result := esbuild.Build(opts)
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

func TestWriteBuildManifestIncludesServiceWorker(t *testing.T) {
	dir := t.TempDir()
	manifest := &BuildManifest{
		Entrypoint:    "entrypoint/abc123/entrypoint.mjs",
		ServiceWorker: "sw-deadbeef.mjs",
		SharedWorker:  "shw-beadfeed.mjs",
		Wasm:          "entrypoint/abc123/runtime.wasm",
		CSS:           []string{"static/app.css"},
	}
	if err := WriteBuildManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(v.GetStringBytes("serviceWorker")); got != manifest.ServiceWorker {
		t.Fatalf("unexpected serviceWorker: %q", got)
	}
	if got := string(v.GetStringBytes("entrypoint")); got != manifest.Entrypoint {
		t.Fatalf("unexpected entrypoint: %q", got)
	}
}

func TestWriteStableBootAsset(t *testing.T) {
	dir := t.TempDir()
	if err := WriteStableBootAsset(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, stableBootFilename))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if !strings.Contains(script, "/browser-release.json") {
		t.Fatalf("boot asset missing stable release manifest path: %s", script)
	}
	if !strings.Contains(script, "__swGenerationId") {
		t.Fatalf("boot asset missing generation exposure: %s", script)
	}
}
