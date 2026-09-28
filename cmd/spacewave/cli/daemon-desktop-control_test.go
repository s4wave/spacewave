//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
	"github.com/s4wave/spacewave/core/appversion"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/sirupsen/logrus"
)

// desktopSocket starts the protected daemon listener with Resource and desktop RPCs.
func desktopSocket(t *testing.T, control *daemonDesktopControl, events <-chan struct{}) string {
	t.Helper()

	// Bind an isolated protected socket for this test's daemon services.
	if err := os.MkdirAll(".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".tmp", "desktop-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket, err := filepath.Abs(filepath.Join(dir, socketName))
	if err != nil {
		t.Fatal(err)
	}
	lis, err := resource_listener.ListenProtectedUnix(socket, true)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %04o, want 0600", info.Mode().Perm())
	}

	// Register the ordinary Resource service beside desktop control.
	root := srpc.NewMux()
	if err := root.Register(&socketResourceWatch{events: events}); err != nil {
		t.Fatal(err)
	}
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(mux); err != nil {
		t.Fatal(err)
	}
	if err := desktop_control.SRPCRegisterDesktopControlService(mux, control); err != nil {
		t.Fatal(err)
	}

	// Serve the protected socket until test cleanup closes its listener.
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		closeClients, _ := acceptDaemonListener(ctx, lis, srpc.NewServer(mux), control.idleTracker)
		closeClients()
	}()
	t.Cleanup(func() {
		cancel()
		_ = lis.Close()
		<-finished
	})
	return socket
}

// desktopSocketClient connects to both services over one protected socket.
func desktopSocketClient(t *testing.T, socket string) (srpc.Client, *resource_client.Client, net.Conn) {
	t.Helper()

	// Connect a launcher or retained Resource client to the protected socket.
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}

	// Initialize the ordinary Resource service on that same socket.
	resources, err := resource_client.NewClient(t.Context(), resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return client, resources, conn
}

// desktopWatch opens a Resource stream that must survive desktop transitions.
func desktopWatch(t *testing.T, resources *resource_client.Client) srpc.Stream {
	t.Helper()

	// Resolve the retained client's Root Resource and its child watch.
	rootRef := resources.AccessRootResource()
	t.Cleanup(rootRef.Release)
	root, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Keep the child stream open until test cleanup.
	watch, err := root.NewStream(t.Context(), "test.DesktopResourceWatch", "Watch", &emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watch.Close() })
	if err := watch.CloseSend(); err != nil {
		t.Fatal(err)
	}
	return watch
}

// TestDesktopControlSocketKeepsResourceAndPluginPresence verifies real socket
// and web-plugin RPCs across launcher exit, concurrent focus, and warm reopen.
func TestDesktopControlSocketKeepsResourceAndPluginPresence(t *testing.T) {
	ctx := t.Context()
	idle := newDaemonIdleTracker(0, nil)
	defer idle.close()
	gate := make(chan struct{})
	desktop := &socketDesktop{entered: make(chan string, 3), gate: gate}
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	releaseDesktop, err := b.AddHandler(desktop)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseDesktop()
	plugin := web_plugin_controller.NewController(le, b, &web_plugin_controller.Config{})
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin)))
	var pluginReleases atomic.Int64
	control := &daemonDesktopControl{ctx: ctx, idleTracker: idle}
	control.load = func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
		return bldr_web_plugin.NewSRPCWebPluginClient(pluginClient), "artifact/test", func() { pluginReleases.Add(1) }, nil
	}
	defer control.close()
	events := make(chan struct{}, 2)
	socket := desktopSocket(t, control, events)
	launcher, launcherResources, launcherConn := desktopSocketClient(t, socket)
	defer launcherResources.Release()
	defer launcherConn.Close()
	retained, retainedResources, retainedConn := desktopSocketClient(t, socket)
	defer retainedResources.Release()
	defer retainedConn.Close()
	watch := desktopWatch(t, retainedResources)
	status := desktopStatusWatch(t, retained)
	initial := recvDesktopStatus(t, status, func(*desktop_control.WatchDesktopStatusResponse) bool { return true })
	if initial.GetGeneration() != 0 || initial.GetPresence() != nil || initial.GetFailure() != "" {
		t.Fatalf("headless desktop status = %v", initial)
	}

	// Send two desktop requests through separate clients before either completes.
	results := make(chan error, 2)
	for _, request := range []struct {
		client srpc.Client
		route  string
	}{{launcher, "/spaces"}, {retained, "/settings"}} {
		go func() {
			resp, err := desktop_control.NewSRPCDesktopControlServiceClient(request.client).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{Route: request.route})
			if err == nil && (resp.GetDaemonPid() != int64(os.Getpid()) || resp.GetDaemonExecutable() == "" || resp.GetUiManifestRef() != "artifact/test" || resp.GetDaemonRelease() != appversion.GetVersion()) {
				err = errors.New("desktop response omitted daemon or UI identity")
			}
			results <- err
		}()
	}
	routes := map[string]bool{<-desktop.entered: true, <-desktop.entered: true}
	if !routes["/spaces"] || !routes["/settings"] {
		t.Fatalf("forwarded routes = %v", routes)
	}
	close(gate)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	desktop.mtx.Lock()
	opens := desktop.opens
	desktop.mtx.Unlock()
	if opens != 1 {
		t.Fatalf("concurrent requests created %d shells, want 1", opens)
	}

	// Confirm open readiness from the owner's presence through the daemon status stream.
	ready := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
	})
	if ready.GetGeneration() != 1 || ready.GetFailure() != "" {
		t.Fatalf("open status = %v", ready)
	}

	// Disconnect the launcher while the Resource stream and desktop stay active.
	if err := launcherConn.Close(); err != nil {
		t.Fatal(err)
	}
	events <- struct{}{}
	if err := watch.MsgRecv(&emptypb.Empty{}); err != nil {
		t.Fatalf("Resource watch after launcher exit: %v", err)
	}
	if pluginReleases.Load() != 1 {
		t.Fatalf("plugin releases before shell close = %d, want 1 transient reference", pluginReleases.Load())
	}

	// Observe the daemon's service release from the shell's terminal event.
	demandEnded := make(chan struct{})
	control.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		releaseDemand := control.demandRelease
		if releaseDemand == nil {
			t.Fatal("active shell has no daemon service demand")
		}
		control.demandRelease = func() {
			releaseDemand()
			close(demandEnded)
		}
	})
	desktop.closeShell("")
	<-demandEnded
	ended := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if ended.GetPresence().GetError() != "" || ended.GetFailure() != "" {
		t.Fatalf("normal end reported failure: %v", ended)
	}
	var retainedPlugin bool
	control.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		retainedPlugin = control.pluginRelease != nil && control.demandRelease == nil
	})
	if !retainedPlugin {
		t.Fatal("shell close did not release only desktop demand")
	}

	// Reopen through the retained client's socket connection.
	resp, err := desktop_control.NewSRPCDesktopControlServiceClient(retained).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetUiManifestRef() != "artifact/test" {
		t.Fatalf("reopened UI artifact = %q", resp.GetUiManifestRef())
	}
	desktop.mtx.Lock()
	opens = desktop.opens
	desktop.mtx.Unlock()
	if opens != 2 {
		t.Fatalf("reopen created %d shells total, want 2", opens)
	}
	var reopened bool
	control.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		reopened = control.pluginRelease != nil && control.demandRelease != nil
	})
	if !reopened {
		t.Fatal("reopen did not retain desktop demand")
	}
	recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetGeneration() == 2 && state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
	})
	late := recvDesktopStatus(t, desktopStatusWatch(t, retained), func(*desktop_control.WatchDesktopStatusResponse) bool { return true })
	if late.GetGeneration() != 2 || late.GetPresence().GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE {
		t.Fatalf("late subscriber missed the current shell: %v", late)
	}
	events <- struct{}{}
	if err := watch.MsgRecv(&emptypb.Empty{}); err != nil {
		t.Fatalf("Resource watch after reopen: %v", err)
	}

	// A failed shell exit is distinct from a clean close and a broken watch.
	desktop.closeShell("Electron exited with status 1")
	failed := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if failed.GetGeneration() != 2 || failed.GetPresence().GetError() != "Electron exited with status 1" || failed.GetFailure() != "" {
		t.Fatalf("shell failure status = %v", failed)
	}
	control.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if control.demandRelease != nil {
			t.Fatal("owner-confirmed failed exit retained desktop demand")
		}
	})
}

// TestDesktopControlQuitWaitsForPresenceAndOtherClients verifies that Quit
// decides busy or stop while the shell is visible and tears down after exit.
func TestDesktopControlQuitWaitsForPresenceAndOtherClients(t *testing.T) {
	// Install a plugin Electron owner and the daemon's desktop control service.
	ctx := t.Context()
	idle := newDaemonIdleTracker(0, nil)
	defer idle.close()
	gate := make(chan struct{})
	close(gate)
	desktop := &socketDesktop{entered: make(chan string, 2), gate: gate}
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	releaseDesktop, err := b.AddHandler(desktop)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseDesktop()
	plugin := web_plugin_controller.NewController(le, b, &web_plugin_controller.Config{})
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin)))
	control := &daemonDesktopControl{ctx: ctx, idleTracker: idle}
	control.load = func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
		return bldr_web_plugin.NewSRPCWebPluginClient(pluginClient), "artifact/test", func() {}, nil
	}
	stopped := make(chan struct{}, 1)
	control.shutdown = func(*trackedConn) { stopped <- struct{}{} }
	defer control.close()

	// Keep a second Resource connection through the first Quit and reopen.
	socket := desktopSocket(t, control, make(chan struct{}))
	launcher, launcherResources, launcherConn := desktopSocketClient(t, socket)
	defer launcherResources.Release()
	defer launcherConn.Close()
	other, otherResources, otherConn := desktopSocketClient(t, socket)
	defer otherResources.Release()
	defer otherConn.Close()
	otherStatus := desktopStatusWatch(t, other)
	defer otherStatus.Close()
	watch := desktopWatch(t, otherResources)
	status := desktopStatusWatch(t, launcher)
	service := desktop_control.NewSRPCDesktopControlServiceClient(launcher)

	// A busy Quit leaves the other client's stream and daemon admission alive.
	if _, err := service.OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{}); err != nil {
		t.Fatal(err)
	}
	<-desktop.entered
	resp, err := service.QuitDesktop(ctx, &desktop_control.QuitDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetOtherClients() != 1 || resp.GetOtherServices() != 0 {
		t.Fatalf("busy Quit = %v, want one other client", resp)
	}
	desktop.closeShell("")
	recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
		return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
	})
	if snapshot, _ := idle.observe(); snapshot.stopping {
		t.Fatal("busy Quit stopped the daemon")
	}
	if err := watch.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if err := otherConn.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and Quit without another client; claim admission before shell exit.
	if _, err := service.OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{}); err != nil {
		t.Fatal(err)
	}
	<-desktop.entered
	resp, err = service.QuitDesktop(ctx, &desktop_control.QuitDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetOtherClients() != 0 || resp.GetOtherServices() != 0 {
		t.Fatalf("unused Quit = %v, want no other demand", resp)
	}
	if snapshot, _ := idle.observe(); !snapshot.stopping || snapshot.services != 1 {
		t.Fatalf("pre-exit Quit claim = %+v", snapshot)
	}
	select {
	case <-stopped:
		t.Fatal("daemon stopped before shell exit")
	default:
	}
	desktop.closeShell("")
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("unused Quit did not claim shutdown after shell exit")
	}
	if snapshot, _ := idle.observe(); !snapshot.stopping || snapshot.services != 0 {
		t.Fatalf("unused Quit state = %+v", snapshot)
	}

	// A socket accepted after the claim cannot open another desktop request.
	lateConn, err := net.Dial("unix", socket)
	if err == nil {
		defer lateConn.Close()
		if err := lateConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		lateClient, err := srpc.NewClientWithConn(lateConn, true, nil)
		if err == nil {
			admissionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := desktop_control.NewSRPCDesktopControlServiceClient(lateClient).OpenOrFocusDesktop(admissionCtx, &desktop_control.OpenOrFocusDesktopRequest{}); err == nil {
				t.Fatal("desktop client admitted after Quit stop claim")
			}
		}
	}
}

// TestDesktopControlLostQuitReply verifies a claimed stop still releases desktop
// demand and requests shutdown when the requester disappears before its reply.
func TestDesktopControlLostQuitReply(t *testing.T) {
	// Attach the requesting connection and the active desktop to one idle tracker.
	idle := newDaemonIdleTracker(0, nil)
	t.Cleanup(idle.close)
	requester := &trackedConn{}
	if !idle.trackedClientAttached(requester) {
		t.Fatal("requester was not admitted")
	}
	hold := idle.attachService()
	stopped := make(chan *trackedConn, 1)
	control := &daemonDesktopControl{
		idleTracker:   idle,
		demandRelease: hold.release,
		demandHold:    hold,
		watchSequence: 1,
		status: &desktop_control.WatchDesktopStatusResponse{
			Presence: &bldr_web_plugin.WatchDesktopPresenceResponse{
				State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE,
			},
		},
		shutdown: func(conn *trackedConn) { stopped <- conn },
	}

	// Claim stop, then lose the requester without delivering its response.
	ctx := context.WithValue(t.Context(), daemonConnCtxKey{}, requester)
	response, err := control.QuitDesktop(ctx, &desktop_control.QuitDesktopRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetOtherClients() != 0 || response.GetOtherServices() != 0 {
		t.Fatalf("unused Quit = %v", response)
	}
	idle.trackedClientDetached(requester)
	if snapshot, _ := idle.observe(); !snapshot.stopping || snapshot.services != 1 || snapshot.clients != 0 {
		t.Fatalf("lost reply changed stop claim or desktop demand = %+v", snapshot)
	}

	// Owner-confirmed shell exit releases demand and completes the claimed stop.
	control.desktopEnded(1, 1, &bldr_web_plugin.WatchDesktopPresenceResponse{
		State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED,
	})
	select {
	case conn := <-stopped:
		if conn != requester {
			t.Fatal("shutdown used another requester")
		}
	default:
		t.Fatal("lost Quit reply left the daemon waiting for desktop exit")
	}
	if snapshot, _ := idle.observe(); !snapshot.stopping || snapshot.services != 0 {
		t.Fatalf("post-exit Quit state = %+v", snapshot)
	}
}

// TestDesktopControlFailuresKeepResource verifies plugin failure cannot close
// an existing Resource stream or retain an unsuccessful plugin reference.
func TestDesktopControlFailuresKeepResource(t *testing.T) {
	for _, failure := range []struct {
		name      string
		loadErr   error
		endOnOpen bool
		want      string
	}{
		{name: "artifact", loadErr: errors.New("desktop UI artifact unavailable"), want: "unavailable"},
		{name: "capability", want: "Electron capability unavailable"},
		{name: "ended", endOnOpen: true, want: "ended before acknowledgement"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			ctx := t.Context()
			idle := newDaemonIdleTracker(0, nil)
			defer idle.close()
			var pluginClient srpc.Client
			if failure.loadErr == nil {
				le := logrus.NewEntry(logrus.New())
				b, _, err := core.NewCoreBus(ctx, le)
				if err != nil {
					t.Fatal(err)
				}
				defer b.Close()
				if failure.endOnOpen {
					gate := make(chan struct{})
					close(gate)
					desktop := &socketDesktop{entered: make(chan string, 1), gate: gate, endOnOpen: true}
					releaseDesktop, err := b.AddHandler(desktop)
					if err != nil {
						t.Fatal(err)
					}
					defer releaseDesktop()
				}
				plugin := web_plugin_controller.NewController(le, b, &web_plugin_controller.Config{})
				pluginClient = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin)))
			}
			var releases atomic.Int64
			control := &daemonDesktopControl{ctx: ctx, idleTracker: idle}
			control.load = func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
				if failure.loadErr != nil {
					return nil, "", nil, failure.loadErr
				}
				return bldr_web_plugin.NewSRPCWebPluginClient(pluginClient), "artifact/test", func() { releases.Add(1) }, nil
			}
			defer control.close()
			events := make(chan struct{}, 1)
			socket := desktopSocket(t, control, events)
			client, resources, conn := desktopSocketClient(t, socket)
			defer resources.Release()
			defer conn.Close()
			watch := desktopWatch(t, resources)
			status := desktopStatusWatch(t, client)

			// Reject desktop failure while the Resource stream remains usable.
			_, err := desktop_control.NewSRPCDesktopControlServiceClient(client).OpenOrFocusDesktop(ctx, &desktop_control.OpenOrFocusDesktopRequest{})
			if err == nil || !strings.Contains(err.Error(), failure.want) {
				t.Fatalf("desktop error = %v, want %q", err, failure.want)
			}
			events <- struct{}{}
			if err := watch.MsgRecv(&emptypb.Empty{}); err != nil {
				t.Fatalf("Resource watch after failure: %v", err)
			}
			if failure.loadErr == nil && !failure.endOnOpen && releases.Load() != 1 {
				t.Fatalf("failed plugin references released = %d, want 1", releases.Load())
			}
			if !failure.endOnOpen {
				failed := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
					return state.GetFailure() != ""
				})
				if !strings.Contains(failed.GetFailure(), failure.want) {
					t.Fatalf("status omitted actionable failure: %v", failed)
				}
			}
		})
	}
}

// TestDesktopControlMissingWebPlugin reports an absent UI artifact from the
// real LoadPlugin directive without constructing an Electron runtime.
func TestDesktopControlMissingWebPlugin(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	idle := newDaemonIdleTracker(0, nil)
	defer idle.close()
	control := newDaemonDesktopControl(ctx, b, idle)
	defer control.close()
	_, _, _, err = control.load(ctx)
	if err == nil || !strings.Contains(err.Error(), "desktop UI artifact unavailable") {
		t.Fatalf("missing web plugin error = %v", err)
	}
}

// TestDesktopControlStreamFailureKeepsDemand verifies failed presence observation
// cannot stop a daemon whose Electron shell has not reported ENDED.
func TestDesktopControlStreamFailureKeepsDemand(t *testing.T) {
	for _, failure := range []string{"open-stream", "first-receive", "active-stream"} {
		t.Run(failure, func(t *testing.T) {
			// Serve status and Resource RPCs while the private presence stream fails.
			idle := newDaemonIdleTracker(0, nil)
			defer idle.close()
			control := &daemonDesktopControl{ctx: t.Context(), idleTracker: idle}
			defer control.close()
			fail := make(chan struct{})
			client := &failingPresenceClient{stream: &failingPresenceStream{fail: fail, failFirst: failure == "first-receive"}}
			if failure == "open-stream" {
				client.openErr = errors.New("plugin stream unavailable")
			}
			if failure != "active-stream" {
				close(fail)
			}
			var pluginReleases atomic.Int64
			control.load = func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
				return client, "artifact/test", func() { pluginReleases.Add(1) }, nil
			}
			events := make(chan struct{}, 1)
			socket := desktopSocket(t, control, events)
			launcher, resources, conn := desktopSocketClient(t, socket)
			defer resources.Release()
			defer conn.Close()
			status := desktopStatusWatch(t, launcher)
			watch := desktopWatch(t, resources)

			// Fail before or after confirmation; neither outcome proves a shell exit.
			_, err := desktop_control.NewSRPCDesktopControlServiceClient(launcher).OpenOrFocusDesktop(t.Context(), &desktop_control.OpenOrFocusDesktopRequest{})
			if failure == "active-stream" {
				if err != nil {
					t.Fatal(err)
				}
				recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
					return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
				})
				close(fail)
			}
			if failure != "active-stream" && err == nil {
				t.Fatal("unconfirmed presence acknowledged readiness")
			}
			failed := recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
				return state.GetFailure() != ""
			})
			if !strings.Contains(failed.GetFailure(), "reopen") {
				t.Fatalf("presence failure is not actionable: %q", failed.GetFailure())
			}
			if failed.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
				t.Fatal("stream failure falsely reported shell completion")
			}

			// A fresh status client receives the failure after the launcher disconnects.
			// SRPC still closes the transport when cancellation follows CloseSend.
			_ = status.Close()
			events <- struct{}{}
			if err := watch.MsgRecv(&emptypb.Empty{}); err != nil {
				t.Fatalf("Resource watch after presence failure: %v", err)
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			lateClient, lateResources, lateConn := desktopSocketClient(t, socket)
			defer lateResources.Release()
			defer lateConn.Close()
			late := recvDesktopStatus(t, desktopStatusWatch(t, lateClient), func(state *desktop_control.WatchDesktopStatusResponse) bool { return true })
			if !failed.EqualVT(late) {
				t.Fatalf("late status = %v, want %v", late, failed)
			}
			control.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				if control.demandRelease == nil || control.pluginRelease == nil {
					t.Fatal("unconfirmed stream failure released desktop demand or plugin")
				}
			})
			if pluginReleases.Load() != 0 {
				t.Fatal("presence failure released the retained plugin")
			}
		})
	}
}

// TestDesktopControlOwnerExitAfterStreamFailure verifies a failed status stream
// preserves demand until the unary owner wait returns, even after a late call.
func TestDesktopControlOwnerExitAfterStreamFailure(t *testing.T) {
	for _, quit := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary-exit", true: "quit"}[quit], func(t *testing.T) {
			idle := newDaemonIdleTracker(0, nil)
			defer idle.close()
			control := &daemonDesktopControl{ctx: t.Context(), idleTracker: idle}
			defer control.close()
			failed := make(chan struct{})
			exited := make(chan struct{})
			waitStarted := make(chan struct{})
			waitGate := make(chan struct{})
			client := &failingPresenceClient{
				stream:      &failingPresenceStream{fail: failed},
				exit:        exited,
				waitStarted: waitStarted,
				waitGate:    waitGate,
			}
			control.load = func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
				return client, "artifact/test", func() {}, nil
			}
			stopped := make(chan struct{}, 1)
			control.shutdown = func(*trackedConn) { stopped <- struct{}{} }
			socket := desktopSocket(t, control, make(chan struct{}))
			launcher, resources, conn := desktopSocketClient(t, socket)
			defer resources.Release()
			defer conn.Close()
			status := desktopStatusWatch(t, launcher)
			service := desktop_control.NewSRPCDesktopControlServiceClient(launcher)

			// Confirm the shell, then fail only the status observation stream.
			if _, err := service.OpenOrFocusDesktop(t.Context(), &desktop_control.OpenOrFocusDesktopRequest{}); err != nil {
				t.Fatal(err)
			}
			close(failed)
			recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
				return state.GetFailure() != ""
			})
			if snapshot, _ := idle.observe(); snapshot.services != 1 || snapshot.stopping {
				t.Fatalf("stream failure changed demand = %+v", snapshot)
			}

			// Quit claims immediately, but neither path releases demand before exit.
			if quit {
				result, err := service.QuitDesktop(t.Context(), &desktop_control.QuitDesktopRequest{})
				if err != nil {
					t.Fatal(err)
				}
				if result.GetOtherClients() != 0 || result.GetOtherServices() != 0 {
					t.Fatalf("unused Quit = %v", result)
				}
			}
			if snapshot, _ := idle.observe(); snapshot.services != 1 || snapshot.stopping != quit {
				t.Fatalf("pre-exit state = %+v", snapshot)
			}

			// End the shell before allowing the owner wait to read its terminal state.
			<-waitStarted
			close(exited)
			if snapshot, _ := idle.observe(); snapshot.services != 1 {
				t.Fatalf("late owner wait released demand early = %+v", snapshot)
			}
			close(waitGate)
			recvDesktopStatus(t, status, func(state *desktop_control.WatchDesktopStatusResponse) bool {
				return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED
			})
			for {
				snapshot, changed := idle.observe()
				if snapshot.services == 0 {
					break
				}
				<-changed
			}
			select {
			case <-stopped:
				if !quit {
					t.Fatal("ordinary exit bypassed idle rule")
				}
			default:
				if quit {
					<-stopped
				}
			}
		})
	}
}

// desktopStatusWatch opens the generated status RPC on an existing protected connection.
func desktopStatusWatch(t *testing.T, client srpc.Client) desktop_control.SRPCDesktopControlService_WatchDesktopStatusClient {
	t.Helper()
	stream, err := desktop_control.NewSRPCDesktopControlServiceClient(client).WatchDesktopStatus(t.Context(), &desktop_control.WatchDesktopStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	return stream
}

// recvDesktopStatus receives owner-driven updates until the expected observation arrives.
func recvDesktopStatus(t *testing.T, stream desktop_control.SRPCDesktopControlService_WatchDesktopStatusClient, match func(*desktop_control.WatchDesktopStatusResponse) bool) *desktop_control.WatchDesktopStatusResponse {
	t.Helper()
	for {
		state, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if match(state) {
			return state
		}
	}
}
