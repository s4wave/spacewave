//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
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
            cliPkgs=["./cmd/spacewave/cli"],
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
	if !strings.Contains(cfg, `"./cmd/spacewave/cli"`) {
		t.Fatalf("expected override config to contain CLI package path, got %s", cfg)
	}
}

func TestEvaluateRejectsRelativeLoadOutsideProject(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.star"), []byte("VALUE = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	starFile := filepath.Join(projectDir, "bldr.star")
	if err := os.WriteFile(starFile, []byte(`
load("../outside.star", "VALUE")
project(id="test")
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Evaluate(starFile)
	if err == nil {
		t.Fatal("expected load outside project root to fail")
	}
	if !strings.Contains(err.Error(), "path escapes project root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvaluateRejectsGoVendorLoadOutsideVendor(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.Mkdir(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.star"), []byte("VALUE = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	starFile := filepath.Join(dir, "bldr.star")
	if err := os.WriteFile(starFile, []byte(`
load("@go/../outside.star", "VALUE")
project(id="test")
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Evaluate(starFile)
	if err == nil {
		t.Fatal("expected @go load outside vendor root to fail")
	}
	if !strings.Contains(err.Error(), "path escapes vendor root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvaluateRejectsLoadSymlinkOutsideProject(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.star")
	if err := os.WriteFile(outside, []byte("VALUE = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(projectDir, "outside-link.star")); err != nil {
		t.Fatal(err)
	}
	starFile := filepath.Join(projectDir, "bldr.star")
	if err := os.WriteFile(starFile, []byte(`
load("outside-link.star", "VALUE")
project(id="test")
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Evaluate(starFile)
	if err == nil {
		t.Fatal("expected load symlink outside project root to fail")
	}
	if !strings.Contains(err.Error(), "path escapes project root") {
		t.Fatalf("unexpected error: %v", err)
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

	core := result.Config.GetManifests()["spacewave-core"]
	if core == nil {
		t.Fatal("spacewave-core manifest not found")
	}
	coreCfg := string(core.GetBuilder().GetConfig())
	for _, want := range []string{
		`"accountEndpoint":"https://account.spacewave.app"`,
		`"endpoint":"https://spacewave.app"`,
	} {
		if !strings.Contains(coreCfg, want) {
			t.Fatalf("spacewave-core config missing %s: %s", want, coreCfg)
		}
	}
	launcher := result.Config.GetManifests()["spacewave-launcher"]
	if launcher == nil {
		t.Fatal("spacewave-launcher manifest not found")
	}
	launcherCfg := string(launcher.GetBuilder().GetConfig())
	for _, want := range []string{
		`"distPeerIds":["12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"]`,
		`"url":"https://spacewave.app/api/release/config"`,
	} {
		if !strings.Contains(launcherCfg, want) {
			t.Fatalf("spacewave-launcher config missing %s: %s", want, launcherCfg)
		}
	}
	if strings.Contains(launcherCfg, "staging.spacewave.app") {
		t.Fatalf("spacewave-launcher public config contains staging endpoint: %s", launcherCfg)
	}
	web := result.Config.GetManifests()["web"]
	if web == nil {
		t.Fatal("web manifest not found")
	}
	webCfg := string(web.GetBuilder().GetConfig())
	for _, want := range []string{
		`"desktopPresencePolicy":"DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND"`,
		`"trayIconPath":"web/images/spacewave-icon.png"`,
		`"macosTemplateTrayIconPath":"web/images/spacewave-tray-template.png"`,
	} {
		if !strings.Contains(webCfg, want) {
			t.Fatalf("web config missing %s: %s", want, webCfg)
		}
	}

	app := result.Config.GetManifests()["spacewave-app"]
	if app == nil {
		t.Fatal("spacewave-app manifest not found")
	}
	appCfg := string(app.GetBuilder().GetConfig())
	if !strings.Contains(appCfg, `"path":"./app/App.tsx"`) {
		t.Fatalf("spacewave-app config missing app frontend: %s", appCfg)
	}
	for _, oldPath := range []string{
		`"./plugin/notes/backend.ts"`,
		`"./plugin/v86/backend.ts"`,
		`"./plugin/vm/backend.ts"`,
	} {
		if strings.Contains(appCfg, oldPath) {
			t.Fatalf("spacewave-app config still owns split backend %s: %s", oldPath, appCfg)
		}
	}

	notes := result.Config.GetManifests()["spacewave-notes"]
	if notes == nil {
		t.Fatal("notes manifest not found")
	}
	notesCfg := string(notes.GetBuilder().GetConfig())
	for _, want := range []string{
		`"path":"./plugin/notes/backend.ts"`,
		`"path":"./plugin/notes/NotebookViewer.tsx"`,
		`"path":"./plugin/notes/BlogViewer.tsx"`,
		`"path":"./plugin/notes/DocsViewer.tsx"`,
	} {
		if !strings.Contains(notesCfg, want) {
			t.Fatalf("notes config missing %s: %s", want, notesCfg)
		}
	}

	v86 := result.Config.GetManifests()["spacewave-v86"]
	if v86 == nil {
		t.Fatal("v86 manifest not found")
	}
	v86Cfg := string(v86.GetBuilder().GetConfig())
	for _, want := range []string{
		`"path":"./plugin/v86/backend.ts"`,
		`"path":"./plugin/v86/VmV86Viewer.tsx"`,
	} {
		if !strings.Contains(v86Cfg, want) {
			t.Fatalf("v86 config missing %s: %s", want, v86Cfg)
		}
	}
	if strings.Contains(v86Cfg, `./plugin/vm/`) {
		t.Fatalf("v86 config still references old plugin/vm path: %s", v86Cfg)
	}
	for _, coldPlugin := range []string{"spacewave-notes", "spacewave-v86"} {
		if slices.Contains(result.Config.GetStart().GetPlugins(), coldPlugin) {
			t.Fatalf("project startup unexpectedly eager-loads %s: %v", coldPlugin, result.Config.GetStart().GetPlugins())
		}
	}
	for _, buildName := range []string{"app", "web"} {
		build := result.Config.GetBuild()[buildName]
		if build == nil {
			t.Fatalf("build target %q not found", buildName)
		}
		for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
			if !slices.Contains(build.GetManifests(), want) {
				t.Fatalf("%s build manifests missing %s: %v", buildName, want, build.GetManifests())
			}
		}
	}

	bc := result.Config.GetBuild()["release-desktop-darwin-arm64"]
	if bc == nil {
		t.Fatal("build target 'release-desktop-darwin-arm64' not found")
	}
	platformIDs := bc.GetPlatformIds()
	if len(platformIDs) != 1 || platformIDs[0] != "desktop/darwin/arm64" {
		t.Fatalf("release desktop platform ids: got %v, want [desktop/darwin/arm64]", platformIDs)
	}
	for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
		if !slices.Contains(bc.GetManifests(), want) {
			t.Fatalf("release desktop manifests missing %s: %v", want, bc.GetManifests())
		}
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
		`"platformId":"desktop/darwin/arm64"`,
		`"spacewave-web"`,
		`"spacewave-app"`,
		`"platformId":"js"`,
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("release desktop override config missing %s: %s", want, cfg)
		}
	}
	for _, coldPlugin := range []string{`"spacewave-notes"`, `"spacewave-v86"`} {
		if strings.Contains(cfg, coldPlugin) {
			t.Fatalf("release desktop embed config unexpectedly includes cold plugin %s: %s", coldPlugin, cfg)
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
	for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
		if !slices.Contains(browserRelease.GetManifests(), want) {
			t.Fatalf("browser release manifests missing %s: %v", want, browserRelease.GetManifests())
		}
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
	for _, coldPlugin := range []string{`"spacewave-notes"`, `"spacewave-v86"`} {
		if strings.Contains(browserCfg, coldPlugin) {
			t.Fatalf("browser release embed/load config unexpectedly includes cold plugin %s: %s", coldPlugin, browserCfg)
		}
	}

	pluginReleaseBrowser := result.Config.GetBuild()["plugin-release-browser"]
	if pluginReleaseBrowser == nil {
		t.Fatal("build target 'plugin-release-browser' not found")
	}
	for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
		if !slices.Contains(pluginReleaseBrowser.GetManifests(), want) {
			t.Fatalf("plugin-release-browser manifests missing %s: %v", want, pluginReleaseBrowser.GetManifests())
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
	for _, want := range []string{"spacewave-web", "spacewave-app", "spacewave-notes", "spacewave-v86"} {
		if !strings.Contains(jsManifests, want) {
			t.Fatalf("release remote js manifests missing %s: %v", want, jsBuild.GetManifests())
		}
	}

	publish := result.Config.GetPublish()["spacewave-release"]
	if publish == nil {
		t.Fatal("publish target 'spacewave-release' not found")
	}
	for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
		if !slices.Contains(publish.GetManifests(), want) {
			t.Fatalf("spacewave-release publish manifests missing %s: %v", want, publish.GetManifests())
		}
	}
}

func TestEvaluateRootDesktopStatusProjectorPlatformBoundary(t *testing.T) {
	starPath := "../../../bldr.star"
	if _, err := os.Stat(starPath); err != nil {
		t.Skipf("bldr.star not found at %s: %v", starPath, err)
	}

	result, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}

	core := result.Config.GetManifests()["spacewave-core"]
	if core == nil {
		t.Fatal("spacewave-core manifest not found")
	}
	coreConf := mustGoPluginConfig(t, core.GetBuilder().GetConfig())
	assertGoConfigOmitsDesktopStatusProjector(t, "base spacewave-core", coreConf)

	desktopConf := flattenGoConfigForPlatform(t, coreConf, "desktop/darwin/arm64")
	assertGoConfigHasDesktopStatusProjector(t, "desktop spacewave-core", desktopConf)

	webConf := flattenGoConfigForPlatform(t, coreConf, "web/js/wasm")
	assertGoConfigOmitsDesktopStatusProjector(t, "web spacewave-core", webConf)

	jsConf := flattenGoConfigForPlatform(t, coreConf, "js")
	assertGoConfigOmitsDesktopStatusProjector(t, "js spacewave-core", jsConf)

	cli := result.Config.GetManifests()["spacewave"]
	if cli == nil {
		t.Fatal("spacewave CLI manifest not found")
	}
	cliConf := mustCliConfig(t, cli.GetBuilder().GetConfig())
	assertCliConfigOmitsDesktopStatusProjector(t, "spacewave CLI", cliConf)
}

func mustGoPluginConfig(t *testing.T, data []byte) *bldr_plugin_compiler_go.Config {
	t.Helper()
	conf := bldr_plugin_compiler_go.NewConfig()
	if err := conf.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal Go plugin config: %v\n%s", err, string(data))
	}
	return conf
}

func mustCliConfig(t *testing.T, data []byte) *bldr_cli_compiler.Config {
	t.Helper()
	conf := &bldr_cli_compiler.Config{}
	if err := conf.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal CLI config: %v\n%s", err, string(data))
	}
	return conf
}

func flattenGoConfigForPlatform(
	t *testing.T,
	base *bldr_plugin_compiler_go.Config,
	platformID string,
) *bldr_plugin_compiler_go.Config {
	t.Helper()
	platform, err := bldr_platform.ParsePlatform(platformID)
	if err != nil {
		t.Fatalf("parse platform %q: %v", platformID, err)
	}
	conf := base.CloneVT()
	conf.FlattenPlatformTypes(platform)
	return conf
}

func assertGoConfigHasDesktopStatusProjector(
	t *testing.T,
	name string,
	conf *bldr_plugin_compiler_go.Config,
) {
	t.Helper()
	if !slices.Contains(conf.GetGoPkgs(), "./core/resource/desktop/statusprojector") {
		t.Fatalf("%s missing desktop status projector package: %v", name, conf.GetGoPkgs())
	}
	got := conf.GetConfigSet()["desktop-status-projector"]
	if got == nil {
		t.Fatalf("%s missing desktop-status-projector config", name)
	}
	if got.GetId() != "resource/desktop/status-projector" {
		t.Fatalf("%s desktop-status-projector config id: got %q", name, got.GetId())
	}
}

func assertGoConfigOmitsDesktopStatusProjector(
	t *testing.T,
	name string,
	conf *bldr_plugin_compiler_go.Config,
) {
	t.Helper()
	if slices.Contains(conf.GetGoPkgs(), "./core/resource/desktop/statusprojector") {
		t.Fatalf("%s unexpectedly contains desktop status projector package: %v", name, conf.GetGoPkgs())
	}
	if got := conf.GetConfigSet()["desktop-status-projector"]; got != nil {
		t.Fatalf("%s unexpectedly contains desktop-status-projector config: %v", name, got)
	}
	for key, cfg := range conf.GetConfigSet() {
		if cfg.GetId() == "resource/desktop/status-projector" {
			t.Fatalf("%s unexpectedly contains status projector config %q", name, key)
		}
	}
}

func assertCliConfigOmitsDesktopStatusProjector(
	t *testing.T,
	name string,
	conf *bldr_cli_compiler.Config,
) {
	t.Helper()
	if slices.Contains(conf.GetGoPkgs(), "./core/resource/desktop/statusprojector") {
		t.Fatalf("%s unexpectedly contains desktop status projector package: %v", name, conf.GetGoPkgs())
	}
	if got := conf.GetConfigSet()["desktop-status-projector"]; got != nil {
		t.Fatalf("%s unexpectedly contains desktop-status-projector config: %v", name, got)
	}
	for key, cfg := range conf.GetConfigSet() {
		if cfg.GetId() == "resource/desktop/status-projector" {
			t.Fatalf("%s unexpectedly contains status projector config %q", name, key)
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
