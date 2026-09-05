package bldr_manifest_builder_controller

import (
	"os"
	"path/filepath"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// TestStartupCacheRejectsDifferentBuildIdentity prevents cached development or
// cross-platform output from satisfying a different requested build.
func TestStartupCacheRejectsDifferentBuildIdentity(t *testing.T) {
	for _, field := range []string{"manifest", "build", "platform"} {
		t.Run(field, func(t *testing.T) {
			// Create valid cached input whose only mismatch is its build identity.
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cached := buildStartupBuilderResult(t, directory, newTestBuilderControllerProto(t))
			meta := cached.GetManifest().GetMeta()
			switch field {
			case "manifest":
				meta.ManifestId = "another-plugin"
			case "build":
				meta.BuildType = bldr_manifest.BuildType_RELEASE.String()
			case "platform":
				meta.PlatformId = "desktop/windows/amd64"
			}
			cached.ManifestRef.Meta = meta.CloneVT()

			// A mismatched cached result must run the compiler for the requested target.
			result, builds := runStartupExecuteTest(t, directory, cached, true, nil)
			if builds != 1 {
				t.Fatalf("compiler calls = %d, want 1", builds)
			}
			want := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
			if !result.GetManifest().GetMeta().EqualVT(want) {
				t.Fatal("compiler did not return the requested build identity")
			}
		})
	}
}
