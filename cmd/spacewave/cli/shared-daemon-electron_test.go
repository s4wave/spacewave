//go:build !js && !wasip1

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
	"github.com/s4wave/spacewave/bldr/web/plugin/electron"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	sdk_local "github.com/s4wave/spacewave/sdk/provider/local"
	sdk_root "github.com/s4wave/spacewave/sdk/root"
	sdk_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// TestSharedDaemonElectron checks both core routes through an actual Electron
// window, preload bridge, dedicated Worker, and retained Unix-socket client.
// The distribution fixture loads the core through production plugin RPC while
// keeping the core implementation in-process; it does not select a packaged
// distribution or plugin artifact.
//
// The test is opt-in. Build the app with
// "go run ./e2e/shareddaemon/prepare <dir>" after "go mod vendor", set
// SPACEWAVE_SHARED_DESKTOP_FIXTURE to that directory and
// SPACEWAVE_SHARED_DESKTOP_ELECTRON to an Electron executable, then run it on a
// display with e2e/shareddaemon/run.sh. Set SPACEWAVE_SHARED_DESKTOP_DEBUG to
// log the daemon and Electron.
func TestSharedDaemonElectron(t *testing.T) {
	// Require prepared shell artifacts when this opt-in integration check runs.
	appDir := os.Getenv("SPACEWAVE_SHARED_DESKTOP_FIXTURE")
	electronPath := os.Getenv("SPACEWAVE_SHARED_DESKTOP_ELECTRON")
	if appDir == "" && electronPath == "" {
		t.Skip("set SPACEWAVE_SHARED_DESKTOP_FIXTURE and SPACEWAVE_SHARED_DESKTOP_ELECTRON")
	}
	for _, path := range []string{electronPath, filepath.Join(appDir, "index.mjs"), filepath.Join(appDir, "preload.mjs"), filepath.Join(appDir, "renderer.mjs"), filepath.Join(appDir, "worker.mjs")} {
		if !filepath.IsAbs(path) {
			t.Fatalf("fixture path must be absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}

	// Run sequentially because each shell uses an isolated process environment.
	for _, shape := range []string{"native-core", "distribution-core"} {
		t.Run(shape, func(t *testing.T) {
			runSharedDaemonElectron(t, appDir, electronPath, shape)
		})
	}
}

// runSharedDaemonElectron retains one CLI Session watch through two shell generations.
func runSharedDaemonElectron(t *testing.T, appDir, electronPath, shape string) {
	// Bound every fixture operation and acquire an isolated writable state lease.
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	statePath := t.TempDir()
	lease, err := acquireStatePathLease(ctx, statePath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()

	// Create the fixture logger before building the persistent daemon bus.
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if os.Getenv("SPACEWAVE_SHARED_DESKTOP_DEBUG") != "" {
		logger.SetOutput(os.Stderr)
		logger.SetLevel(logrus.DebugLevel)
	}
	le := logrus.NewEntry(logger)

	// Build the native state bus and retain its lifetime through both shells.
	cliBus, err := cli_entrypoint.BuildCliBus(ctx, le, "spacewave", statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cliBus.Release()
	b := cliBus.GetBus()

	// Load the production Session catalog and local provider against this bus.
	cliBus.GetStaticResolver().AddFactory(session_controller.NewFactory(b))
	cliBus.GetStaticResolver().AddFactory(provider_local.NewFactory(b))
	cliBus.GetStaticResolver().AddFactory(space_sobject.NewFactory(b))
	for _, conf := range []config.Config{
		&session_controller.Config{VolumeId: cliBus.GetVolume().GetID()},
		&space_sobject.Config{},
		&provider_local.Config{ProviderId: "local", StorageId: default_storage.StorageID},
	} {
		_, ref, err := b.AddDirective(resolver.NewLoadControllerWithConfig(conf), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ref.Release()
	}

	// Publish one Resource authority using the selected production serve route.
	coreRoot := resource_root.NewCoreRootServer(le, b)
	defer coreRoot.Close()
	rootMux := srpc.NewMux()
	if err := coreRoot.Register(rootMux); err != nil {
		t.Fatal(err)
	}

	// Expose the core Resource tree over independent client connections.
	resources := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(resources); err != nil {
		t.Fatal(err)
	}

	// Register the authority behind the native or plugin serve interface.
	coreController := controller.Controller(bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/shared-core", controller.MustParseVersion("0.0.1"), ""),
		bifrost_rpc.NewRpcServiceBuilder(resources), nil, false, nil,
		[]string{resource.SRPCResourceServiceServiceID}, regexp.MustCompile("^$")))
	if shape == "distribution-core" {
		coreController = newNativeCorePlugin(resources)
	}
	releaseCore, err := b.AddController(ctx, coreController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCore()

	// Resolve the production daemon Resource forwarding path.
	var invoker srpc.Invoker
	if shape == "distribution-core" {
		invoker = newDaemonResourceInvoker(b)
	} else {
		local, ref, err := lookupLocalResourceInvoker(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		defer ref.Release()
		invoker = local
	}

	// Both frontend and worker Resource requests pass through that same authority.
	routes := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/shared-web-routes", controller.MustParseVersion("0.0.1"), ""),
		bifrost_rpc.NewRpcServiceBuilder(invoker), nil, false, nil,
		[]string{resource.SRPCResourceServiceServiceID},
		regexp.MustCompile("^(web-view/shared-fixture-view|web-worker/shared-fixture-worker)$"))
	releaseRoutes, err := b.AddController(ctx, routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseRoutes()

	// Start an idle production Electron controller with a disposable profile.
	t.Setenv("BLDR_PLUGIN_STATE_PATH", filepath.Join(statePath, "electron-profile"))
	t.Setenv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", "")
	t.Setenv("BLDR_ELECTRON_LOG_RENDERER", "1")
	shell, err := electron.NewController(le, b, electronPath, statePath, appDir, "shared-fixture",
		[]string{"--no-sandbox", "--disable-gpu"}, &electron.ElectronInit{
			AppName:               "Shared daemon fixture",
			DesktopPresencePolicy: electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME,
		})
	if err != nil {
		t.Fatal(err)
	}

	// Retain the process controller and the desktop plugin for this daemon.
	releaseShell, err := b.AddController(ctx, shell, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseShell()
	plugin := web_plugin_controller.NewController(le, b, &web_plugin_controller.Config{})
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin)))

	// Bind protected desktop control to the real web plugin capability.
	idle := newDaemonIdleTracker(0, nil)
	defer idle.close()
	control := &daemonDesktopControl{ctx: ctx, idleTracker: idle}
	control.load = func(context.Context) (web_plugin.SRPCWebPluginClient, string, func(), error) {
		return web_plugin.NewSRPCWebPluginClient(pluginClient), "fixture/" + shape, func() {}, nil
	}
	defer control.close()

	// Connect the retained CLI over the real protected listener before opening a window.
	socket := sharedElectronSocket(t, ctx, statePath, control, invoker)
	client, resourceClient, conn := desktopSocketClient(t, socket)
	defer resourceClient.Release()
	defer conn.Close()

	// Access the same root Resource the CLI commands use.
	root, err := sdk_root.NewRoot(resourceClient, resourceClient.AccessRootResource())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Release()

	// Create one local account through the retained CLI connection.
	providerID, err := root.LookupProvider(ctx, "local")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := sdk_local.NewLocalProvider(resourceClient, resourceClient.CreateResourceReference(providerID))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Release()
	account, err := provider.CreateAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := account.GetSessionListEntry()

	// Mount that account Session through the public root operation.
	mounted, err := root.MountSessionByIdx(ctx, entry.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	session, err := sdk_session.NewSession(resourceClient, resourceClient.CreateResourceReference(mounted.GetResourceId()))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Release()

	// Create the sole Space through the CLI Session API.
	created, err := session.CreateSpace(ctx, &sdk_session.CreateSpaceRequest{SpaceName: "CLI seeded Space"})
	if err != nil {
		t.Fatal(err)
	}
	spaceID := created.GetSharedObjectRef().GetProviderResourceRef().GetId()
	sessionID := entry.GetSessionRef().GetProviderResourceRef().GetId()

	// Retain one CLI watch before either desktop generation starts.
	watch, err := session.WatchResourcesList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	sharedElectronWatchName(t, watch, spaceID, "CLI seeded Space")

	// Observe desktop lifecycle separately from the retained Resource stream.
	desktop := desktop_control.NewSRPCDesktopControlServiceClient(client)
	status, err := desktop.WatchDesktopStatus(ctx, &desktop_control.WatchDesktopStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer status.Close()

	// Open, inspect the displayed Session and Space, then rename from the actual Worker.
	view := sharedElectronOpen(t, ctx, cliBus, desktop, sessionID, entry.GetSessionIndex(), spaceID, "CLI seeded Space")
	if err := view.GetClient().ExecCall(ctx, "test.SharedDesktop", "Rename",
		&resource_state.SetStateRequest{StateJson: "worker first open"}, &resource_state.SetStateResponse{}); err != nil {
		t.Fatal(err)
	}
	sharedElectronWatchName(t, watch, spaceID, "worker first open")

	// Close the BrowserWindow; its IPC reply can disappear with the renderer.
	// The daemon status watch below confirms the joined shell result.
	_ = view.Remove(ctx)
	ended := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetGeneration() == 1 && state.GetPresence().GetState() == web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if ended.GetFailure() != "" || ended.GetPresence().GetError() != "" {
		t.Fatalf("desktop close failed: %v", ended)
	}
	if _, err := session.RenameSpace(ctx, spaceID, "CLI while closed"); err != nil {
		t.Fatal(err)
	}
	sharedElectronWatchName(t, watch, spaceID, "CLI while closed")

	// Reopen on the same authority and prove the original CLI stream still receives worker writes.
	view = sharedElectronOpen(t, ctx, cliBus, desktop, sessionID, entry.GetSessionIndex(), spaceID, "CLI while closed")
	if err := view.GetClient().ExecCall(ctx, "test.SharedDesktop", "Rename",
		&resource_state.SetStateRequest{StateJson: "worker after reopen"}, &resource_state.SetStateResponse{}); err != nil {
		t.Fatal(err)
	}
	sharedElectronWatchName(t, watch, spaceID, "worker after reopen")

	// Observe the second joined process exit even if the renderer loses its reply.
	_ = view.Remove(ctx)
	ended = recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetGeneration() == 2 && state.GetPresence().GetState() == web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if ended.GetFailure() != "" || ended.GetPresence().GetError() != "" {
		t.Fatalf("desktop reopen close failed: %v", ended)
	}
}

// sharedElectronOpen resolves a real WebView and checks its rendered identity through RPC.
func sharedElectronOpen(t *testing.T, ctx context.Context, cliBus *cli_entrypoint.CliBusImpl,
	desktop desktop_control.SRPCDesktopControlServiceClient, sessionID string, index uint32, spaceID, name string,
) web_view.WebView {
	// Open through the same protected control operation used by the launcher.
	t.Helper()
	if _, err := desktop.OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{}); err != nil {
		t.Fatalf("open desktop: %v", err)
	}
	runtime, _, ref, err := web_runtime.ExLookupWebRuntime(ctx, cliBus.GetBus(), false, electron.ControllerID)
	if err != nil {
		t.Fatalf("lookup runtime: %v", err)
	}
	defer ref.Release()

	// Wait on the runtime's document and view registration events.
	document, err := runtime.WaitFirstWebDocument(ctx)
	if err != nil {
		t.Fatalf("wait document: %v", err)
	}
	view, err := document.GetWebView(ctx, "shared-fixture-view", true)
	if err != nil {
		t.Fatalf("wait view: %v", err)
	}

	// Read the identity rendered by the registered window.
	response := &resource_state.GetStateResponse{}
	if err := view.GetClient().ExecCall(ctx, "test.SharedDesktop", "Read", &resource_state.GetStateRequest{}, response); err != nil {
		t.Fatalf("read window: %v", err)
	}
	want := strings.Join([]string{strconv.FormatUint(uint64(index), 10), sessionID, spaceID, name}, "\n")
	if response.GetStateJson() != want {
		t.Fatalf("window identity = %q, CLI identity = %q", response.GetStateJson(), want)
	}
	return view
}

// sharedElectronWatchName reads only the original watch until the expected committed name appears.
func sharedElectronWatchName(t *testing.T, watch sdk_session.SRPCSessionResourceService_WatchResourcesListClient, spaceID, name string) {
	// Require each snapshot to retain the same sole Space identity.
	t.Helper()
	for {
		snapshot, err := watch.Recv()
		if err != nil {
			t.Fatalf("retained CLI watch: %v", err)
		}
		spaces := snapshot.GetSpacesList()
		if len(spaces) != 1 || spaces[0].GetEntry().GetRef().GetProviderResourceRef().GetId() != spaceID {
			t.Fatalf("retained CLI Space changed: %v", snapshot)
		}
		if spaces[0].GetSpaceMeta().GetName() == name {
			return
		}
	}
}

// sharedElectronSocket serves production desktop control beside the selected Resource invoker.
func sharedElectronSocket(t *testing.T, ctx context.Context, statePath string, control *daemonDesktopControl, resources srpc.Invoker) string {
	// Bind and register the daemon's protected socket.
	t.Helper()
	socket := filepath.Join(statePath, socketName)
	listener, err := resource_listener.ListenProtectedUnix(socket, true)
	if err != nil {
		t.Fatal(err)
	}
	mux := srpc.NewMux(resources)
	if err := desktop_control.SRPCRegisterDesktopControlService(mux, control); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}

	// Stop accepting and join all socket clients before other fixture cleanup.
	done := make(chan struct{})
	serveCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		closeClients, _ := acceptDaemonListener(serveCtx, listener, srpc.NewServer(mux), control.idleTracker)
		closeClients()
	}()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-done
	})
	return socket
}
