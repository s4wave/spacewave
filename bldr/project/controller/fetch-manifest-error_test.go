//go:build !js

package bldr_project_controller

import (
	"context"
	"slices"
	"strings"
	"sync"
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
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	"github.com/s4wave/spacewave/db/bucket"
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

// recordingFetchManifestBuilderState records the builds started by a
// FetchManifest request.
type recordingFetchManifestBuilderState struct {
	mtx   sync.Mutex
	built map[string][]string
}

// recordingFetchManifestBuilder records each manifest ID and its dependencies.
type recordingFetchManifestBuilder struct {
	*bus.BusController[*js_compiler.Config]
	state *recordingFetchManifestBuilderState
}

func newRecordingFetchManifestBuilderFactory(
	b bus.Bus,
	state *recordingFetchManifestBuilderState,
) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		js_compiler.ConfigID,
		js_compiler.ConfigID,
		controller.MustParseVersion("0.0.1"),
		"recording fetch manifest builder",
		func() *js_compiler.Config { return &js_compiler.Config{} },
		func(base *bus.BusController[*js_compiler.Config]) (*recordingFetchManifestBuilder, error) {
			return &recordingFetchManifestBuilder{BusController: base, state: state}, nil
		},
	)
}

func (c *recordingFetchManifestBuilder) Execute(ctx context.Context) error {
	return nil
}

func (c *recordingFetchManifestBuilder) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	builderConfig := args.GetBuilderConfig()
	meta := builderConfig.GetManifestMeta().CloneVT()
	c.state.mtx.Lock()
	c.state.built[meta.GetManifestId()] = builderConfig.GetDeps()
	c.state.mtx.Unlock()
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/"+meta.GetManifestId()),
		&bucket.ObjectRef{BucketId: meta.GetManifestId()},
		bldr_manifest_builder.NewInputManifest(nil, nil),
	), nil
}

func (c *recordingFetchManifestBuilder) SupportsStartupManifestCache() bool {
	return false
}

func (c *recordingFetchManifestBuilder) GetSupportedPlatforms() []string {
	return nil
}

// A web package consumer references its provider only by package ID, so its
// build neither starts nor waits for the provider build. The plugin host loads
// the provider from the recorded dependency.
func TestAddFetchManifestBuilderRefBuildsWebPkgConsumerAlone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	state := &recordingFetchManifestBuilderState{built: make(map[string][]string)}
	tb.GetStaticResolver().AddFactory(manifest_builder_controller.NewFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(newRecordingFetchManifestBuilderFactory(tb.GetBus(), state))

	projectConfig := &bldr_project.ProjectConfig{
		Id: "test-project",
		Manifests: map[string]*bldr_project.ManifestConfig{
			"provider": makeJSManifestConfig(
				t,
				[]*bldr_web_bundler.WebPkgRefConfig{{Id: "@pkg/shared"}},
			),
			"consumer": makeJSManifestConfig(
				t,
				[]*bldr_web_bundler.WebPkgRefConfig{{Id: "@pkg/shared", Exclude: true}},
			),
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

	builderRef, remoteRef, err := projectCtrl.AddFetchManifestBuilderRef(
		ctx,
		bldr_manifest.NewManifestMeta("consumer", bldr_manifest.BuildType_DEV, "web/js/wasm", 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer builderRef.Release()
	defer remoteRef.Release()
	if _, err := builderRef.GetResultPromiseContainer().Await(ctx); err != nil {
		t.Fatal(err)
	}

	state.mtx.Lock()
	defer state.mtx.Unlock()
	if _, ok := state.built["provider"]; ok {
		t.Fatal("consumer fetch built its provider")
	}
	if deps := state.built["consumer"]; !slices.Equal(deps, []string{"provider"}) {
		t.Fatalf("consumer deps = %v, want [provider]", deps)
	}
}

// _ is a type assertion
var (
	_ bldr_manifest_builder.Controller = (*failingFetchManifestBuilder)(nil)
	_ bldr_manifest_builder.Controller = (*recordingFetchManifestBuilder)(nil)
)
