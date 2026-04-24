//go:build !js

package bldr_project_starlark

import (
	"os"
	"testing"
)

func TestEvaluateAlpha(t *testing.T) {
	// Locate the alpha repo bldr.star relative to GOPATH/workspace.
	// Skip if not available (CI or different machine layout).
	starPath := "../../../alpha/bldr.star"
	if _, err := os.Stat(starPath); err != nil {
		t.Skipf("alpha bldr.star not found at %s: %v", starPath, err)
	}

	result, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify project
	if result.Config.GetId() != "spacewave" {
		t.Fatalf("expected project id 'spacewave', got %q", result.Config.GetId())
	}

	// Verify start config
	start := result.Config.GetStart()
	if start == nil {
		t.Fatal("expected start config")
	}
	if len(start.GetPlugins()) == 0 {
		t.Fatal("expected start plugins")
	}
	if start.GetLoadWebStartup() != "app/prerender/startup.tsx" {
		t.Fatalf("unexpected loadWebStartup: %q", start.GetLoadWebStartup())
	}

	// Verify manifests
	expectedManifests := []string{
		"web", "spacewave-launcher", "spacewave-loader",
		"spacewave-core", "spacewave-debug", "spacewave-cli",
		"spacewave-web", "spacewave-app", "spacewave-dist",
	}
	for _, id := range expectedManifests {
		if result.Config.GetManifests()[id] == nil {
			t.Errorf("missing manifest %q", id)
		}
	}

	// Verify builds
	expectedBuilds := []string{
		"app",
		"web",
		"release-web",
		"cli",
		"plugin-release-browser",
		"release-desktop-darwin-arm64",
		"plugin-release-desktop-darwin-arm64",
		"release-remote-js",
	}
	for _, id := range expectedBuilds {
		if result.Config.GetBuild()[id] == nil {
			t.Errorf("missing build target %q", id)
		}
	}

	// Verify core manifest has configSet in builder config
	core := result.Config.GetManifests()["spacewave-core"]
	if core == nil {
		t.Fatal("spacewave-core manifest not found")
	}
	if core.GetBuilder().GetId() != "bldr/plugin/compiler/go" {
		t.Fatalf("unexpected builder: %q", core.GetBuilder().GetId())
	}
	if core.GetBuilder().GetRev() == 0 {
		t.Fatal("expected non-zero builder rev")
	}
	configData := core.GetBuilder().GetConfig()
	if len(configData) == 0 {
		t.Fatal("expected non-empty builder config for spacewave-core")
	}
	t.Logf("spacewave-core builder config length: %d bytes", len(configData))

	t.Logf("alpha bldr.star evaluated successfully: %d manifests, %d builds",
		len(result.Config.GetManifests()), len(result.Config.GetBuild()))
}
