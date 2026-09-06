//go:build !js

package bldr_project_controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/enabled"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/sirupsen/logrus"
)

// newFetchBuildConfigTestController starts the project with a real remote world.
func newFetchBuildConfigTestController(t *testing.T) (context.Context, *Controller) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// Builder selection uses the same remote readiness path as a dist build.
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	projectConfig := &bldr_project.ProjectConfig{
		Id: "fetch-build-config",
		Manifests: map[string]*bldr_project.ManifestConfig{
			"app": makeJSManifestConfig(t, nil),
		},
		Remotes: map[string]*bldr_project.RemoteConfig{
			"devtool": {
				EngineId:  tb.GetWorldEngineID(),
				ObjectKey: tb.GetPluginHostObjKey(),
				PeerId:    tb.GetVolume().GetPeerID().String(),
			},
		},
	}
	sourcePath := t.TempDir()
	conf := NewConfig(sourcePath, sourcePath, projectConfig, true, false)
	conf.FetchManifestRemote = "devtool"
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), conf)
	release, err := tb.GetBus().AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return ctx, ctrl
}

// TestFetchManifestUsesActiveBuildConfiguration preserves target overrides and policy.
func TestFetchManifestUsesActiveBuildConfiguration(t *testing.T) {
	ctx, ctrl := newFetchBuildConfigTestController(t)

	// Keep the configured build active while its dependency tuple is fetched.
	conf := NewManifestBuilderConfigWithTargetPlatforms("app", "release", "js", "devtool", []string{"js"})
	override, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, &js_compiler.Config{WebPluginId: "configured"}), false)
	if err != nil {
		t.Fatal(err)
	}
	conf.BuilderConfigOverride = override
	conf.BuildPolicy = manifest_build.NewBuildPolicy(enabled.Enabled_DISABLE, enabled.Enabled_ENABLE, enabled.Enabled_ENABLE)
	active, err := ctrl.AddManifestBuilderRef(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Release()

	// A dist dependency must receive that build, including its nondefault inputs.
	fetched, remote, err := ctrl.AddFetchManifestBuilderRef(ctx, bldr_manifest.NewManifestMeta("app", bldr_manifest.BuildType_RELEASE, "js", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer fetched.Release()
	defer remote.Release()
	got := fetched.GetManifestBuilderConfig()
	if !got.GetBuilderConfigOverride().EqualVT(conf.GetBuilderConfigOverride()) {
		t.Fatalf("dependency lost its active builder override: %s", got.GetBuilderConfigOverride())
	}
	if !got.GetBuildPolicy().EqualVT(conf.GetBuildPolicy()) {
		t.Fatalf("dependency lost its active build policy: %s", got.GetBuildPolicy())
	}
}

// TestFetchManifestRejectsConflictingActiveBuilds requires an unambiguous tuple.
func TestFetchManifestRejectsConflictingActiveBuilds(t *testing.T) {
	ctx, ctrl := newFetchBuildConfigTestController(t)

	// Both builds describe the same tuple but different executable inputs.
	for _, webPluginID := range []string{"first", "second"} {
		conf := NewManifestBuilderConfig("app", "release", "js", "devtool")
		override, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(1, &js_compiler.Config{WebPluginId: webPluginID}), false)
		if err != nil {
			t.Fatal(err)
		}
		conf.BuilderConfigOverride = override
		active, err := ctrl.AddManifestBuilderRef(conf)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(active.Release)
	}

	// Fetching a default would silently discard both declared configurations.
	fetched, remote, err := ctrl.AddFetchManifestBuilderRef(ctx, bldr_manifest.NewManifestMeta("app", bldr_manifest.BuildType_RELEASE, "js", 0))
	if fetched != nil {
		defer fetched.Release()
	}
	if remote != nil {
		defer remote.Release()
	}
	if err == nil || !strings.Contains(err.Error(), "conflicting active builds") {
		t.Fatalf("fetch error = %v, want conflicting active builds", err)
	}
}
