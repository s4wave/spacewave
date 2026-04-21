//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
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
