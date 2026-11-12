//go:build !js

package bldr_project_controller

import (
	"context"
	"testing"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// TestBuildTargetsManifestRevisionFloor builds through the production revision allocator.
func TestBuildTargetsManifestRevisionFloor(t *testing.T) {
	// Check fresh Worlds and the configured and stored revision floors.
	for _, tc := range []struct {
		name       string
		configured uint64
		requested  uint64
		existing   uint64
		want       uint64
		// targets exercises deployment-target expansion instead of explicit IDs.
		targets []string
		// allowed restricts the manifest's supported platforms.
		allowed []string
		// platform is the expected resolved output platform, when specified.
		platform string
	}{
		{name: "fresh", requested: 42, want: 42},
		{name: "configured floor", configured: 60, requested: 42, want: 60},
		{name: "configured default", configured: 60, want: 60},
		{name: "stored revision", requested: 42, existing: 70, want: 71},
		{name: "default", want: 0},
		{name: "native-only desktop target", targets: []string{"desktop"}, allowed: []string{"desktop"}, platform: "desktop"},
		{name: "qualified desktop target", targets: []string{"desktop/darwin/arm64"}, allowed: []string{"desktop/darwin/arm64"}, platform: "desktop/darwin/arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Use an isolated World and the production manifest builder controller.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
			if err != nil {
				t.Fatal(err)
			}
			defer tb.Release()

			// Seed only the case that must advance past a stored revision.
			const platform = "web/js/wasm"
			if tc.existing != 0 {
				meta := bldr_manifest.NewManifestMeta("app", bldr_manifest.BuildType_RELEASE, platform, tc.existing)
				if _, _, err := tb.CreateManifestWithBilly(ctx, meta, "app", nil, nil, nil); err != nil {
					t.Fatal(err)
				}
			}

			// Record compiler output without replacing the production build lifecycle.
			tb.GetStaticResolver().AddFactory(manifest_builder_controller.NewFactory(tb.GetBus()))
			tb.GetStaticResolver().AddFactory(newRecordingFetchManifestBuilderFactory(tb.GetBus(), &recordingFetchManifestBuilderState{built: make(map[string][]string)}))

			// Configure a finite build with the existing manifest revision field.
			manifest := makeJSManifestConfig(t, nil)
			manifest.Rev = tc.configured
			manifest.PlatformIds = tc.allowed
			build := &bldr_project.BuildConfig{Manifests: []string{"app"}, PlatformIds: []string{platform}}
			if len(tc.targets) != 0 {
				build.PlatformIds = nil
				build.Targets = tc.targets
			}

			// Attach the finite build to the testbed's manifest store.
			project := &bldr_project.ProjectConfig{
				Id:        "revision-floor",
				Manifests: map[string]*bldr_project.ManifestConfig{"app": manifest},
				Build: map[string]*bldr_project.BuildConfig{
					"release": build,
				},
				Remotes: map[string]*bldr_project.RemoteConfig{
					"devtool": {
						EngineId:  tb.GetWorldEngineID(),
						ObjectKey: tb.GetPluginHostObjKey(),
						PeerId:    tb.GetVolume().GetPeerID().String(),
					},
				},
			}
			source := t.TempDir()
			ctrl := NewController(tb.GetLogger(), tb.GetBus(), NewConfig(source, source, project, false, false))
			release, err := tb.GetBus().AddController(ctx, ctrl, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			// Invoke the same finite-build entry point as the build command.
			if err := ctrl.BuildTargets(ctx, "devtool", []string{"release"}, bldr_manifest.BuildType_RELEASE, nil, nil, tc.requested, nil); err != nil {
				t.Fatal(err)
			}

			// Read the completed build from its committed World provenance.
			tx, err := tb.GetWorldEngine().NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			key := bldr_manifest.NewManifestArtifactKey(&bucket.ObjectRef{BucketId: "app"})
			result, _, err := resultworld.LookupManifestBuildResult(ctx, tx, key)
			if err != nil {
				t.Fatal(err)
			}

			// Check the allocated revision and selected output platform.
			if got := result.GetManifest().GetMeta().GetRev(); got != tc.want {
				t.Fatalf("built revision = %d, want %d", got, tc.want)
			}
			if tc.platform != "" {
				expected, err := bldr_platform.ParsePlatform(tc.platform)
				if err != nil {
					t.Fatal(err)
				}
				if got := result.GetManifest().GetMeta().GetPlatformId(); got != expected.GetPlatformID() {
					t.Fatalf("built platform = %s, want %s", got, expected.GetPlatformID())
				}
			}
			if manifest.GetRev() != tc.configured {
				t.Fatalf("source configuration revision changed to %d", manifest.GetRev())
			}
		})
	}
}
