//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"strings"
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

func TestEvaluateManifestOverrides(t *testing.T) {
	dir := t.TempDir()
	starFile := filepath.Join(dir, "bldr.star")
	err := os.WriteFile(starFile, []byte(`
project(id="test")
manifest("spacewave-dist", builder="bldr/plugin/compiler/dist", rev=1)
build("release-desktop-darwin-arm64",
    manifests=["spacewave-dist"],
    targets=["desktop/darwin/arm64"],
    manifestOverrides={
        "spacewave-dist": dist_compiler_config(
            cliPkgs=["./cmd/spacewave-cli/cli"],
            embedManifests=[
                {"manifestId": "spacewave-launcher", "platformId": "desktop/darwin/arm64"},
                {"manifestId": "spacewave-loader", "platformId": "desktop/darwin/arm64"},
            ],
        ),
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

	bc := result.Config.GetBuild()["release-desktop-darwin-arm64"]
	if bc == nil {
		t.Fatal("build target 'release-desktop-darwin-arm64' not found")
	}
	overrides := bc.GetManifestOverrides()
	if len(overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(overrides))
	}
	override := overrides["spacewave-dist"]
	if override == nil {
		t.Fatal("override for 'spacewave-dist' not found")
	}
	if override.GetId() != "" {
		t.Fatalf("expected empty override id, got %q", override.GetId())
	}
	cfg := string(override.GetConfig())
	if !strings.Contains(cfg, `"embedManifests"`) {
		t.Fatalf("expected override config to contain embedManifests, got %s", cfg)
	}
	if !strings.Contains(cfg, `"spacewave-launcher"`) {
		t.Fatalf("expected override config to contain spacewave-launcher, got %s", cfg)
	}
	if !strings.Contains(cfg, `"desktop/darwin/arm64"`) {
		t.Fatalf("expected override config to contain platform id, got %s", cfg)
	}
	if !strings.Contains(cfg, `"cliPkgs"`) {
		t.Fatalf("expected override config to contain cliPkgs, got %s", cfg)
	}
	if !strings.Contains(cfg, `"./cmd/spacewave-cli/cli"`) {
		t.Fatalf("expected override config to contain CLI package path, got %s", cfg)
	}
}

func TestEvaluateManifestOverridesRejectsNonDict(t *testing.T) {
	dir := t.TempDir()
	starFile := filepath.Join(dir, "bldr.star")
	err := os.WriteFile(starFile, []byte(`
project(id="test")
manifest("foo", builder="bldr/plugin/compiler/go", rev=1)
build("bad",
    manifests=["foo"],
    manifestOverrides={"foo": "not-a-dict"},
)
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Evaluate(starFile)
	if err == nil {
		t.Fatal("expected error for non-dict override value")
	}
	if !strings.Contains(err.Error(), `manifestOverrides["foo"]`) {
		t.Fatalf("expected error to name manifest id, got %v", err)
	}
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
