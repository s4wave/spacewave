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

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
	"github.com/s4wave/spacewave/core/appversion"
	desktopcontrol "github.com/s4wave/spacewave/core/daemon/desktopcontrol"
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
	if err := desktopcontrol.SRPCRegisterDesktopControlService(mux, control); err != nil {
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
	initial := recvDesktopStatus(t, status, func(*desktopcontrol.WatchDesktopStatusResponse) bool { return true })
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
			resp, err := desktopcontrol.NewSRPCDesktopControlServiceClient(request.client).OpenOrFocusDesktop(ctx, &desktopcontrol.OpenOrFocusDesktopRequest{Route: request.route})
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
	ready := recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
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
	ended := recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
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
	resp, err := desktopcontrol.NewSRPCDesktopControlServiceClient(retained).OpenOrFocusDesktop(ctx, &desktopcontrol.OpenOrFocusDesktopRequest{})
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
	recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
		return state.GetGeneration() == 2 && state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
	})
	late := recvDesktopStatus(t, desktopStatusWatch(t, retained), func(*desktopcontrol.WatchDesktopStatusResponse) bool { return true })
	if late.GetGeneration() != 2 || late.GetPresence().GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE {
		t.Fatalf("late subscriber missed the current shell: %v", late)
	}
	events <- struct{}{}
	if err := watch.MsgRecv(&emptypb.Empty{}); err != nil {
		t.Fatalf("Resource watch after reopen: %v", err)
	}

	// A failed shell exit is distinct from a clean close and a broken watch.
	desktop.closeShell("Electron exited with status 1")
	failed := recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
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
			_, err := desktopcontrol.NewSRPCDesktopControlServiceClient(client).OpenOrFocusDesktop(ctx, &desktopcontrol.OpenOrFocusDesktopRequest{})
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
				failed := recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
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
			_, err := desktopcontrol.NewSRPCDesktopControlServiceClient(launcher).OpenOrFocusDesktop(t.Context(), &desktopcontrol.OpenOrFocusDesktopRequest{})
			if failure == "active-stream" {
				if err != nil {
					t.Fatal(err)
				}
				recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
					return state.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
				})
				close(fail)
			}
			if failure != "active-stream" && err == nil {
				t.Fatal("unconfirmed presence acknowledged readiness")
			}
			failed := recvDesktopStatus(t, status, func(state *desktopcontrol.WatchDesktopStatusResponse) bool {
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
			late := recvDesktopStatus(t, desktopStatusWatch(t, lateClient), func(state *desktopcontrol.WatchDesktopStatusResponse) bool { return true })
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

// desktopStatusWatch opens the generated status RPC on an existing protected connection.
func desktopStatusWatch(t *testing.T, client srpc.Client) desktopcontrol.SRPCDesktopControlService_WatchDesktopStatusClient {
	t.Helper()
	stream, err := desktopcontrol.NewSRPCDesktopControlServiceClient(client).WatchDesktopStatus(t.Context(), &desktopcontrol.WatchDesktopStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	return stream
}

// recvDesktopStatus receives owner-driven updates until the expected observation arrives.
func recvDesktopStatus(t *testing.T, stream desktopcontrol.SRPCDesktopControlService_WatchDesktopStatusClient, match func(*desktopcontrol.WatchDesktopStatusResponse) bool) *desktopcontrol.WatchDesktopStatusResponse {
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
