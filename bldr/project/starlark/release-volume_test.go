//go:build !js

package bldr_project_starlark

import (
	"slices"
	"testing"
)

// TestReleaseEntrypointsEmbedVolume checks the actual release build overrides:
// installers and native updates carry the executable without a volume sidecar.
func TestReleaseEntrypointsEmbedVolume(t *testing.T) {
	// Evaluate the product configuration through the same loader as Bldr.
	result, err := Evaluate("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}

	// Every supported desktop and CLI package must retain its bootstrap World.
	for _, platform := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"} {
		for _, target := range []struct {
			prefix     string
			manifestID string
		}{
			{"release-desktop-", "spacewave-dist"},
			{"release-cli-", "spacewave-cli"},
		} {
			buildID := target.prefix + platform
			t.Run(buildID, func(t *testing.T) {
				build := result.Config.GetBuild()[buildID]
				if build == nil {
					t.Fatalf("release target %s is missing", buildID)
				}
				override := build.GetManifestOverrides()[target.manifestID]
				if override == nil {
					t.Fatalf("release target %s has no %s override", buildID, target.manifestID)
				}
				config := mustDistConfig(t, override.GetConfig())
				if !config.GetEmbedNativeVolume().IsEnabled(false) {
					t.Fatal("release executable omits the bootstrap volume required without a sidecar")
				}
			})
		}
	}
}

// TestNativePluginReleaseIncludesWebHost prevents a published desktop from
// waiting forever for an Electron host absent from the Release World.
func TestNativePluginReleaseIncludesWebHost(t *testing.T) {
	result, err := Evaluate("../../../bldr.star")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"} {
		buildID := "plugin-release-desktop-" + platform
		if !slices.Contains(result.Config.GetBuild()[buildID].GetManifests(), "web") {
			t.Fatalf("%s omits the native web host", buildID)
		}
	}
}
