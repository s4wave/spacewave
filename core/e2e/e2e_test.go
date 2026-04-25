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

func TestSpacewaveCoreE2E(t *testing.T) {
	if os.Getenv("RUN_CORE_E2E") == "" {
		t.Skip("set RUN_CORE_E2E=1 to run the core E2E test")
	}

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// get path to repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}
	repoRoot := filepath.Join(wd, "../..")
	workDir := filepath.Join(wd, ".bldr")
	buildDir := filepath.Join(workDir, "build")
	distDir := filepath.Join(workDir, "src")
	pluginStateDir := filepath.Join(workDir, "plugin", "state")
	pluginDistDir := filepath.Join(workDir, "plugin", "dist")

	// cleanup the build dir if it exists
	if err := fsutil.CleanCreateDir(buildDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginStateDir); err != nil {
		t.Fatal(err.Error())
	}
	if err := fsutil.CleanCreateDir(pluginDistDir); err != nil {
		t.Fatal(err.Error())
	}

	// check out the web dist sources
	err = s4wave_core_e2e.CheckoutWebDistSources(ctx, le, distDir)
	if err != nil {
		t.Fatal(err.Error())
	}

	// build the bldr testbed
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// add the controllers we will need
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

	// create testbed resource server
	volumeID := tb.GetVolume().GetID()
	bucketID := "e2e-testbed-bucket"
	testbedResourceServer := resource_testbed.NewTestbedResourceServer(
		ctx,
		le,
		b,
		volumeID,
		bucketID,
	)

	// register testbed resource service directly on testbed mux
	// the JS plugin reaches this via bus fallback: hostMux has its own
	// ResourceServer (from scheduler), so wrapping in ResourceServer here
	// would be shadowed. Direct registration is reachable because hostMux
	// falls through to LookupRpcService for unknown services.
	if err := testbedResourceServer.Register(tb.GetMux()); err != nil {
		t.Fatal(err.Error())
	}

	// start a peer controller to serve GetPeer directives
	volPeer, err := tb.GetVolume().GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	peerCtrl := peer_controller.NewController(le, volPeer)
	relPeerCtrl, err := tb.GetBus().AddController(ctx, peerCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relPeerCtrl()

	// start objecttype controller to resolve LookupObjectType directives
	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	relObjectTypeCtrl, err := tb.GetBus().AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relObjectTypeCtrl()

	// load the go plugin host
	processHost, _, processRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_process.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_process.NewConfig(pluginStateDir, pluginDistDir)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer processRef.Release()
	_ = processHost

	// load the js plugin host
	quickjsHost, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer quickjsHostRef.Release()
	_ = quickjsHost

	// load the merged project config
	projectConfig, err := s4wave_core_e2e.LoadProjectConfig(repoRoot)
	if err == nil {
		err = projectConfig.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply the devtool remote for building manifests
	// see devtool/bus.go in bldr
	projectConfig.Remotes = map[string]*bldr_project.RemoteConfig{
		"devtool": {
			EngineId:       tb.GetWorldEngineID(),
			PeerId:         tb.GetVolume().GetPeerID().String(),
			ObjectKey:      tb.GetPluginHostObjKey(),
			LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		},
	}

	// configure the project controller
	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, workDir, projectConfig, false, true)
	projCtrlConf.FetchManifestRemote = "devtool"

	// run the project controller, which also compiles and starts the plugins
	projCtrl, _, projCtrlRef, err := loader.WaitExecControllerRunningTyped[*bldr_project_controller.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer projCtrlRef.Release()
	_ = projCtrl

	// wait for the test to complete and get the result
	le.Info("waiting for test to complete...")
	success, errorMsg, err := testbedResourceServer.WaitForTestResult(ctx)
	if err != nil {
		t.Fatalf("error waiting for test result: %v", err)
	}

	if !success {
		t.Fatalf("test failed: %s", errorMsg)
	}

	le.Info("test completed successfully")
}
