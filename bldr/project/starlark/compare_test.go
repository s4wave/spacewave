//go:build !js

package bldr_project_starlark

import (
	"os"
	"testing"
)

func TestAlphaStarCompleteness(t *testing.T) {
	starPath := "../../../alpha/bldr.star"
	if _, err := os.Stat(starPath); err != nil {
		t.Skipf("alpha bldr.star not found: %v", err)
	}
	starResult, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}
	conf := starResult.Config

	// Verify project
	if conf.GetId() != "spacewave" {
		t.Errorf("project id: got %q, want 'spacewave'", conf.GetId())
	}

	// Verify all expected manifests exist with correct builders.
	expectedManifests := map[string]string{
		"web":                "bldr/web/plugin/compiler",
		"spacewave-launcher": "bldr/plugin/compiler/go",
		"spacewave-loader":   "bldr/plugin/compiler/go",
		"spacewave-core":     "bldr/plugin/compiler/go",
		"spacewave-debug":    "bldr/plugin/compiler/go",
		"spacewave-cli":      "bldr/cli/compiler",
		"spacewave-web":      "bldr/plugin/compiler/js",
		"spacewave-app":      "bldr/plugin/compiler/js",
		"spacewave-dist":     "bldr/dist/compiler",
	}
	for id, wantBuilder := range expectedManifests {
		mc := conf.GetManifests()[id]
		if mc == nil {
			t.Errorf("missing manifest %q", id)
			continue
		}
		if mc.GetBuilder().GetId() != wantBuilder {
			t.Errorf("manifest %q builder: got %q, want %q", id, mc.GetBuilder().GetId(), wantBuilder)
		}
	}

	// Verify all expected builds exist.
	expectedBuilds := map[string]int{
		"app":                                 5,
		"web":                                 5,
		"release-web":                         6,
		"cli":                                 2,
		"plugin-release-browser":              4,
		"release-desktop-darwin-arm64":        7,
		"plugin-release-desktop-darwin-arm64": 1,
		"release-remote-js":                   2,
	}
	for id, wantManifests := range expectedBuilds {
		bc := conf.GetBuild()[id]
		if bc == nil {
			t.Errorf("missing build %q", id)
			continue
		}
		if len(bc.GetManifests()) != wantManifests {
			t.Errorf("build %q manifests: got %d, want %d", id, len(bc.GetManifests()), wantManifests)
		}
	}

	// Verify start config.
	start := conf.GetStart()
	if start == nil {
		t.Fatal("missing start config")
	}
	if len(start.GetPlugins()) != 5 {
		t.Errorf("start plugins: got %d, want 5", len(start.GetPlugins()))
	}
	if start.GetLoadWebStartup() != "app/prerender/startup.tsx" {
		t.Errorf("start loadWebStartup: got %q", start.GetLoadWebStartup())
	}

	// Verify core manifest has non-empty builder config (configSet, goPkgs).
	core := conf.GetManifests()["spacewave-core"]
	if core != nil && len(core.GetBuilder().GetConfig()) == 0 {
		t.Error("spacewave-core builder config is empty")
	}
}
