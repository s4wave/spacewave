//go:build !js

package bldr_project_controller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/sirupsen/logrus"
)

const failingFetchManifestBuilderConfigID = "test/failing-fetch-manifest-builder"

var errFailingFetchManifestBuild = errors.New("test manifest build failed")

type failingFetchManifestBuilderConfig struct{}

func (c *failingFetchManifestBuilderConfig) GetConfigID() string {
	return failingFetchManifestBuilderConfigID
}

func (c *failingFetchManifestBuilderConfig) EqualsConfig(c2 config.Config) bool {
	_, ok := c2.(*failingFetchManifestBuilderConfig)
	return ok
}

func (c *failingFetchManifestBuilderConfig) Validate() error {
	return nil
}

func (c *failingFetchManifestBuilderConfig) SizeVT() int {
	return 0
}

func (c *failingFetchManifestBuilderConfig) MarshalToSizedBufferVT(dAtA []byte) (int, error) {
	return 0, nil
}

func (c *failingFetchManifestBuilderConfig) MarshalVT() ([]byte, error) {
	return nil, nil
}

func (c *failingFetchManifestBuilderConfig) UnmarshalVT(data []byte) error {
	return nil
}

func (c *failingFetchManifestBuilderConfig) Reset() {}

func (c *failingFetchManifestBuilderConfig) MarshalJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (c *failingFetchManifestBuilderConfig) UnmarshalJSON(data []byte) error {
	return nil
}

type failingFetchManifestBuilder struct {
	*bus.BusController[*failingFetchManifestBuilderConfig]
}

func newFailingFetchManifestBuilderFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		failingFetchManifestBuilderConfigID,
		failingFetchManifestBuilderConfigID,
		controller.MustParseVersion("0.0.1"),
		"failing fetch manifest builder",
		func() *failingFetchManifestBuilderConfig { return &failingFetchManifestBuilderConfig{} },
		func(base *bus.BusController[*failingFetchManifestBuilderConfig]) (*failingFetchManifestBuilder, error) {
			return &failingFetchManifestBuilder{BusController: base}, nil
		},
	)
}

func (c *failingFetchManifestBuilder) Execute(ctx context.Context) error {
	return nil
}

func (c *failingFetchManifestBuilder) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	return nil, errFailingFetchManifestBuild
}

func (c *failingFetchManifestBuilder) SupportsStartupManifestCache() bool {
	return false
}

func (c *failingFetchManifestBuilder) GetSupportedPlatforms() []string {
	return nil
}

func TestFetchManifestPropagatesBuilderErrorInWatchMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.GetStaticResolver().AddFactory(manifest_builder_controller.NewFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(newFailingFetchManifestBuilderFactory(tb.GetBus()))

	builderControllerConfig, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &failingFetchManifestBuilderConfig{}),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	projectConfig := &bldr_project.ProjectConfig{
		Id: "test-project",
		Manifests: map[string]*bldr_project.ManifestConfig{
			"broken-plugin": {
				Builder: builderControllerConfig,
			},
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
	ctrlConf := NewConfig(sourcePath, sourcePath, projectConfig, true, false)
	ctrlConf.FetchManifestRemote = "devtool"
	projectCtrl := NewController(tb.GetLogger(), tb.GetBus(), ctrlConf)
	relProjectCtrl, err := tb.GetBus().AddController(ctx, projectCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relProjectCtrl()

	_, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		tb.GetBus(),
		bldr_manifest.NewFetchManifest(
			"broken-plugin",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_DEV},
			[]string{"web/js/wasm"},
			0,
		),
		func(isIdle bool, errs []error) (bool, error) {
			if isIdle && len(errs) != 0 {
				return false, errs[0]
			}
			return true, nil
		},
		nil,
		nil,
	)
	if ref != nil {
		defer ref.Release()
	}
	if err == nil {
		t.Fatal("expected FetchManifest to return the builder error")
	}
	if !strings.Contains(err.Error(), errFailingFetchManifestBuild.Error()) {
		t.Fatalf("expected builder error %q, got %v", errFailingFetchManifestBuild, err)
	}
}

// _ is a type assertion
var _ bldr_manifest_builder.Controller = ((*failingFetchManifestBuilder)(nil))
