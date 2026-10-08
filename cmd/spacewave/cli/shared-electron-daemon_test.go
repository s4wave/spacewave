//go:build !js && !wasip1

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
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

// sharedElectronIdleTimeout is the fixture daemon's final-release deadline.
const sharedElectronIdleTimeout = 500 * time.Millisecond

// sharedElectronApp locates the prepared Electron fixture.
type sharedElectronApp struct {
	// dir is the prepared app directory.
	dir string
	// electron is the Electron executable.
	electron string
}

// requireSharedElectronApp skips unless the opt-in Electron fixture is prepared.
func requireSharedElectronApp(t *testing.T) sharedElectronApp {
	// Require prepared shell artifacts when this opt-in integration check runs.
	t.Helper()
	app := sharedElectronApp{
		dir:      os.Getenv("SPACEWAVE_SHARED_DESKTOP_FIXTURE"),
		electron: os.Getenv("SPACEWAVE_SHARED_DESKTOP_ELECTRON"),
	}
	if app.dir == "" && app.electron == "" {
		t.Skip("set SPACEWAVE_SHARED_DESKTOP_FIXTURE and SPACEWAVE_SHARED_DESKTOP_ELECTRON")
	}
	for _, path := range []string{app.electron, filepath.Join(app.dir, "index.mjs"), filepath.Join(app.dir, "preload.mjs"), filepath.Join(app.dir, "renderer.mjs"), filepath.Join(app.dir, "worker.mjs")} {
		if !filepath.IsAbs(path) {
			t.Fatalf("fixture path must be absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

// sharedElectronOptions selects the daemon shape and desktop policy under test.
type sharedElectronOptions struct {
	// shape is "native-core" or "distribution-core".
	shape string
	// policy decides whether the shell outlives its last window.
	policy electron.DesktopPresencePolicy
}

// quitRecorder reports each decision the daemon returns to a desktop Quit.
type quitRecorder struct {
	*daemonDesktopControl
	// decisions receives every reply, whether it stops the daemon or not.
	decisions chan *desktop_control.QuitDesktopResponse
}

// QuitDesktop forwards the request and records the daemon's decision.
func (r *quitRecorder) QuitDesktop(ctx context.Context, req *desktop_control.QuitDesktopRequest) (*desktop_control.QuitDesktopResponse, error) {
	resp, err := r.daemonDesktopControl.QuitDesktop(ctx, req)
	if err == nil {
		r.decisions <- resp
	}
	return resp, err
}

// sharedElectronDaemon is an isolated daemon serving a production Electron
// shell controller through the production listener and idle lifecycle.
type sharedElectronDaemon struct {
	// ctx bounds every fixture operation and ends when the test cleans up.
	ctx context.Context
	// cliBus is the daemon's native state bus.
	cliBus *cli_entrypoint.CliBusImpl
	// statePath holds this daemon's state and its Electron profile.
	statePath string
	// socket is the protected daemon socket the shell dials to Quit.
	socket string
	// idle is the daemon's service-demand owner.
	idle *daemonIdleTracker
	// control is the production desktop control service.
	control *daemonDesktopControl
	// desktop reaches desktop control without joining the daemon's clients.
	desktop desktop_control.SRPCDesktopControlServiceClient
	// quits receives each Quit decision the shell obtains.
	quits chan *desktop_control.QuitDesktopResponse
	// quitCalls joins the shell Quit requests started by requestQuit.
	quitCalls sync.WaitGroup
	// exited closes when the listener has drained after Quit or idle expiry.
	exited chan struct{}
}

// newSharedElectronDaemon starts a daemon with an idle shell controller.
// The daemon, its shell, and its socket stop when the test ends.
func newSharedElectronDaemon(t *testing.T, app sharedElectronApp, opts sharedElectronOptions) *sharedElectronDaemon {
	// Bound every fixture operation. The context outlives t.Context so cleanup can use it.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	// Acquire an isolated writable state lease.
	statePath := t.TempDir()
	lease, err := acquireStatePathLease(ctx, statePath, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.release() })

	// Create the fixture logger before building the persistent daemon bus.
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if os.Getenv("SPACEWAVE_SHARED_DESKTOP_DEBUG") != "" {
		logger.SetOutput(os.Stderr)
		logger.SetLevel(logrus.DebugLevel)
	}
	le := logrus.NewEntry(logger)

	// Build the native state bus and retain its lifetime through the test.
	cliBus, err := cli_entrypoint.BuildCliBus(ctx, le, "spacewave", statePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cliBus.Release)
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
		t.Cleanup(ref.Release)
	}

	// Publish one Resource authority using the selected production serve route.
	coreRoot := resource_root.NewCoreRootServer(le, b)
	t.Cleanup(coreRoot.Close)
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
	if opts.shape == "distribution-core" {
		coreController = newNativeCorePlugin(resources)
	}
	releaseCore, err := b.AddController(ctx, coreController, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseCore)

	// Resolve the production daemon Resource forwarding path.
	var invoker srpc.Invoker
	if opts.shape == "distribution-core" {
		invoker = newDaemonResourceInvoker(b)
	} else {
		local, ref, err := lookupLocalResourceInvoker(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ref.Release)
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
	t.Cleanup(releaseRoutes)

	// Create the daemon record that the serve lifecycle below fills in.
	d := &sharedElectronDaemon{
		ctx:       ctx,
		cliBus:    cliBus,
		statePath: statePath,
		socket:    filepath.Join(statePath, socketName),
		quits:     make(chan *desktop_control.QuitDesktopResponse, 4),
		exited:    make(chan struct{}),
	}

	// Start an idle production Electron controller with a disposable profile.
	d.setEnv(t)
	shell, err := electron.NewController(le, b, app.electron, statePath, app.dir, "shared-fixture",
		[]string{"--no-sandbox", "--disable-gpu"}, &electron.ElectronInit{
			AppName:               "Shared daemon fixture",
			DesktopPresencePolicy: opts.policy,
		})
	if err != nil {
		t.Fatal(err)
	}

	// Retain the process controller and the desktop plugin for this daemon.
	releaseShell, err := b.AddController(ctx, shell, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseShell)
	plugin := web_plugin_controller.NewController(le, b, &web_plugin_controller.Config{})
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin)))

	// Bind the protected socket as serve does: idle expiry and desktop Quit
	// both close admission and cancel the daemon.
	lis, err := resource_listener.ListenProtectedUnix(d.socket, true)
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, serveCancel := context.WithCancel(ctx)
	shutdownCtx, shutdownCancel := context.WithCancel(serveCtx)
	d.idle = newDaemonIdleTracker(sharedElectronIdleTimeout, func() {
		serveCancel()
		_ = lis.Close()
	})
	t.Cleanup(d.idle.close)
	controlHandler := newDaemonControlHandler(func() {
		shutdownCancel()
		_ = lis.Close()
	})

	// Bind protected desktop control to the real web plugin capability.
	d.control = &daemonDesktopControl{ctx: serveCtx, idleTracker: d.idle}
	d.control.load = func(context.Context) (web_plugin.SRPCWebPluginClient, string, func(), error) {
		return web_plugin.NewSRPCWebPluginClient(pluginClient), "fixture/" + opts.shape, func() {}, nil
	}
	d.control.shutdown = func(requester *trackedConn) {
		controlHandler.desktopQuitConn.Store(requester)
		shutdownCancel()
		_ = lis.Close()
	}
	t.Cleanup(d.control.close)

	// Serve Resource and desktop control on the socket and to the in-process launcher.
	mux := srpc.NewMux(invoker)
	recorder := &quitRecorder{daemonDesktopControl: d.control, decisions: d.quits}
	if err := desktop_control.SRPCRegisterDesktopControlService(mux, recorder); err != nil {
		t.Fatal(err)
	}
	d.desktop = desktop_control.NewSRPCDesktopControlServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	go func() {
		defer close(d.exited)
		_ = serveDaemonListener(serveCtx, serveCancel, lis, srpc.NewServer(mux), controlHandler, shutdownCtx.Done(), d.idle)
	}()

	// Register the teardown that outlives the fixture state.
	t.Cleanup(func() {
		// Stop serving and join every Quit request before the state goes.
		serveCancel()
		_ = lis.Close()
		<-d.exited
		cancel()
		d.quitCalls.Wait()
	})
	return d
}

// setEnv points the next Electron launch at this daemon and its profile.
// The process environment is global, so call it right before opening.
func (d *sharedElectronDaemon) setEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BLDR_PLUGIN_STATE_PATH", filepath.Join(d.statePath, "electron-profile"))
	t.Setenv("SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH", d.socket)
	t.Setenv("BLDR_ELECTRON_LOG_RENDERER", "1")
}

// open starts or focuses the desktop through the given launcher connection and
// resolves the rendered WebView.
func (d *sharedElectronDaemon) open(t *testing.T, desktop desktop_control.SRPCDesktopControlServiceClient) web_view.WebView {
	// Open through the same protected control operation used by the launcher.
	t.Helper()
	d.setEnv(t)
	if _, err := desktop.OpenOrFocusDesktop(d.ctx, &desktop_control.OpenOrFocusDesktopRequest{}); err != nil {
		t.Fatalf("open desktop: %v", err)
	}
	runtime, _, ref, err := web_runtime.ExLookupWebRuntime(d.ctx, d.cliBus.GetBus(), false, electron.ControllerID)
	if err != nil {
		t.Fatalf("lookup runtime: %v", err)
	}
	defer ref.Release()

	// Wait on the runtime's document and view registration events.
	document, err := runtime.WaitFirstWebDocument(d.ctx)
	if err != nil {
		t.Fatalf("wait document: %v", err)
	}
	view, err := document.GetWebView(d.ctx, "shared-fixture-view", true)
	if err != nil {
		t.Fatalf("wait view: %v", err)
	}
	return view
}

// requireIdentity checks the identity a window rendered against the one the CLI holds.
func (d *sharedElectronDaemon) requireIdentity(t *testing.T, view web_view.WebView, want string) {
	// Read the identity rendered by the registered window.
	t.Helper()
	response := &resource_state.GetStateResponse{}
	if err := view.GetClient().ExecCall(d.ctx, "test.SharedDesktop", "Read", &resource_state.GetStateRequest{}, response); err != nil {
		t.Fatalf("read window: %v", err)
	}
	if response.GetStateJson() != want {
		t.Fatalf("window identity = %q, CLI identity = %q", response.GetStateJson(), want)
	}
}

// watchStatus subscribes to desktop status without joining the daemon's clients.
func (d *sharedElectronDaemon) watchStatus(t *testing.T) desktop_control.SRPCDesktopControlService_WatchDesktopStatusClient {
	// Open the watch on the in-process launcher connection.
	t.Helper()
	status, err := d.desktop.WatchDesktopStatus(d.ctx, &desktop_control.WatchDesktopStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = status.Close() })
	return status
}

// requestQuit asks the shell to quit as its tray and application menu do. The
// call returns only once the shell has quit, so it runs aside: a busy decision
// waits on the shell's dialog, and the reply can vanish with the exiting
// process. Read the outcome from nextQuit and the status watch.
func (d *sharedElectronDaemon) requestQuit(t *testing.T) {
	// Reach the shell's desktop runtime Resource through its private pipe.
	t.Helper()
	runtime, _, ref, err := web_runtime.ExLookupWebRuntime(d.ctx, d.cliBus.GetBus(), false, electron.ControllerID)
	if err != nil {
		t.Fatalf("lookup runtime: %v", err)
	}
	resources, err := runtime.ConnectDesktopRuntimeResourceClient(d.ctx)
	if err != nil {
		ref.Release()
		t.Fatalf("connect desktop runtime: %v", err)
	}
	rootRef := resources.AccessRootResource() //nolint:lostresource // The WaitGroup.Go callback retains and releases rootRef after Quit returns.
	root, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		ref.Release()
		t.Fatalf("access desktop runtime: %v", err)
	}

	// Send the request and release the connection when the shell answers or exits.
	service := desktop_runtime.NewSRPCDesktopRuntimeResourceServiceClient(root)
	d.quitCalls.Go(func() {
		defer ref.Release()
		defer resources.Release()
		defer rootRef.Release()
		_, _ = service.QuitDesktopRuntime(d.ctx, &desktop_runtime.QuitDesktopRuntimeRequest{})
	})
}

// nextQuit returns the daemon's next decision on a desktop Quit.
func (d *sharedElectronDaemon) nextQuit(t *testing.T) *desktop_control.QuitDesktopResponse {
	t.Helper()
	select {
	case decision := <-d.quits:
		return decision
	case <-d.ctx.Done():
		t.Fatal("daemon never decided the desktop Quit")
		return nil
	}
}

// waitIdle blocks until the daemon's holds match, watching the tracker's changes.
func (d *sharedElectronDaemon) waitIdle(t *testing.T, match func(daemonIdleSnapshot) bool) daemonIdleSnapshot {
	t.Helper()
	for {
		snapshot, changed := d.idle.observe()
		if match(snapshot) {
			return snapshot
		}
		select {
		case <-changed:
		case <-d.ctx.Done():
			t.Fatalf("daemon holds never matched: %+v", snapshot)
		}
	}
}

// waitExited blocks until the daemon has stopped serving.
func (d *sharedElectronDaemon) waitExited(t *testing.T) {
	t.Helper()
	select {
	case <-d.exited:
	case <-d.ctx.Done():
		t.Fatal("daemon never exited")
	}
}

// requireRunning fails if the daemon has stopped serving.
func (d *sharedElectronDaemon) requireRunning(t *testing.T) {
	t.Helper()
	select {
	case <-d.exited:
		t.Fatal("daemon exited while it had work")
	default:
	}
}

// requireDesktopEnded waits for the shell generation to end cleanly.
func requireDesktopEnded(t *testing.T, status desktop_control.SRPCDesktopControlService_WatchDesktopStatusClient, generation uint64) {
	// Wait for the daemon-owned observation of the shell's exit.
	t.Helper()
	ended := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetGeneration() == generation && state.GetPresence().GetState() == web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if ended.GetFailure() != "" || ended.GetPresence().GetError() != "" {
		t.Fatalf("desktop generation %d ended with a failure: %v", generation, ended)
	}
}

// sharedElectronCLI is a retained CLI connection with one seeded Space and one
// watch on its Resource list.
type sharedElectronCLI struct {
	// desktop reaches desktop control over the CLI's own protected connection.
	desktop desktop_control.SRPCDesktopControlServiceClient
	// session is the mounted Session that owns the Space.
	session *sdk_session.Session
	// watch is the Space list stream retained across desktop generations.
	watch sdk_session.SRPCSessionResourceService_WatchResourcesListClient
	// sessionID is the seeded Session's identifier.
	sessionID string
	// sessionIdx is the seeded Session's list index.
	sessionIdx uint32
	// spaceID is the seeded Space's identifier.
	spaceID string
	// closers release the connection and its Resources in reverse order.
	closers []func()
}

// attachCLI connects a CLI over the protected socket, creates one local account
// with a Space of the given name, and retains a watch on the Space list.
func (d *sharedElectronDaemon) attachCLI(t *testing.T, spaceName string) *sharedElectronCLI {
	// Connect over the real protected listener before any window opens.
	t.Helper()
	client, resourceClient, conn := desktopSocketClient(t, d.socket)
	cli := &sharedElectronCLI{desktop: desktop_control.NewSRPCDesktopControlServiceClient(client)}
	cli.closers = append(cli.closers, resourceClient.Release, func() { _ = conn.Close() })
	t.Cleanup(cli.close)

	// Access the same root Resource the CLI commands use.
	root, err := sdk_root.NewRoot(resourceClient, resourceClient.AccessRootResource())
	if err != nil {
		t.Fatal(err)
	}
	cli.closers = append(cli.closers, root.Release)

	// Create one local account through the retained CLI connection.
	providerID, err := root.LookupProvider(d.ctx, "local")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := sdk_local.NewLocalProvider(resourceClient, resourceClient.CreateResourceReference(providerID))
	if err != nil {
		t.Fatal(err)
	}
	cli.closers = append(cli.closers, provider.Release)
	account, err := provider.CreateAccount(d.ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := account.GetSessionListEntry()

	// Mount that account Session through the public root operation.
	mounted, err := root.MountSessionByIdx(d.ctx, entry.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	cli.session, err = sdk_session.NewSession(resourceClient, resourceClient.CreateResourceReference(mounted.GetResourceId()))
	if err != nil {
		t.Fatal(err)
	}
	cli.closers = append(cli.closers, cli.session.Release)

	// Create the sole Space through the CLI Session API.
	created, err := cli.session.CreateSpace(d.ctx, &sdk_session.CreateSpaceRequest{SpaceName: spaceName})
	if err != nil {
		t.Fatal(err)
	}
	cli.spaceID = created.GetSharedObjectRef().GetProviderResourceRef().GetId()
	cli.sessionID = entry.GetSessionRef().GetProviderResourceRef().GetId()
	cli.sessionIdx = entry.GetSessionIndex()

	// Retain one CLI watch before any desktop generation starts.
	cli.watch, err = cli.session.WatchResourcesList(d.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cli.closers = append(cli.closers, func() { _ = cli.watch.Close() })
	cli.requireName(t, spaceName)
	return cli
}

// identity returns the Session and Space identity a window must render.
func (c *sharedElectronCLI) identity(name string) string {
	return strings.Join([]string{strconv.FormatUint(uint64(c.sessionIdx), 10), c.sessionID, c.spaceID, name}, "\n")
}

// requireName reads only the original watch until the expected committed name appears.
func (c *sharedElectronCLI) requireName(t *testing.T, name string) {
	// Require each snapshot to retain the same sole Space identity.
	t.Helper()
	for {
		snapshot, err := c.watch.Recv()
		if err != nil {
			t.Fatalf("retained CLI watch: %v", err)
		}
		spaces := snapshot.GetSpacesList()
		if len(spaces) != 1 || spaces[0].GetEntry().GetRef().GetProviderResourceRef().GetId() != c.spaceID {
			t.Fatalf("retained CLI Space changed: %v", snapshot)
		}
		if spaces[0].GetSpaceMeta().GetName() == name {
			return
		}
	}
}

// close releases the CLI's watch, Resources, and connection once.
func (c *sharedElectronCLI) close() {
	for _, v := range slices.Backward(c.closers) {
		v()
	}
	c.closers = nil
}
