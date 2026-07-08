//go:build !skip_e2e && !js

package s4wave_core_e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/fsutil"
	"github.com/pkg/errors"
	bldr_manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	"github.com/s4wave/spacewave/bldr/testbed"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	s4wave_core_e2e "github.com/s4wave/spacewave/core/e2e"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

const (
	coreE2ETestProviderQuickstartDrive    = "provider"
	coreE2ETestHashObjectRefAndValidation = "hash"
	coreE2ETestTypedObjectLayoutAccess    = "typedObject"
)

var coreE2E *coreE2EFixture

type coreE2EFixture struct {
	cancel                context.CancelFunc
	testbedResourceServer *resource_testbed.TestbedResourceServer
	release               []func()
}

func TestMain(m *testing.M) {
	if os.Getenv("RUN_CORE_E2E") == "" {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithCancel(context.Background())
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	fixture, err := newCoreE2EFixture(ctx, cancel, le)
	if err != nil {
		os.Stderr.WriteString("core e2e setup failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	coreE2E = fixture

	code := m.Run()
	fixture.Close()
	os.Exit(code)
}

// TIER: pr
func TestSpacewaveCoreE2EProviderQuickstartDrive(t *testing.T) {
	runCoreE2ETest(t, coreE2ETestProviderQuickstartDrive)
}

// TIER: pr
func TestSpacewaveCoreE2EHashObjectRefAndValidation(t *testing.T) {
	runCoreE2ETest(t, coreE2ETestHashObjectRefAndValidation)
}

// TIER: pr
func TestSpacewaveCoreE2ETypedObjectLayoutAccess(t *testing.T) {
	runCoreE2ETest(t, coreE2ETestTypedObjectLayoutAccess)
}

func runCoreE2ETest(t *testing.T, testName string) {
	if os.Getenv("RUN_CORE_E2E") == "" {
		t.Skip("set RUN_CORE_E2E=1 to run the core E2E test")
	}

	t.Parallel()

	if coreE2E == nil {
		t.Fatal("core e2e fixture was not initialized")
	}

	success, errorMsg, err := coreE2E.testbedResourceServer.RunTest(t.Context(), testName)
	if err != nil {
		t.Fatalf("error waiting for %s test result: %v", testName, err)
	}
	if !success {
		t.Fatalf("%s test failed: %s", testName, errorMsg)
	}
}

func newCoreE2EFixture(
	ctx context.Context,
	cancel context.CancelFunc,
	le *logrus.Entry,
) (_ *coreE2EFixture, retErr error) {
	fixture := &coreE2EFixture{
		cancel: cancel,
	}
	defer func() {
		if retErr != nil {
			fixture.Close()
		}
	}()

	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "get working directory")
	}
	repoRoot := filepath.Join(wd, "../..")
	workDir := filepath.Join(wd, ".bldr")
	buildDir := filepath.Join(workDir, "build")
	distDir := filepath.Join(workDir, "src")
	pluginStateDir := filepath.Join(workDir, "plugin", "state")
	pluginDistDir := filepath.Join(workDir, "plugin", "dist")

	if err := fsutil.CleanCreateDir(buildDir); err != nil {
		return nil, errors.Wrap(err, "clean build directory")
	}
	if err := fsutil.CleanCreateDir(pluginStateDir); err != nil {
		return nil, errors.Wrap(err, "clean plugin state directory")
	}
	if err := fsutil.CleanCreateDir(pluginDistDir); err != nil {
		return nil, errors.Wrap(err, "clean plugin dist directory")
	}

	if err := s4wave_core_e2e.CheckoutWebDistSources(ctx, le, repoRoot, distDir); err != nil {
		return nil, errors.Wrap(err, "checkout web dist sources")
	}

	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		return nil, errors.Wrap(err, "build bldr testbed")
	}
	fixture.release = append(fixture.release, tb.Release)

	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_manifest_builder_controller.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_go.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_js.NewFactory(b))
	sr.AddFactory(bldr_web_bundler_vite_compiler.NewFactory(b))
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	testbedResourceServer := resource_testbed.NewTestbedResourceServer(
		ctx,
		le,
		b,
		tb.GetVolume().GetID(),
		"e2e-testbed-bucket",
	)
	fixture.testbedResourceServer = testbedResourceServer

	// The JS plugin reaches this via bus fallback: hostMux has its own
	// ResourceServer, so wrapping in ResourceServer here would be shadowed.
	if err := testbedResourceServer.Register(tb.GetMux()); err != nil {
		return nil, errors.Wrap(err, "register testbed resource server")
	}

	volPeer, err := tb.GetVolume().GetPeer(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "get volume peer")
	}
	peerCtrl := peer_controller.NewController(le, volPeer)
	relPeerCtrl, err := tb.GetBus().AddController(ctx, peerCtrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "add peer controller")
	}
	fixture.release = append(fixture.release, relPeerCtrl)

	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	relObjectTypeCtrl, err := tb.GetBus().AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "add object type controller")
	}
	fixture.release = append(fixture.release, relObjectTypeCtrl)

	_, _, processRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_process.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_process.NewConfig(pluginStateDir, pluginDistDir)),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start process plugin host")
	}
	fixture.release = append(fixture.release, processRef.Release)

	_, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start quickjs plugin host")
	}
	fixture.release = append(fixture.release, quickjsHostRef.Release)

	projectConfig, err := s4wave_core_e2e.LoadProjectConfig(repoRoot)
	if err != nil {
		return nil, errors.Wrap(err, "load project config")
	}
	if err := projectConfig.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate project config")
	}

	projectConfig.Remotes = map[string]*bldr_project.RemoteConfig{
		"devtool": {
			EngineId:       tb.GetWorldEngineID(),
			PeerId:         tb.GetVolume().GetPeerID().String(),
			ObjectKey:      tb.GetPluginHostObjKey(),
			LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		},
	}

	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, workDir, projectConfig, false, true)
	projCtrlConf.FetchManifestRemote = "devtool"

	_, _, projCtrlRef, err := loader.WaitExecControllerRunningTyped[*bldr_project_controller.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "start project controller")
	}
	fixture.release = append(fixture.release, projCtrlRef.Release)

	go func() {
		err := tb.GetScheduler().WaitPluginsRunning(ctx, projectConfig.GetStart().GetPlugins())
		if err != nil {
			if ctx.Err() == nil {
				testbedResourceServer.FailQueuedTests(false, "error waiting for startup plugins: "+err.Error())
			}
			return
		}
		le.Info("startup plugins running")
	}()

	return fixture, nil
}

func (f *coreE2EFixture) Close() {
	if f.testbedResourceServer != nil {
		f.testbedResourceServer.CloseTestQueue()
	}
	for i := len(f.release) - 1; i >= 0; i-- {
		f.release[i]()
	}
	f.release = nil
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
}
