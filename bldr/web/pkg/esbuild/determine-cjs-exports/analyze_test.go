package determine_cjs_exports

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"
)

func TestCjsExportsResultRetainsFiveFieldCompositeShape(t *testing.T) {
	result := CjsExportsResult{"pkg", true, []string{"named"}, "error", "stack"}
	if result.Reexport != "pkg" || !result.ExportDefault || len(result.Exports) != 1 || result.Error != "error" || result.Stack != "stack" {
		t.Fatalf("unexpected unkeyed result: %#v", result)
	}
}

func TestAnalyzeCjsExports_JSON(t *testing.T) {
	// Create a temp directory with a JSON file.
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "test.json")
	if err := os.WriteFile(jsonPath, []byte(`{"foo": 1, "bar": "hello", "baz": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeCjsExports(dir, "./test.json", nil, "production")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Exports) != 3 {
		t.Fatalf("expected 3 exports, got %d: %v", len(result.Exports), result.Exports)
	}
}

func TestAnalyzeCjsExports_UnsupportedExt(t *testing.T) {
	dir := t.TempDir()
	nodePath := filepath.Join(dir, "test.wasm")
	if err := os.WriteFile(nodePath, []byte{0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeCjsExports(dir, "./test.wasm", nil, "production")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Exports) != 0 {
		t.Fatalf("expected 0 exports for .wasm file, got %d", len(result.Exports))
	}
}

func TestAnalyzeCjsExports_NodeEnvConditionalRequire(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "index.js"),
		[]byte(`
if (process.env.NODE_ENV === "production") {
	module.exports = require("./prod.js")
}
if (process.env.NODE_ENV !== "production") {
	module.exports = require("./dev.js")
}
`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prod.js"), []byte(`exports.prodOnly = true`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.js"), []byte(`exports.devOnly = true`), 0o644); err != nil {
		t.Fatal(err)
	}

	prodResult, prodSources, err := AnalyzeCjsExportsWithProvenance(dir, "./index.js", nil, "production")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(prodResult.Exports, "prodOnly") || slices.Contains(prodResult.Exports, "devOnly") {
		t.Fatalf("unexpected production exports: %v", prodResult.Exports)
	}
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantProdSources := []string{filepath.Join(canonicalDir, "index.js"), filepath.Join(canonicalDir, "prod.js")}
	if !slices.Equal(prodSources, wantProdSources) {
		t.Fatalf("unexpected production sources: got %v want %v", prodSources, wantProdSources)
	}

	devResult, err := AnalyzeCjsExports(dir, "./index.js", nil, "development")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(devResult.Exports, "devOnly") || slices.Contains(devResult.Exports, "prodOnly") {
		t.Fatalf("unexpected development exports: %v", devResult.Exports)
	}
}

func TestVerifyExports(t *testing.T) {
	names := []string{"default", "foo", "bar", "class", "123invalid", "valid$name", "foo"}
	result := verifyExports(names)

	if !result.ExportDefault {
		t.Fatal("expected exportDefault to be true")
	}

	// "class" is reserved, "123invalid" is not a valid identifier, "foo" is duplicated.
	// Valid: "foo", "bar", "valid$name"
	if len(result.Exports) != 3 {
		t.Fatalf("expected 3 exports, got %d: %v", len(result.Exports), result.Exports)
	}

	// Check "default" is excluded (reserved word).
	for _, exp := range result.Exports {
		if exp == "default" {
			t.Fatal("default should be excluded from exports (reserved word)")
		}
		if exp == "class" {
			t.Fatal("class should be excluded from exports (reserved word)")
		}
	}
}

func TestResolveModule_Relative(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "index.js"), []byte("module.exports = {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Resolve ./lib should find ./lib/index.js
	resolved, err := ResolveModule(dir, "./lib")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(libDir, "index.js")
	if resolved != expected {
		t.Fatalf("expected %s, got %s", expected, resolved)
	}
}

func TestResolveModule_WithExtension(t *testing.T) {
	dir := t.TempDir()
	jsFile := filepath.Join(dir, "main.js")
	if err := os.WriteFile(jsFile, []byte("module.exports = {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Resolve ./main should find ./main.js
	resolved, err := ResolveModule(dir, "./main")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != jsFile {
		t.Fatalf("expected %s, got %s", jsFile, resolved)
	}
}

func TestAnalyzeCjsExports_React(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// React is installed in the bldr project root node_modules.
	bldrRoot := filepath.Join(wd, "../../../..")
	result, err := AnalyzeCjsExports(bldrRoot, "react", nil, "production")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("pure-Go react exports (%d): %v", len(result.Exports), result.Exports)

	// React should have many exports (useState, useEffect, createElement, etc.)
	if len(result.Exports) < 10 {
		t.Fatalf("expected at least 10 exports from react, got %d: %v", len(result.Exports), result.Exports)
	}

	// Check some specific well-known React exports are present.
	sort.Strings(result.Exports)
	expected := []string{"useState", "useEffect", "createElement", "Component", "Fragment"}
	for _, exp := range expected {
		if !slices.Contains(result.Exports, exp) {
			t.Errorf("missing expected export: %s", exp)
		}
	}
}

func TestResolveModule_BarePackage(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "node_modules", "testpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"main": "./lib/index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	libDir := filepath.Join(pkgDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "index.js"), []byte("module.exports = {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveModule(dir, "testpkg")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(libDir, "index.js")
	if resolved != expected {
		t.Fatalf("expected %s, got %s", expected, resolved)
	}
}

func TestAnalyzeCjsExportsPackageMainProvenance(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "node_modules", "redirect-pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := filepath.Join(pkgDir, "package.json")
	first := filepath.Join(pkgDir, "first.js")
	second := filepath.Join(pkgDir, "second.js")
	if err := os.WriteFile(first, []byte("exports.first = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("exports.second = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageJSON, []byte(`{"main":"first.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	firstResult, firstSources, err := AnalyzeCjsExportsWithProvenance(dir, "redirect-pkg", nil, "production")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageJSON, []byte(`{"main":"second.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	secondResult, secondSources, err := AnalyzeCjsExportsWithProvenance(dir, "redirect-pkg", nil, "production")
	if err != nil {
		t.Fatal(err)
	}
	canonicalPackageJSON, err := filepath.EvalSymlinks(packageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(firstSources, canonicalPackageJSON) || !slices.Contains(secondSources, canonicalPackageJSON) {
		t.Fatalf("package manifest missing from provenance: first=%v second=%v", firstSources, secondSources)
	}
	if !slices.Contains(firstResult.Exports, "first") || !slices.Contains(secondResult.Exports, "second") {
		t.Fatalf("package main redirect did not change exports: first=%v second=%v", firstResult.Exports, secondResult.Exports)
	}
}

func TestResolveModuleProvenancePreservesPrimaryBareError(t *testing.T) {
	dir := t.TempDir()
	failedNodePath := filepath.Join(dir, "extra", "node_modules")
	failedPkg := filepath.Join(failedNodePath, "missing-package")
	if err := os.MkdirAll(failedPkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(failedPkg, "package.json"), []byte(`{"main":"also-missing.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ResolveModuleWithProvenance(dir, "missing-package", []string{failedNodePath})
	var notFound *ModuleNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("resolution error = %v, want ModuleNotFoundError", err)
	}
	if notFound.Path != "missing-package" {
		t.Fatalf("unresolved path = %q, want primary import", notFound.Path)
	}
}

func TestResolveModulePreservesAbsentAbsoluteExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.js")
	resolved, err := ResolveModule(t.TempDir(), path)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolved path = %q, want %q", resolved, path)
	}
	provenanceResolved, provenance, err := ResolveModuleWithProvenance(t.TempDir(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provenanceResolved != path || len(provenance) != 0 {
		t.Fatalf("provenance resolution = %q %v, want absent path and no manifests", provenanceResolved, provenance)
	}
}

func TestResolveModuleProvenanceIncludesFailedPackageCandidates(t *testing.T) {
	dir := t.TempDir()
	failedNodePath := filepath.Join(dir, "failed", "node_modules")
	goodNodePath := filepath.Join(dir, "good", "node_modules")
	failedPkg := filepath.Join(failedNodePath, "candidate")
	goodPkg := filepath.Join(goodNodePath, "candidate")
	for _, pkg := range []string{failedPkg, goodPkg} {
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	failedManifest := filepath.Join(failedPkg, "package.json")
	goodManifest := filepath.Join(goodPkg, "package.json")
	if err := os.WriteFile(failedManifest, []byte(`{"main":"missing.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goodManifest, []byte(`{"main":"index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goodPkg, "index.js"), []byte("exports.ok = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, provenance, err := ResolveModuleWithProvenance(filepath.Join(dir, "source"), "candidate", []string{failedNodePath, goodNodePath})
	if err != nil {
		t.Fatal(err)
	}
	canonicalFailed, err := filepath.EvalSymlinks(failedManifest)
	if err != nil {
		t.Fatal(err)
	}
	canonicalGood, err := filepath.EvalSymlinks(goodManifest)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{canonicalFailed, canonicalGood}
	if !slices.Equal(provenance, want) {
		t.Fatalf("resolution provenance = %v, want %v", provenance, want)
	}
}

func TestAnalyzeCjsExportsRelativeReexportCycles(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string]string
	}{
		{name: "self", files: map[string]string{"a.js": `module.exports = require("./a.js")`}},
		{name: "two-file", files: map[string]string{
			"a.js": `module.exports = require("./b.js")`,
			"b.js": `module.exports = require("./a.js")`,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range test.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				_, err := AnalyzeCjsExports(dir, "./a.js", nil, "production")
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cyclic CJS reexport analysis did not terminate")
			}
		})
	}
}
