//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bldr_cli_compiler "github.com/s4wave/spacewave/bldr/cli/compiler"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
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

	for _, host := range []struct {
		name       string
		platformID string
	}{
		{name: "desktop-darwin-arm64", platformID: "desktop/darwin/arm64"},
		{name: "desktop-darwin-amd64", platformID: "desktop/darwin/amd64"},
		{name: "desktop-linux-arm64", platformID: "desktop/linux/arm64"},
		{name: "desktop-linux-amd64", platformID: "desktop/linux/amd64"},
		{name: "desktop-windows-arm64", platformID: "desktop/windows/arm64"},
		{name: "desktop-windows-amd64", platformID: "desktop/windows/amd64"},
	} {
		releaseName := "release-" + host.name
		build := result.Config.GetBuild()[releaseName]
		if build == nil {
			t.Fatalf("build target %q not found", releaseName)
		}
		if got := build.GetPlatformIds(); !slices.Equal(got, []string{host.platformID}) {
			t.Fatalf("%s platform ids: got %v, want [%s]", releaseName, got, host.platformID)
		}
		if got := build.GetManifests(); !slices.Equal(got, []string{"spacewave-launcher", "spacewave-dist"}) {
			t.Fatalf("%s manifests: got %v, want launcher and shell", releaseName, got)
		}
		override := build.GetManifestOverrides()["spacewave-dist"]
		if override == nil {
			t.Fatalf("%s override for spacewave-dist not found", releaseName)
		}
		distConf := mustDistConfig(t, override.GetConfig())
		assertDistEmbedManifests(t, releaseName, distConf, []distEmbedManifestWant{{
			manifestID: "spacewave-launcher",
			platformID: host.platformID,
		}})

		pluginReleaseName := "plugin-release-" + host.name
		pluginRelease := result.Config.GetBuild()[pluginReleaseName]
		if pluginRelease == nil || !slices.Contains(pluginRelease.GetManifests(), "spacewave-loader") {
			t.Fatalf("%s does not provide native loader through Release World: %v", pluginReleaseName, pluginRelease)
		}
	}

	core := result.Config.GetManifests()["spacewave-core"]
	if core == nil {
		t.Fatal("spacewave-core manifest not found")
	}
	if got := core.GetBuilder().GetRev(); got != 13 {
		t.Fatalf("spacewave-core manifest revision: got %d, want 13", got)
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
	launcherConf := mustGoPluginConfig(t, launcher.GetBuilder().GetConfig())
	if launcherConf.GetConfigSet()["release-world-fetch"] != nil {
		t.Fatal("spacewave-launcher plugin config set mounts release-world-fetch")
	}
	if launcherConf.GetHostConfigSet()["release-world-fetch"] == nil {
		t.Fatal("spacewave-launcher host config set omits release-world-fetch")
	}
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

	cliRelease := result.Config.GetBuild()["release-cli-darwin-arm64"]
	if cliRelease == nil {
		t.Fatal("build target 'release-cli-darwin-arm64' not found")
	}
	if got := cliRelease.GetManifests(); !slices.Equal(got, []string{"spacewave-launcher", "spacewave-cli"}) {
		t.Fatalf("release cli manifests: got %v, want launcher and shell", got)
	}
	cliOverride := cliRelease.GetManifestOverrides()["spacewave-cli"]
	if cliOverride == nil {
		t.Fatal("override for 'spacewave-cli' not found")
	}
	cliCfg := string(cliOverride.GetConfig())
	for _, want := range []string{
		`"entrypointRole":"cli"`,
		`"spacewave-launcher"`,
		`"spacewave-core"`,
		`"spacewave-web"`,
		`"spacewave-app"`,
		`"web"`,
	} {
		if !strings.Contains(cliCfg, want) {
			t.Fatalf("release cli override config missing %s: %s", want, cliCfg)
		}
	}
	if strings.Contains(cliCfg, `"spacewave-loader"`) {
		t.Fatalf("release cli override unexpectedly includes desktop loader: %s", cliCfg)
	}

	browserRelease := result.Config.GetBuild()["release-web"]
	if browserRelease == nil {
		t.Fatal("build target 'release-web' not found")
	}
	browserManifests := strings.Join(browserRelease.GetManifests(), ",")
	if strings.Contains(browserManifests, "spacewave-loader") {
		t.Fatalf("browser release manifests unexpectedly include spacewave-loader: %v", browserRelease.GetManifests())
	}
	if slices.Contains(browserRelease.GetManifests(), "spacewave-dist") {
		t.Fatalf("browser release manifests unexpectedly include spacewave-dist: %v", browserRelease.GetManifests())
	}
	if got := browserRelease.GetManifests(); !slices.Equal(got, []string{"spacewave-launcher", "spacewave-browser"}) {
		t.Fatalf("browser release manifests: got %v, want launcher and shell", got)
	}
	browserLauncherOverride := browserRelease.GetManifestOverrides()["spacewave-launcher"]
	if browserLauncherOverride == nil {
		t.Fatal("production browser release should override spacewave-launcher compiler")
	}
	browserOverride := browserRelease.GetManifestOverrides()["spacewave-browser"]
	if browserOverride == nil {
		t.Fatal("override for browser 'spacewave-browser' not found")
	}
	for name, override := range map[string]string{
		"spacewave-launcher": string(browserLauncherOverride.GetConfig()),
		"spacewave-browser":  string(browserOverride.GetConfig()),
	} {
		if !strings.Contains(override, `"goCompiler":"GO_COMPILER_GOSCRIPT"`) {
			t.Fatalf("production browser release %s override should use GoScript: %s", name, override)
		}
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
	browserDistConf := mustDistConfig(t, browserOverride.GetConfig())
	assertDistBrowserStartupClosure(t, "release-web", browserDistConf, "js")

	for _, coldPlugin := range []string{`"spacewave-notes"`, `"spacewave-v86"`} {
		if strings.Contains(browserCfg, coldPlugin) {
			t.Fatalf("browser release embed/load config unexpectedly includes cold plugin %s: %s", coldPlugin, browserCfg)
		}
	}

	e2eBrowserRelease := result.Config.GetBuild()["release-web-e2e"]
	if e2eBrowserRelease == nil {
		t.Fatal("build target 'release-web-e2e' not found")
	}
	for _, want := range []string{"spacewave-launcher", "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-notes", "spacewave-cli-plugin", "web", "spacewave-browser"} {
		if !slices.Contains(e2eBrowserRelease.GetManifests(), want) {
			t.Fatalf("release-web-e2e manifests missing %s: %v", want, e2eBrowserRelease.GetManifests())
		}
	}
	for _, coldPlugin := range []string{"spacewave-v86"} {
		if slices.Contains(e2eBrowserRelease.GetManifests(), coldPlugin) {
			t.Fatalf("release-web-e2e should not build %s: %v", coldPlugin, e2eBrowserRelease.GetManifests())
		}
	}
	if !slices.Equal(e2eBrowserRelease.GetTargets(), []string{"browser"}) {
		t.Fatalf("release-web-e2e targets: got %v, want [browser]", e2eBrowserRelease.GetTargets())
	}
	e2eLauncherOverride := e2eBrowserRelease.GetManifestOverrides()["spacewave-launcher"]
	if e2eLauncherOverride == nil {
		t.Fatal("release-web-e2e should override spacewave-launcher")
	}
	e2eLauncherCfg := string(e2eLauncherOverride.GetConfig())
	for _, want := range []string{
		`"distPeerIds":["12D3KooWJPNip1SbsUG7SjteoegGjfq22D4WThuctPvCwJzHesD5"]`,
		`"initDistConfig":"QnF9fgszS28jFAQYB2t0Nx0lOx8KM1VoPCAjKShHamY9cXwMfVVQbjskPBo7R3FIMg0pPws1EkRcQlQImHAm4evJX816W1ZueiWpxzExZyZWv1gerVT0-gbD34GuNied4VzXsHGfTa6sG61t8q9C0PgfEylbZYCUALSnTFVsClHn7sqR2OZPikT405ESAbYmf7rjOJ8kw5554IZXVFPXU83lSBXRaZI9DgLim3kd90GNr2qcznKz-FKKOxfcHf-kjy-pUnR7qBQ0ZF4sCZ2zJUqe6BJL-EZIGczDuXKnli5WfY8V32P9jF2D--5UwB2ZNPU"`,
		`"disableEndpointFetch":true`,
	} {
		if !strings.Contains(e2eLauncherCfg, want) {
			t.Fatalf("release-web-e2e spacewave-launcher override missing %s: %s", want, e2eLauncherCfg)
		}
	}
	if strings.Contains(e2eLauncherCfg, `"endpoints"`) {
		t.Fatalf("release-web-e2e spacewave-launcher override contains endpoints: %s", e2eLauncherCfg)
	}
	if strings.Contains(e2eLauncherCfg, "https://spacewave.app/api/release/config") {
		t.Fatalf("release-web-e2e spacewave-launcher override contains production endpoint: %s", e2eLauncherCfg)
	}
	e2eBrowserOverride := e2eBrowserRelease.GetManifestOverrides()["spacewave-browser"]
	if e2eBrowserOverride == nil {
		t.Fatal("release-web-e2e override for 'spacewave-browser' not found")
	}
	e2eDistCfg := string(e2eBrowserOverride.GetConfig())
	for _, want := range []string{
		`"spacewave-launcher"`,
		`"spacewave-core"`,
		`"spacewave-web"`,
		`"spacewave-app"`,
		`"web"`,
	} {
		if !strings.Contains(e2eDistCfg, want) {
			t.Fatalf("release-web-e2e dist override missing %s: %s", want, e2eDistCfg)
		}
	}
	e2eDistConf := mustDistConfig(t, e2eBrowserOverride.GetConfig())
	assertDistBrowserStartupClosure(
		t,
		"release-web-e2e",
		e2eDistConf,
		"web/js/wasm",
		distEmbedManifestWant{manifestID: "spacewave-sql", platformID: "js"},
	)

	for _, unexpected := range []string{
		`"embeddedManifestOverrides"`,
		`"spacewave-notes"`,
		`"spacewave-v86"`,
		"https://spacewave.app/api/release/config",
	} {
		if strings.Contains(e2eDistCfg, unexpected) {
			t.Fatalf("release-web-e2e dist override contains %s: %s", unexpected, e2eDistCfg)
		}
	}

	e2eAssetBuild := result.Config.GetBuild()["release-web-e2e-assets"]
	if e2eAssetBuild == nil {
		t.Fatal("build target 'release-web-e2e-assets' not found")
	}
	for _, want := range []string{"spacewave-launcher", "spacewave-core", "spacewave-web", "spacewave-app", "spacewave-cli-plugin", "web"} {
		if !slices.Contains(e2eAssetBuild.GetManifests(), want) {
			t.Fatalf("release-web-e2e-assets manifests missing %s: %v", want, e2eAssetBuild.GetManifests())
		}
	}
	for _, unexpected := range []string{"spacewave-browser", "spacewave-dist"} {
		if slices.Contains(e2eAssetBuild.GetManifests(), unexpected) {
			t.Fatalf("release-web-e2e-assets should not build %s: %v", unexpected, e2eAssetBuild.GetManifests())
		}
	}
	if !slices.Equal(e2eAssetBuild.GetTargets(), []string{"browser"}) {
		t.Fatalf("release-web-e2e-assets targets: got %v, want [browser]", e2eAssetBuild.GetTargets())
	}
	if e2eAssetBuild.GetManifestOverrides()["spacewave-launcher"] == nil {
		t.Fatal("release-web-e2e-assets should override spacewave-launcher")
	}

	tinygoE2EAssetBuild := result.Config.GetBuild()["release-web-e2e-tinygo-assets"]
	if tinygoE2EAssetBuild == nil {
		t.Fatal("build target 'release-web-e2e-tinygo-assets' not found")
	}
	if !slices.Contains(tinygoE2EAssetBuild.GetManifests(), "spacewave-cli-plugin") {
		t.Fatalf("release-web-e2e-tinygo-assets manifests missing spacewave-cli-plugin: %v", tinygoE2EAssetBuild.GetManifests())
	}

	e2eDistBuild := result.Config.GetBuild()["release-web-e2e-dist"]
	if e2eDistBuild == nil {
		t.Fatal("build target 'release-web-e2e-dist' not found")
	}
	if !slices.Equal(e2eDistBuild.GetManifests(), []string{"spacewave-browser"}) {
		t.Fatalf("release-web-e2e-dist manifests: got %v, want [spacewave-browser]", e2eDistBuild.GetManifests())
	}
	if len(e2eDistBuild.GetTargets()) != 0 {
		t.Fatalf("release-web-e2e-dist targets: got %v, want none", e2eDistBuild.GetTargets())
	}
	if !slices.Equal(e2eDistBuild.GetPlatformIds(), []string{"js"}) {
		t.Fatalf("release-web-e2e-dist platformIds: got %v, want [js]", e2eDistBuild.GetPlatformIds())
	}
	e2eDistOnlyOverride := e2eDistBuild.GetManifestOverrides()["spacewave-browser"]
	if e2eDistOnlyOverride == nil {
		t.Fatal("release-web-e2e-dist should override spacewave-browser")
	}
	e2eDistOnlyConf := mustDistConfig(t, e2eDistOnlyOverride.GetConfig())
	assertDistBrowserStartupClosure(
		t,
		"release-web-e2e-dist",
		e2eDistOnlyConf,
		"web/js/wasm",
		distEmbedManifestWant{manifestID: "spacewave-sql", platformID: "js"},
	)

	pluginReleaseBrowser := result.Config.GetBuild()["plugin-release-browser"]
	if pluginReleaseBrowser == nil {
		t.Fatal("build target 'plugin-release-browser' not found")
	}
	for _, want := range []string{"spacewave-notes", "spacewave-v86"} {
		if !slices.Contains(pluginReleaseBrowser.GetManifests(), want) {
			t.Fatalf("plugin-release-browser manifests missing %s: %v", want, pluginReleaseBrowser.GetManifests())
		}
	}
	pluginReleaseOverride := pluginReleaseBrowser.GetManifestOverrides()["spacewave-core"]
	if pluginReleaseOverride == nil {
		t.Fatal("plugin-release-browser spacewave-core GoScript override not found")
	}
	pluginReleaseCoreConf := mustGoPluginConfig(t, pluginReleaseOverride.GetConfig())
	pluginReleaseJSConf := flattenGoConfigForPlatform(t, pluginReleaseCoreConf, "js")
	if pluginReleaseJSConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT {
		t.Fatalf("plugin-release-browser js spacewave-core goCompiler: got %s, want GO_COMPILER_GOSCRIPT", pluginReleaseJSConf.GetGoCompiler())
	}
	pluginReleaseCoreCfg := string(pluginReleaseOverride.GetConfig())
	if !strings.Contains(pluginReleaseCoreCfg, `"endpoint":"/"`) {
		t.Fatalf("plugin-release-browser core config should use serving-origin endpoint: %s", pluginReleaseCoreCfg)
	}
	if strings.Contains(pluginReleaseCoreCfg, `"endpoint":"https://spacewave.app"`) {
		t.Fatalf("plugin-release-browser core config uses dev fallback endpoint: %s", pluginReleaseCoreCfg)
	}

	for _, obsolete := range []string{"release-remote-web", "release-remote-js"} {
		if result.Config.GetBuild()[obsolete] != nil {
			t.Fatalf("entrypoint configuration retains ordinary plugin producer %s", obsolete)
		}
	}

	publish := result.Config.GetPublish()["spacewave-release"]
	if publish == nil {
		t.Fatal("publish target 'spacewave-release' not found")
	}
	for _, want := range []string{"spacewave-loader", "spacewave-notes", "spacewave-v86"} {
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
	if webConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_DEFAULT {
		t.Fatalf("web spacewave-core goCompiler: got %s, want GO_COMPILER_DEFAULT", webConf.GetGoCompiler())
	}

	jsConf := flattenGoConfigForPlatform(t, coreConf, "js")
	assertGoConfigOmitsDesktopStatusProjector(t, "js spacewave-core", jsConf)
	if jsConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_DEFAULT {
		t.Fatalf("js spacewave-core goCompiler: got %s, want GO_COMPILER_DEFAULT", jsConf.GetGoCompiler())
	}

	tinygoBuild := result.Config.GetBuild()["plugin-release-browser-tinygo"]
	if tinygoBuild == nil {
		t.Fatal("build target 'plugin-release-browser-tinygo' not found")
	}
	tinygoOverride := tinygoBuild.GetManifestOverrides()["spacewave-core"]
	if tinygoOverride == nil {
		t.Fatal("spacewave-core TinyGo proof override not found")
	}
	tinygoCoreConf := mustGoPluginConfig(t, tinygoOverride.GetConfig())
	tinygoWebConf := flattenGoConfigForPlatform(t, tinygoCoreConf, "web/js/wasm")
	assertGoConfigOmitsDesktopStatusProjector(t, "TinyGo web spacewave-core", tinygoWebConf)
	if tinygoWebConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO {
		t.Fatalf("TinyGo web spacewave-core goCompiler: got %s, want GO_COMPILER_TINYGO", tinygoWebConf.GetGoCompiler())
	}

	tinygoReleaseBuild := result.Config.GetBuild()["release-web-tinygo"]
	if tinygoReleaseBuild == nil {
		t.Fatal("build target 'release-web-tinygo' not found")
	}
	tinygoReleaseOverride := tinygoReleaseBuild.GetManifestOverrides()["spacewave-core"]
	if tinygoReleaseOverride == nil {
		t.Fatal("spacewave-core TinyGo release-web override not found")
	}
	tinygoReleaseCoreConf := mustGoPluginConfig(t, tinygoReleaseOverride.GetConfig())
	tinygoReleaseWebConf := flattenGoConfigForPlatform(t, tinygoReleaseCoreConf, "web/js/wasm")
	if tinygoReleaseWebConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_TINYGO {
		t.Fatalf("TinyGo release-web spacewave-core goCompiler: got %s, want GO_COMPILER_TINYGO", tinygoReleaseWebConf.GetGoCompiler())
	}
	tinygoReleaseBrowserOverride := tinygoReleaseBuild.GetManifestOverrides()["spacewave-browser"]
	if tinygoReleaseBrowserOverride == nil {
		t.Fatal("spacewave-browser TinyGo release-web override not found")
	}
	tinygoReleaseDistConf := mustDistConfig(t, tinygoReleaseBrowserOverride.GetConfig())
	assertDistBrowserStartupClosure(t, "release-web-tinygo", tinygoReleaseDistConf, "web/js/wasm")

	tinygoE2EReleaseBuild := result.Config.GetBuild()["release-web-e2e-tinygo"]
	if tinygoE2EReleaseBuild == nil {
		t.Fatal("build target 'release-web-e2e-tinygo' not found")
	}
	tinygoE2EBrowserOverride := tinygoE2EReleaseBuild.GetManifestOverrides()["spacewave-browser"]
	if tinygoE2EBrowserOverride == nil {
		t.Fatal("spacewave-browser TinyGo release-web e2e override not found")
	}
	tinygoE2EDistConf := mustDistConfig(t, tinygoE2EBrowserOverride.GetConfig())
	assertDistBrowserStartupClosure(
		t,
		"release-web-e2e-tinygo",
		tinygoE2EDistConf,
		"web/js/wasm",
		distEmbedManifestWant{manifestID: "spacewave-sql", platformID: "js"},
	)

	goscriptE2EReleaseBuild := result.Config.GetBuild()["release-web-e2e-goscript"]
	if goscriptE2EReleaseBuild == nil {
		t.Fatal("build target 'release-web-e2e-goscript' not found")
	}
	goscriptE2EReleaseOverride := goscriptE2EReleaseBuild.GetManifestOverrides()["spacewave-core"]
	if goscriptE2EReleaseOverride == nil {
		t.Fatal("spacewave-core GoScript release-web e2e override not found")
	}
	goscriptE2EReleaseCoreConf := mustGoPluginConfig(t, goscriptE2EReleaseOverride.GetConfig())
	goscriptE2EReleaseJSConf := flattenGoConfigForPlatform(t, goscriptE2EReleaseCoreConf, "js")
	if goscriptE2EReleaseJSConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT {
		t.Fatalf("GoScript release-web e2e spacewave-core js goCompiler: got %s, want GO_COMPILER_GOSCRIPT", goscriptE2EReleaseJSConf.GetGoCompiler())
	}
	assertGoConfigIncludesHTTPExport(t, "GoScript release-web e2e js spacewave-core", goscriptE2EReleaseJSConf)
	goscriptE2EReleaseLauncherOverride := goscriptE2EReleaseBuild.GetManifestOverrides()["spacewave-launcher"]
	if goscriptE2EReleaseLauncherOverride == nil {
		t.Fatal("spacewave-launcher GoScript release-web e2e override not found")
	}
	goscriptE2EReleaseLauncherConf := mustGoPluginConfig(t, goscriptE2EReleaseLauncherOverride.GetConfig())
	goscriptE2EReleaseLauncherJSConf := flattenGoConfigForPlatform(t, goscriptE2EReleaseLauncherConf, "js")
	if goscriptE2EReleaseLauncherJSConf.GetGoCompiler() != bldr_plugin_compiler_go.GoCompiler_GO_COMPILER_GOSCRIPT {
		t.Fatalf("GoScript release-web e2e spacewave-launcher js goCompiler: got %s, want GO_COMPILER_GOSCRIPT", goscriptE2EReleaseLauncherJSConf.GetGoCompiler())
	}
	assertBrowserLauncherOmitsReleaseWorld(t, "GoScript release-web e2e", goscriptE2EReleaseLauncherConf)
	goscriptE2EBrowserOverride := goscriptE2EReleaseBuild.GetManifestOverrides()["spacewave-browser"]
	if goscriptE2EBrowserOverride == nil {
		t.Fatal("spacewave-browser GoScript release-web e2e override not found")
	}
	goscriptE2EDistConf := mustDistConfig(t, goscriptE2EBrowserOverride.GetConfig())
	assertDistBrowserStartupClosure(
		t,
		"release-web-e2e-goscript",
		goscriptE2EDistConf,
		"js",
		distEmbedManifestWant{manifestID: "spacewave-sql", platformID: "js"},
	)

	cli := result.Config.GetManifests()["spacewave"]
	if cli == nil {
		t.Fatal("spacewave CLI manifest not found")
	}
	cliConf := mustCliConfig(t, cli.GetBuilder().GetConfig())
	assertCliConfigOmitsDesktopStatusProjector(t, "spacewave CLI", cliConf)
}

func mustDistConfig(t *testing.T, data []byte) *bldr_dist_compiler.Config {
	t.Helper()
	conf := &bldr_dist_compiler.Config{}
	if err := conf.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal dist config: %v\n%s", err, string(data))
	}
	return conf
}

func assertDistEmbedPlatform(
	t *testing.T,
	label string,
	conf *bldr_dist_compiler.Config,
	manifestID string,
	wantPlatformID string,
) {
	t.Helper()
	for _, embed := range conf.GetEmbedManifests() {
		if embed.GetManifestId() == manifestID {
			if got := embed.GetPlatformId(); got != wantPlatformID {
				t.Fatalf("%s embeds %s platform: got %q, want %q", label, manifestID, got, wantPlatformID)
			}
			return
		}
	}
	t.Fatalf("%s embed manifests missing %s: %v", label, manifestID, conf.GetEmbedManifests())
}

type distEmbedManifestWant struct {
	manifestID string
	platformID string
}

func assertDistBrowserStartupClosure(
	t *testing.T,
	label string,
	conf *bldr_dist_compiler.Config,
	goPlatformID string,
	additionalEmbeds ...distEmbedManifestWant,
) {
	t.Helper()

	wantLoadPlugins := []string{
		"spacewave-launcher",
		"spacewave-core",
		"spacewave-web",
		"spacewave-app",
		"web",
	}
	if got := conf.GetLoadPlugins(); !slices.Equal(got, wantLoadPlugins) {
		t.Fatalf("%s load plugins: got %v, want %v", label, got, wantLoadPlugins)
	}

	wantEmbedManifests := []distEmbedManifestWant{
		{manifestID: "spacewave-launcher", platformID: goPlatformID},
	}
	wantEmbedManifests = append(wantEmbedManifests, additionalEmbeds...)
	for _, want := range wantEmbedManifests {
		assertDistEmbedPlatform(t, label, conf, want.manifestID, want.platformID)
	}
	assertDistEmbedManifests(t, label, conf, wantEmbedManifests)
}

func assertDistEmbedManifests(
	t *testing.T,
	label string,
	conf *bldr_dist_compiler.Config,
	want []distEmbedManifestWant,
) {
	t.Helper()

	got := conf.GetEmbedManifests()
	if len(got) != len(want) {
		t.Fatalf("%s embed manifests: got %v, want %v", label, distEmbedManifestTuples(got), want)
	}
	for i, embed := range got {
		if embed.GetManifestId() != want[i].manifestID || embed.GetPlatformId() != want[i].platformID {
			t.Fatalf(
				"%s embed manifest[%d]: got %s@%s, want %s@%s",
				label,
				i,
				embed.GetManifestId(),
				embed.GetPlatformId(),
				want[i].manifestID,
				want[i].platformID,
			)
		}
	}
}

func distEmbedManifestTuples(embeds []*bldr_dist_compiler.EmbedManifest) []distEmbedManifestWant {
	tuples := make([]distEmbedManifestWant, 0, len(embeds))
	for _, embed := range embeds {
		tuples = append(tuples, distEmbedManifestWant{
			manifestID: embed.GetManifestId(),
			platformID: embed.GetPlatformId(),
		})
	}
	return tuples
}

func mustGoPluginConfig(t *testing.T, data []byte) *bldr_plugin_compiler_go.Config {
	t.Helper()
	conf := bldr_plugin_compiler_go.NewConfig()
	if err := conf.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal Go plugin config: %v\n%s", err, string(data))
	}
	return conf
}

func assertBrowserLauncherOmitsReleaseWorld(t *testing.T, label string, conf *bldr_plugin_compiler_go.Config) {
	t.Helper()
	for _, key := range []string{"release-world", "release-world-fetch", "release-world-ops"} {
		if conf.GetConfigSet()[key] != nil {
			t.Fatalf("%s launcher configSet includes %s", label, key)
		}
	}
	for _, pkg := range []string{
		"github.com/s4wave/spacewave/bldr/manifest/fetch/world",
		"github.com/s4wave/spacewave/core/cdn/world/controller",
		"github.com/s4wave/spacewave/core/space/world/optypes",
	} {
		if slices.Contains(conf.GetGoPkgs(), pkg) {
			t.Fatalf("%s launcher goPkgs includes %s", label, pkg)
		}
	}
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

func assertGoConfigIncludesHTTPExport(
	t *testing.T,
	name string,
	conf *bldr_plugin_compiler_go.Config,
) {
	t.Helper()
	if !slices.Contains(conf.GetGoPkgs(), "./core/space/http/export") {
		t.Fatalf("%s missing HTTP export package: %v", name, conf.GetGoPkgs())
	}
	if got := conf.GetConfigSet()["export"]; got == nil {
		t.Fatalf("%s missing export config", name)
	} else if got.GetId() != "space/http/export" {
		t.Fatalf("%s export config id: got %q", name, got.GetId())
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
