package plugin_host_scheduler

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestExecPluginHoldsDeclaredDeps runs an instanced consumer whose manifest
// declares a dependency and proves the dependency is loaded while the consumer
// runs and released when the consumer stops. The host receives the logical
// instance key for the plugin and the execution key for its worker.
func TestExecPluginHoldsDeclaredDeps(t *testing.T) {
	// Bind the test context and logger.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Open a testbed for the consumer and its dependency.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build a mock World state over a testbed cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write the consumer manifest with its declared dependency.
	const platformID = "desktop/darwin/arm64"
	distFS := memfs.New()
	if err := billy_util.WriteFile(distFS, "consumer.js", []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	manifest := bldr_manifest.NewManifest(
		bldr_manifest.NewManifestMeta("consumer", bldr_manifest.BuildType_DEV, platformID, 1),
		"consumer.js",
	)
	manifest.Deps = []string{"provider"}
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifest, distFS, nil, timestamppb.Now())
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Register the dependency provider that signals load and release.
	provider := &testDepProvider{
		pluginID: "provider",
		loaded:   make(chan struct{}),
		released: make(chan struct{}),
	}
	providerRel, err := tb.Bus.AddController(ctx, provider, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer providerRel()

	// Register the plugin host controller that records execution keys.
	host := &blockingPluginHost{
		id:      platformID,
		started: make(chan struct{}),
	}
	hostRel, err := tb.Bus.AddController(ctx, plugin_host_controller.NewController(
		le,
		tb.Bus,
		controller.NewInfo("test/plugin-host", controller.MustParseVersion("0.0.1"), ""),
		host,
	), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer hostRel()

	// Build the consumer plugin instance wired to the testbed bus.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			worldStateCtr: ccontainer.NewCContainer(wsv),
			hostVolumeCtr: ccontainer.NewCContainer(&hostVol{
				vol:  tb.Volume,
				info: &volume.VolumeInfo{VolumeId: tb.Volume.GetID()},
			}),
			pluginStatusCtr: ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
			pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
		},
		le:               le,
		pluginID:         "consumer",
		instanceKey:      "vm-1/generation/root",
		bindingKey:       "vm-1",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(nil, bldr_plugin.InitialCapabilityRegistrationPending),
		),
	}

	// Start the consumer execution in the background.
	execCtx, cancelExec := context.WithCancel(ctx)
	defer cancelExec()
	execErr := make(chan error, 1)
	go func() {
		execErr <- pi.execPlugin(execCtx, &executePluginArgs{
			manifestSnapshot: &bldr_manifest.ManifestSnapshot{
				ManifestRef: manifestRef,
				Manifest:    manifest,
			},
			pluginHost: host,
		})
	}()

	// Wait for the consumer to start and the dependency to load.
	for _, step := range []struct {
		name string
		ch   <-chan struct{}
	}{
		{"consumer start", host.started},
		{"dependency load", provider.loaded},
	} {
		select {
		case <-step.ch:
		case err := <-execErr:
			t.Fatalf("execPlugin exited before %s: %v", step.name, err)
		case <-ctx.Done():
			t.Fatalf("waiting for %s: %v", step.name, ctx.Err())
		}
	}

	// The host received the logical instance key and the execution key.
	if host.instanceKey != "vm-1" || host.executionKey != "vm-1/generation/root" {
		t.Fatalf("host keys = (%q, %q), want (\"vm-1\", \"vm-1/generation/root\")", host.instanceKey, host.executionKey)
	}

	// Stop the consumer and confirm the dependency is released.
	cancelExec()
	select {
	case <-provider.released:
	case <-ctx.Done():
		t.Fatalf("dependency was not released after the consumer stopped: %v", ctx.Err())
	}
	<-execErr
}

// testDepProvider resolves LoadPlugin for one plugin ID and reports when the
// directive starts and stops demanding it.
type testDepProvider struct {
	pluginID string
	loaded   chan struct{}
	released chan struct{}
}

func (p *testDepProvider) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/dep-provider", controller.MustParseVersion("0.0.1"), "")
}

func (p *testDepProvider) Execute(ctx context.Context) error {
	return nil
}

func (p *testDepProvider) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || d.LoadPluginID() != p.pluginID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, _ directive.ResolverHandler) error {
		close(p.loaded)
		<-ctx.Done()
		close(p.released)
		return nil
	}), nil)
}

func (p *testDepProvider) Close() error {
	return nil
}

// blockingPluginHost runs each plugin until its context ends.
type blockingPluginHost struct {
	testPluginHost
	started chan struct{}

	// instanceKey and executionKey are the keys of the last execution.
	// Written before started closes.
	instanceKey, executionKey string
}

func (h *blockingPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	h.instanceKey, h.executionKey = instanceKey, executionKey
	close(h.started)
	<-ctx.Done()
	return context.Canceled
}
