package bldr_web_plugin_controller

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	"github.com/sirupsen/logrus"
)

// desktopHandler resolves a plugin-local desktop request and records its route.
type desktopHandler struct {
	// requests records forwarded desktop requests.
	requests chan *bldr_web_plugin.OpenOrFocusDesktopRequest
	// presence supplies the shell state and terminal result.
	presence *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse]
}

// OpenOrFocusMainWindow forwards the test request.
func (h *desktopHandler) OpenOrFocusMainWindow(ctx context.Context, req *bldr_web_plugin.OpenOrFocusDesktopRequest) (uint64, error) {
	h.requests <- req
	return 1, nil
}

// DesktopPresence returns the test shell's completion event while it is active.
func (h *desktopHandler) DesktopPresence(generation uint64) *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse] {
	if generation != 1 {
		return nil
	}
	return h.presence
}

// HandleDirective exposes the test desktop through the plugin lookup.
func (h *desktopHandler) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(bldr_web_plugin.LookupDesktop); !ok {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_web_plugin.Desktop{h}), nil)
}

func TestOpenOrFocusDesktopForwardsThroughPluginRPC(t *testing.T) {
	// Start the controller bus for plugin-local desktop RPC requests.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	// Register the plugin's desktop controller through its lookup directive.
	handler := &desktopHandler{requests: make(chan *bldr_web_plugin.OpenOrFocusDesktopRequest, 1), presence: ccontainer.NewCContainerVT(&bldr_web_plugin.WatchDesktopPresenceResponse{State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE})}
	release, err := b.AddHandler(handler)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Send the request through the web-plugin RPC and check its acknowledgement.
	ctrl := NewController(le, b, &Config{})
	client := bldr_web_plugin.NewSRPCWebPluginClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(ctrl.mux))))
	opened, err := client.OpenOrFocusDesktop(ctx, &bldr_web_plugin.OpenOrFocusDesktopRequest{
		Route:        "/settings",
		InstalledApp: "/Applications/Spacewave.app",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the desktop acknowledgement and the forwarded launch request.
	if opened.GetGeneration() != 1 {
		t.Fatalf("generation = %d, want 1", opened.GetGeneration())
	}
	if req := <-handler.requests; req.GetRoute() != "/settings" || req.GetInstalledApp() != "/Applications/Spacewave.app" {
		t.Fatalf("request = %v, want the route and installed app", req)
	}

	// The plugin stream confirms this shell and reports its completion.
	stream, err := client.WatchDesktopPresence(ctx, &bldr_web_plugin.WatchDesktopPresenceRequest{Generation: opened.GetGeneration()})
	if err != nil {
		t.Fatal(err)
	}
	state, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the plugin stream reports the active desktop generation.
	if state.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE {
		t.Fatalf("initial presence = %v, want active", state.GetState())
	}

	// End the desktop generation and receive its terminal presence.
	handler.presence.SetValue(&bldr_web_plugin.WatchDesktopPresenceResponse{State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED, Error: "Electron exited with status 1"})
	state, err = stream.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the terminal desktop state, exit error, and stream completion.
	if state.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
		t.Fatalf("final presence = %v, want ended", state.GetState())
	}
	if state.GetError() != "Electron exited with status 1" {
		t.Fatalf("shell error = %q", state.GetError())
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("stream completion = %v, want EOF", err)
	}

	// A late subscription cannot confirm a shell that has already ended.
	late, err := client.WatchDesktopPresence(ctx, &bldr_web_plugin.WatchDesktopPresenceRequest{Generation: opened.GetGeneration()})
	if err != nil {
		t.Fatal(err)
	}
	state, err = late.Recv()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the terminal desktop state, exit error, and stream completion.
	if state.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
		t.Fatalf("late presence = %v, want ended", state.GetState())
	}
	if state.GetError() != "Electron exited with status 1" {
		t.Fatalf("late shell error = %q", state.GetError())
	}
	if _, err := late.Recv(); err != io.EOF {
		t.Fatalf("late stream completion = %v, want EOF", err)
	}

	// A unary wait made after exit returns the owner's retained terminal result.
	terminal, err := client.WaitDesktopExit(ctx, &bldr_web_plugin.WatchDesktopPresenceRequest{Generation: opened.GetGeneration()})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the unary desktop wait retains the terminal state and exit error.
	if terminal.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED || terminal.GetError() != "Electron exited with status 1" {
		t.Fatalf("late owner wait = %v, want ended with shell error", terminal)
	}
}
