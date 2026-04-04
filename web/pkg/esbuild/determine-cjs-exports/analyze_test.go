package determine_cjs_exports

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

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
