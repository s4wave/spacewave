//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateMinimal(t *testing.T) {
	dir := t.TempDir()
	starFile := filepath.Join(dir, "bldr.star")
	err := os.WriteFile(starFile, []byte(`
project(id="test-project")
manifest("test-manifest", builder="bldr/plugin/compiler/go", rev=1, config={"goPkgs": ["./pkg"]})
build("app", manifests=["test-manifest"], targets=["desktop"])
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Evaluate(starFile)
	if err != nil {
		t.Fatal(err)
	}

	if result.Config.GetId() != "test-project" {
		t.Fatalf("expected project id 'test-project', got %q", result.Config.GetId())
	}
	if len(result.Config.GetManifests()) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(result.Config.GetManifests()))
	}
	mc := result.Config.GetManifests()["test-manifest"]
	if mc == nil {
		t.Fatal("manifest 'test-manifest' not found")
	}
	if mc.GetBuilder().GetId() != "bldr/plugin/compiler/go" {
		t.Fatalf("expected builder id 'bldr/plugin/compiler/go', got %q", mc.GetBuilder().GetId())
	}
	if mc.GetBuilder().GetRev() != 1 {
		t.Fatalf("expected builder rev 1, got %d", mc.GetBuilder().GetRev())
	}
	if len(result.Config.GetBuild()) != 1 {
		t.Fatalf("expected 1 build target, got %d", len(result.Config.GetBuild()))
	}
	bc := result.Config.GetBuild()["app"]
	if bc == nil {
		t.Fatal("build target 'app' not found")
	}
	if len(bc.GetManifests()) != 1 || bc.GetManifests()[0] != "test-manifest" {
		t.Fatalf("unexpected build manifests: %v", bc.GetManifests())
	}
}

func TestEvaluateConfigEntry(t *testing.T) {
	dir := t.TempDir()
	starFile := filepath.Join(dir, "bldr.star")
	err := os.WriteFile(starFile, []byte(`
project(id="test")
manifest("core",
    builder="bldr/plugin/compiler/go",
    rev=1,
    config={
        "goPkgs": ["./pkg"],
        "configSet": {
            "store-peer": config_entry("object/peer", 1, {
                "objectStoreId": "test-peer",
            }),
            "root-resource": config_entry("resource/root", 1),
        },
    },
)
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Evaluate(starFile)
	if err != nil {
		t.Fatal(err)
	}

	mc := result.Config.GetManifests()["core"]
	if mc == nil {
		t.Fatal("manifest 'core' not found")
	}

	// The config should be valid JSON containing configSet.
	configData := mc.GetBuilder().GetConfig()
	if len(configData) == 0 {
		t.Fatal("expected non-empty builder config")
	}
	t.Logf("builder config JSON: %s", string(configData))
}

func TestEvaluateLoad(t *testing.T) {
	dir := t.TempDir()

	// Write a library file.
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := os.WriteFile(filepath.Join(libDir, "common.star"), []byte(`
SHARED_PKGS = ["./shared/pkg1", "./shared/pkg2"]
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Write the root file that loads the library.
	starFile := filepath.Join(dir, "bldr.star")
	err = os.WriteFile(starFile, []byte(`
load("lib/common.star", "SHARED_PKGS")
project(id="test")
manifest("core",
    builder="bldr/plugin/compiler/go",
    config={"goPkgs": SHARED_PKGS},
)
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Evaluate(starFile)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.LoadedFiles) != 2 {
		t.Fatalf("expected 2 loaded files, got %d: %v", len(result.LoadedFiles), result.LoadedFiles)
	}
}
