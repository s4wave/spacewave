//go:build !js

package bldr_project_controller

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/sirupsen/logrus"
)

func TestManifestBuilderDesktopAliasAdvancesCanonicalRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	platform := "desktop/" + runtime.GOOS + "/" + runtime.GOARCH
	meta := bldr_manifest.NewManifestMeta("app", bldr_manifest.BuildType_DEV, platform, 10)
	if _, _, err := tb.CreateManifestWithBilly(ctx, meta, "app", nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Reuse the test builder to inspect the metadata handed to compilation.
	tb.GetStaticResolver().AddFactory(manifest_builder_controller.NewFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(newOrderedFetchManifestBuilderFactory(tb.GetBus(), nil))
	project := &bldr_project.ProjectConfig{
		Id:        "canonical-platform",
		Manifests: map[string]*bldr_project.ManifestConfig{"app": makeJSManifestConfig(t, nil)},
		Remotes: map[string]*bldr_project.RemoteConfig{
			"devtool": {
				EngineId:  tb.GetWorldEngineID(),
				ObjectKey: tb.GetPluginHostObjKey(),
				PeerId:    tb.GetVolume().GetPeerID().String(),
			},
		},
	}
	source := t.TempDir()
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), NewConfig(source, source, project, true, false))
	release, err := tb.GetBus().AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	build, err := ctrl.AddManifestBuilderRef(NewManifestBuilderConfig("app", "development", "desktop", "devtool"))
	if err != nil {
		t.Fatal(err)
	}
	defer build.Release()
	result, err := build.GetResultPromiseContainer().Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conf := result.GetBuilderConfig()
	got := conf.GetManifestMeta()
	if got.GetRev() != 11 || got.GetPlatformId() != platform || got.GetBuildType() != "dev" {
		t.Fatalf("build metadata = %s", got)
	}
	if conf.GetWorkingPath() != filepath.Join(source, "build", platform, "app") {
		t.Fatalf("build directory = %q", conf.GetWorkingPath())
	}
}
