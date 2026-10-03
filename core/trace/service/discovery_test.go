//go:build !skip_e2e && !js

package trace_service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/fsutil"
	bldr_manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_plugin_compiler_js "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	plugin_host_process "github.com/s4wave/spacewave/bldr/plugin/host/process"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	s4wave_core_e2e "github.com/s4wave/spacewave/core/e2e"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

const (
	// corePluginID identifies the native application service plugin.
	corePluginID = "spacewave-core"
	// debugPluginID identifies the native debugging service plugin.
	debugPluginID = "spacewave-debug"
)

// setupPluginClients builds isolated native plugins and resolves their RPC clients.
func setupPluginClients(
	ctx context.Context,
	t *testing.T,
	startPlugins []string,
	pluginIDs []string,
) map[string]srpc.Client {
	// Attribute native plugin setup failures to the calling test.
	t.Helper()

	// Retain compiler and subprocess diagnostics for startup failures.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Materialize sources and plugin state in an isolated test directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join(wd, "../../..")
	workDir := t.TempDir()
	distDir := filepath.Join(workDir, "src")
	pluginStateDir := filepath.Join(workDir, "plugin", "state")
	pluginDistDir := filepath.Join(workDir, "plugin", "dist")

	// Create plugin state directories and materialize the build sources.
	if err := fsutil.CleanCreateDir(pluginStateDir); err != nil {
		t.Fatal(err)
	}
	if err := fsutil.CleanCreateDir(pluginDistDir); err != nil {
		t.Fatal(err)
	}
	if err := s4wave_core_e2e.CheckoutWebDistSources(ctx, le, repoRoot, distDir); err != nil {
		t.Fatal(err)
	}

	// Compose native execution with the project's manifest builders.
	tb, err := s4wave_core_e2e.NewNativeTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register native plugin hosts on the testbed bus.
	b := tb.GetBus()
	sr := tb.GetStaticResolver()
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))

	// Register the project and manifest compilers for plugin discovery.
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_manifest_builder_controller.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_go.NewFactory(b))
	sr.AddFactory(bldr_plugin_compiler_js.NewFactory(b))
	sr.AddFactory(bldr_web_bundler_vite_compiler.NewFactory(b))

	// Register storage controllers for the native plugin testbed.
	sr.AddFactory(volume_rpc_server.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	// Mount the test volume identity and application object types.
	volPeer, err := tb.GetVolume().GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	peerCtrl := peer_controller.NewController(le, volPeer)
	relPeerCtrl, err := b.AddController(ctx, peerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relPeerCtrl)

	// Attach application object types for the plugin test lifetime.
	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	relObjectTypeCtrl, err := b.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relObjectTypeCtrl)

	// Start both hosts; scheduler policy selects each plugin's permitted runtime.
	_, _, processRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_process.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(plugin_host_process.NewConfig(pluginStateDir, pluginDistDir)),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(processRef.Release)

	// Start the QuickJS host for JavaScript plugin execution.
	_, _, quickjsRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(quickjsRef.Release)

	// Enable trace services in the project's native plugin configurations.
	projectConfig, err := s4wave_core_e2e.LoadProjectConfig(repoRoot)
	if err == nil {
		err = InjectTraceConfig(projectConfig)
	}
	if err == nil {
		err = projectConfig.Validate()
	}
	if err != nil {
		t.Fatal(err)
	}
	projectConfig.Start.Plugins = slices.Clone(startPlugins)
	projectConfig.Remotes = map[string]*bldr_project.RemoteConfig{
		"devtool": {
			EngineId:       tb.GetWorldEngineID(),
			PeerId:         tb.GetVolume().GetPeerID().String(),
			ObjectKey:      tb.GetPluginHostObjKey(),
			LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		},
	}

	// Compile and launch the requested startup plugins.
	projCtrlConf := bldr_project_controller.NewConfig(repoRoot, workDir, projectConfig, false, true)
	projCtrlConf.FetchManifestRemote = "devtool"
	_, _, projCtrlRef, err := loader.WaitExecControllerRunningTyped[*bldr_project_controller.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(projCtrlRef.Release)

	// Retain each client reference for the test's lifetime.
	clients := make(map[string]srpc.Client, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		pluginClient, pluginRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, b, pluginID, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pluginRef.Release)
		clients[pluginID] = pluginClient
	}

	return clients
}

// TestTraceServiceDiscovery discovers and streams a trace from the core plugin.
func TestTraceServiceDiscovery(t *testing.T) {
	// Require explicit opt-in before compiling native discovery plugins.
	if os.Getenv("RUN_TRACE_E2E") == "" {
		t.Skip("set RUN_TRACE_E2E=1 to run trace service discovery E2E tests")
	}

	// Start a recording through the plugin's discovered trace service.
	ctx := context.Background()
	clients := setupPluginClients(ctx, t, []string{corePluginID}, []string{corePluginID})
	pluginClient := clients[corePluginID]
	traceClient := s4wave_trace.NewSRPCTraceServiceClient(pluginClient)

	// Start trace recording through the discovered core plugin client.
	_, err := traceClient.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: "discovery"})
	if err != nil {
		t.Fatal(err)
	}

	// Stop the recording and require at least one streamed trace chunk.
	stopStrm, err := traceClient.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Count every response chunk in the discovered trace stream.
	var chunkCount int
	for {
		msg, err := stopStrm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if msg == nil {
			t.Fatal("expected stop trace response")
		}
		chunkCount++
	}

	// Require the discovered core plugin to return trace bytes.
	if chunkCount == 0 {
		t.Fatal("expected at least one trace response chunk")
	}
}

// TestTraceServiceAllPlugins records traces from core and debug with frontend plugins loaded.
func TestTraceServiceAllPlugins(t *testing.T) {
	if os.Getenv("RUN_TRACE_E2E") == "" {
		t.Skip("set RUN_TRACE_E2E=1 to run trace service discovery E2E tests")
	}

	// Load frontend plugins alongside both native trace providers.
	ctx := context.Background()
	clients := setupPluginClients(
		ctx,
		t,
		[]string{"web", "spacewave-web", "spacewave-app", corePluginID, debugPluginID},
		[]string{corePluginID, debugPluginID},
	)

	// Verify that each provider independently returns a complete trace stream.
	for _, pluginID := range []string{corePluginID, debugPluginID} {
		t.Run(pluginID, func(t *testing.T) {
			// Connect a trace client to this native plugin provider.
			traceClient := s4wave_trace.NewSRPCTraceServiceClient(clients[pluginID])

			// Start runtime recording in this native plugin provider.
			_, err := traceClient.StartTrace(ctx, &s4wave_trace.StartTraceRequest{Label: pluginID})
			if err != nil {
				t.Fatal(err)
			}

			// Stop this provider's recording and open its trace stream.
			stopStrm, err := traceClient.StopTrace(ctx, &s4wave_trace.StopTraceRequest{})
			if err != nil {
				t.Fatal(err)
			}

			// Count every response chunk returned by this native provider.
			var chunkCount int
			for {
				msg, err := stopStrm.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if msg == nil {
					t.Fatal("expected stop trace response")
				}
				chunkCount++
			}

			// Require this native provider to return trace bytes.
			if chunkCount == 0 {
				t.Fatal("expected at least one trace response chunk")
			}
		})
	}
}
