//go:build !js

package bldr_project_starlark

import (
	"os"
	"testing"

	bldr_dist_compiler "github.com/aperturerobotics/bldr/dist/compiler"
)

func TestEvaluateBldr(t *testing.T) {
	starPath := "../../../bldr/bldr.star"
	if _, err := os.Stat(starPath); err != nil {
		t.Skipf("bldr.star not found at %s: %v", starPath, err)
	}

	result, err := Evaluate(starPath)
	if err != nil {
		t.Fatal(err)
	}

	conf := result.Config
	if conf.GetId() != "bldr-demo" {
		t.Fatalf("project id: got %q, want %q", conf.GetId(), "bldr-demo")
	}

	start := conf.GetStart()
	if start == nil {
		t.Fatal("missing start config")
	}
	if got := start.GetPlugins(); len(got) != 1 || got[0] != "bldr-demo" {
		t.Fatalf("start plugins: got %v", got)
	}

	for _, manifestID := range []string{"web", "bldr-demo", "bldr-demo-cli", "bldr-demo-release"} {
		if conf.GetManifests()[manifestID] == nil {
			t.Fatalf("missing manifest %q", manifestID)
		}
	}

	releaseWeb := conf.GetBuild()["release-web"]
	if releaseWeb == nil {
		t.Fatal("missing build target release-web")
	}
	override := releaseWeb.GetManifestOverrides()["bldr-demo-release"]
	if override == nil {
		t.Fatal("missing bldr-demo-release override on release-web")
	}
	if len(override.GetConfig()) == 0 {
		t.Fatal("release-web override config is empty")
	}

	overrideConf := &bldr_dist_compiler.Config{}
	if err := overrideConf.UnmarshalJSON(override.GetConfig()); err != nil {
		t.Fatalf("parse release-web override config: %v", err)
	}
	embedManifests := overrideConf.GetEmbedManifests()
	if len(embedManifests) != 2 {
		t.Fatalf("release-web override embeds: got %d, want 2", len(embedManifests))
	}
	if embedManifests[0].GetManifestId() != "web" || embedManifests[0].GetPlatformId() != "web/js/wasm" {
		t.Fatalf("unexpected first embed manifest: %s@%s", embedManifests[0].GetManifestId(), embedManifests[0].GetPlatformId())
	}
	if embedManifests[1].GetManifestId() != "bldr-demo" || embedManifests[1].GetPlatformId() != "web/js/wasm" {
		t.Fatalf("unexpected second embed manifest: %s@%s", embedManifests[1].GetManifestId(), embedManifests[1].GetPlatformId())
	}
}
