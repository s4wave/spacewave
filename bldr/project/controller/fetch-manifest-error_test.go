//go:build !js

package bldr_project_controller

import (
	"context"
	"strings"
	"sync/atomic"
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

type orderedFetchManifestBuilderState struct {
	providerStarted chan struct{}
	releaseProvider chan struct{}
	consumerStarted chan struct{}
	providerDone    atomic.Bool
}

type orderedFetchManifestBuilder struct {
	*bus.BusController[*js_compiler.Config]
	state *orderedFetchManifestBuilderState
}

func newOrderedFetchManifestBuilderFactory(
	b bus.Bus,
	state *orderedFetchManifestBuilderState,
) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		js_compiler.ConfigID,
		js_compiler.ConfigID,
		controller.MustParseVersion("0.0.1"),
		"ordered fetch manifest builder",
		func() *js_compiler.Config { return &js_compiler.Config{} },
		func(base *bus.BusController[*js_compiler.Config]) (*orderedFetchManifestBuilder, error) {
			return &orderedFetchManifestBuilder{BusController: base, state: state}, nil
		},
	)
}

func (c *orderedFetchManifestBuilder) Execute(ctx context.Context) error {
	return nil
}

func (c *orderedFetchManifestBuilder) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	meta := args.GetBuilderConfig().GetManifestMeta().CloneVT()
	switch meta.GetManifestId() {
	case "provider":
		close(c.state.providerStarted)
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-c.state.releaseProvider:
		}
		c.state.providerDone.Store(true)
	case "consumer":
		if !c.state.providerDone.Load() {
			return nil, errors.New("consumer started before provider completed")
		}
		close(c.state.consumerStarted)
	}
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/"+meta.GetManifestId()),
		&bucket.ObjectRef{BucketId: meta.GetManifestId()},
		bldr_manifest_builder.NewInputManifest(nil, nil),
	), nil
}

func (c *orderedFetchManifestBuilder) SupportsStartupManifestCache() bool {
	return false
}

func (c *orderedFetchManifestBuilder) GetSupportedPlatforms() []string {
	return nil
}

func TestAddFetchManifestBuilderRefWaitsForWebPkgProviders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	state := &orderedFetchManifestBuilderState{
		providerStarted: make(chan struct{}),
		releaseProvider: make(chan struct{}),
		consumerStarted: make(chan struct{}),
	}
	tb.GetStaticResolver().AddFactory(manifest_builder_controller.NewFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(newOrderedFetchManifestBuilderFactory(tb.GetBus(), state))

	projectConfig := &bldr_project.ProjectConfig{
		Id: "test-project",
		Manifests: map[string]*bldr_project.ManifestConfig{
			"provider": makeJSManifestConfig(t, nil),
			"consumer": makeJSManifestConfig(t, nil),
		},
		Remotes: map[string]*bldr_project.RemoteConfig{
			"devtool": {
				EngineId:  tb.GetWorldEngineID(),
				ObjectKey: tb.GetPluginHostObjKey(),
				PeerId:    tb.GetVolume().GetPeerID().String(),
			},
		},
	}
	projectConfig.Manifests["provider"] = makeJSManifestConfig(
		t,
		[]*bldr_web_bundler.WebPkgRefConfig{{Id: "@pkg/shared"}},
	)
	projectConfig.Manifests["consumer"] = makeJSManifestConfig(
		t,
		[]*bldr_web_bundler.WebPkgRefConfig{{Id: "@pkg/shared", Exclude: true}},
	)

	sourcePath := t.TempDir()
	ctrlConf := NewConfig(sourcePath, sourcePath, projectConfig, true, false)
	ctrlConf.FetchManifestRemote = "devtool"
	projectCtrl := NewController(tb.GetLogger(), tb.GetBus(), ctrlConf)
	relProjectCtrl, err := tb.GetBus().AddController(ctx, projectCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relProjectCtrl()

	type fetchResult struct {
		builderRef *ManifestBuilderRef
		remoteRef  *RemoteRef
		err        error
	}
	fetchCh := make(chan fetchResult, 1)
	go func() {
		builderRef, remoteRef, err := projectCtrl.AddFetchManifestBuilderRef(
			ctx,
			bldr_manifest.NewManifestMeta(
				"consumer",
				bldr_manifest.BuildType_DEV,
				"web/js/wasm",
				0,
			),
		)
		fetchCh <- fetchResult{builderRef: builderRef, remoteRef: remoteRef, err: err}
	}()

	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-state.providerStarted:
	}
	select {
	case result := <-fetchCh:
		t.Fatalf("consumer fetch returned before provider completed: %v", result.err)
	default:
	}
	close(state.releaseProvider)

	var result fetchResult
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case result = <-fetchCh:
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.builderRef.Release()
	defer result.remoteRef.Release()
	if _, err := result.builderRef.GetResultPromiseContainer().Await(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-state.consumerStarted:
	}
}

// _ is a type assertion
var (
	_ bldr_manifest_builder.Controller = (*failingFetchManifestBuilder)(nil)
	_ bldr_manifest_builder.Controller = (*orderedFetchManifestBuilder)(nil)
)
