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

func TestEvaluateRootDesktopReleaseBuildsJsEmbeds(t *testing.T) {
	starPath := "../../../bldr.star"
	if _, err := os.Stat(starPath); err != nil {
		t.Skipf("bldr.star not found at %s: %v", starPath, err)
	}

	result, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}

	bc := result.Config.GetBuild()["release-desktop-darwin-arm64"]
	if bc == nil {
		t.Fatal("build target 'release-desktop-darwin-arm64' not found")
	}
	platformIDs := bc.GetPlatformIds()
	if len(platformIDs) != 1 || platformIDs[0] != "desktop/darwin/arm64" {
		t.Fatalf("release desktop platform ids: got %v, want [desktop/darwin/arm64]", platformIDs)
	}

	override := bc.GetManifestOverrides()["spacewave-dist"]
	if override == nil {
		t.Fatal("override for 'spacewave-dist' not found")
	}
	cfg := string(override.GetConfig())
	for _, want := range []string{
		`"spacewave-loader"`,
		`"spacewave-core"`,
		`"web"`,
		`"platformId":"web/js/wasm"`,
		`"spacewave-web"`,
		`"spacewave-app"`,
		`"platformId":"js"`,
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("release desktop override config missing %s: %s", want, cfg)
		}
	}

	browserRelease := result.Config.GetBuild()["release-web"]
	if browserRelease == nil {
		t.Fatal("build target 'release-web' not found")
	}
	browserManifests := strings.Join(browserRelease.GetManifests(), ",")
	if strings.Contains(browserManifests, "spacewave-loader") {
		t.Fatalf("browser release manifests unexpectedly include spacewave-loader: %v", browserRelease.GetManifests())
	}
	browserOverride := browserRelease.GetManifestOverrides()["spacewave-dist"]
	if browserOverride == nil {
		t.Fatal("override for browser 'spacewave-dist' not found")
	}
	browserCfg := string(browserOverride.GetConfig())
	if strings.Contains(browserCfg, `"spacewave-loader"`) {
		t.Fatalf("browser release override unexpectedly includes spacewave-loader: %s", browserCfg)
	}
	for _, want := range []string{
		`"spacewave-launcher"`,
		`"spacewave-core"`,
		`"spacewave-web"`,
		`"spacewave-app"`,
		`"web"`,
	} {
		if !strings.Contains(browserCfg, want) {
			t.Fatalf("browser release override config missing %s: %s", want, browserCfg)
		}
	}

	webBuild := result.Config.GetBuild()["release-remote-web"]
	if webBuild == nil {
		t.Fatal("build target 'release-remote-web' not found")
	}
	webPlatformIDs := webBuild.GetPlatformIds()
	if len(webPlatformIDs) != 1 || webPlatformIDs[0] != "web/js/wasm" {
		t.Fatalf("release remote web platform ids: got %v, want [web/js/wasm]", webPlatformIDs)
	}
	webManifests := strings.Join(webBuild.GetManifests(), ",")
	if !strings.Contains(webManifests, "web") {
		t.Fatalf("release remote web manifests missing web: %v", webBuild.GetManifests())
	}

	jsBuild := result.Config.GetBuild()["release-remote-js"]
	if jsBuild == nil {
		t.Fatal("build target 'release-remote-js' not found")
	}
	jsPlatformIDs := jsBuild.GetPlatformIds()
	if len(jsPlatformIDs) != 1 || jsPlatformIDs[0] != "js" {
		t.Fatalf("release remote js platform ids: got %v, want [js]", jsPlatformIDs)
	}
	jsManifests := strings.Join(jsBuild.GetManifests(), ",")
	for _, want := range []string{"spacewave-web", "spacewave-app"} {
		if !strings.Contains(jsManifests, want) {
			t.Fatalf("release remote js manifests missing %s: %v", want, jsBuild.GetManifests())
		}
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
