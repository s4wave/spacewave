//go:build !js

package bldr_project_starlark

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	spacewave_launcher_controller "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
)

// TestDesktopOfflineE2EConfig audits the composed fixture before its artifact
// can be used to launch a desktop daemon.
func TestDesktopOfflineE2EConfig(t *testing.T) {
	root, err := os.ReadFile("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := os.ReadFile("../../../desktop-offline-e2e.star")
	if err != nil {
		t.Fatal(err)
	}
	yaml, err := os.ReadFile("../../../desktop-offline-e2e.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(yaml) != "id: spacewave\n" {
		t.Fatalf("offline fixture YAML adds configuration: %s", yaml)
	}
	starPath := filepath.Join(t.TempDir(), "bldr.star")
	if err := os.WriteFile(starPath, append(append(root, '\n'), overlay...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}

	assets := result.Config.GetBuild()["desktop-offline-e2e-assets"]
	dist := result.Config.GetBuild()["desktop-offline-e2e-dist"]
	if assets == nil || dist == nil {
		t.Fatal("offline desktop fixture build targets are missing")
	}
	if got := assets.GetPlatformIds(); !slices.Equal(got, []string{"desktop/darwin/arm64", "js"}) {
		t.Fatalf("asset platforms: %v", got)
	}
	if got := dist.GetPlatformIds(); !slices.Equal(got, []string{"desktop/darwin/arm64"}) {
		t.Fatalf("distribution platforms: %v", got)
	}
	if got := assets.GetManifests(); !slices.Equal(got, []string{
		"spacewave-launcher", "spacewave-core", "spacewave-web",
		"spacewave-code", "spacewave-app", "web",
	}) {
		t.Fatalf("asset manifests: %v", got)
	}
	if got := dist.GetManifests(); !slices.Equal(got, []string{"spacewave-dist"}) {
		t.Fatalf("distribution manifests: %v", got)
	}

	if len(assets.GetManifestOverrides()) != 0 {
		t.Fatal("fixture assets must use the effective manifest definitions")
	}
	launcherManifest := result.Config.GetManifests()["spacewave-launcher"]
	coreManifest := result.Config.GetManifests()["spacewave-core"]
	distOverride := dist.GetManifestOverrides()["spacewave-dist"]
	if launcherManifest == nil || coreManifest == nil || distOverride == nil {
		t.Fatal("offline desktop fixture manifests are incomplete")
	}
	for name, config := range map[string][]byte{
		"launcher": launcherManifest.GetBuilder().GetConfig(),
		"core":     coreManifest.GetBuilder().GetConfig(),
		"dist":     distOverride.GetConfig(),
	} {
		if strings.Contains(string(config), "spacewave.app") || strings.Contains(string(config), "cdn.spacewave.app") {
			t.Fatalf("%s override contains a project endpoint", name)
		}
	}

	var launcher bldr_plugin_compiler_go.Config
	if err := launcher.UnmarshalJSON(launcherManifest.GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if len(launcher.GetHostConfigSet()) != 0 || launcher.GetConfigSet()["release-world"] != nil {
		t.Fatal("offline launcher mounts a Release World provider")
	}
	var fetch spacewave_launcher_controller.Config
	if err := fetch.UnmarshalJSON(launcher.GetConfigSet()["spacewave-launcher"].GetConfig()); err != nil {
		t.Fatal(err)
	}
	if !fetch.GetDisableEndpointFetch() || len(fetch.GetEndpoints()) != 0 || len(fetch.GetDistPeerIds()) != 0 {
		t.Fatalf("offline launcher can fetch a distribution: %s", launcherManifest.GetBuilder().GetConfig())
	}

	var core bldr_plugin_compiler_go.Config
	if err := core.UnmarshalJSON(coreManifest.GetBuilder().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if core.GetConfigSet()["provider-spacewave"] != nil {
		t.Fatal("offline core mounts the project cloud provider")
	}
	local := core.GetConfigSet()["provider-local"]
	if local == nil || len(local.GetConfig()) != 0 {
		t.Fatalf("offline core local provider has signaling config: %v", local)
	}

	var bundle bldr_dist_compiler.Config
	if err := bundle.UnmarshalJSON(distOverride.GetConfig()); err != nil {
		t.Fatal(err)
	}
	if got := bundle.GetLoadPlugins(); !slices.Equal(got, assets.GetManifests()) {
		t.Fatalf("startup plugins: %v", got)
	}
	assertDistEmbedManifests(t, "desktop-offline-e2e-dist", &bundle, []distEmbedManifestWant{
		{manifestID: "spacewave-launcher", platformID: "desktop/darwin/arm64"},
		{manifestID: "spacewave-core", platformID: "desktop/darwin/arm64"},
		{manifestID: "spacewave-web", platformID: "js"},
		{manifestID: "spacewave-code", platformID: "js"},
		{manifestID: "spacewave-app", platformID: "js"},
		{manifestID: "web", platformID: "desktop/darwin/arm64"},
	})
	if bundle.GetLoadWebStartup() == "" || len(bundle.GetHostConfigSet()) != 0 {
		t.Fatal("distribution startup is missing or adds a host provider")
	}
}
