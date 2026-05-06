package statusprojector

import (
	"context"
	"testing"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

type fakeDesktopRuntimePublisher struct {
	states []*desktop_runtime.DesktopRuntimeState
}

func (p *fakeDesktopRuntimePublisher) SetDesktopState(
	_ context.Context,
	req *desktop_runtime.SetDesktopStateRequest,
) (*desktop_runtime.SetDesktopStateResponse, error) {
	p.states = append(p.states, req.GetState().CloneVT())
	return &desktop_runtime.SetDesktopStateResponse{}, nil
}

func TestBuildDesktopRuntimeStateFromListenerReachable(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath:       "/run/spacewave.sock",
		Listening:        true,
		ConnectedClients: 2,
	})
	if state.GetStatusText() != "Running" {
		t.Fatalf("status text = %q, want Running", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY {
		t.Fatalf("health = %v, want healthy", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING {
		t.Fatalf("lifecycle = %v, want running", state.GetLifecycle())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_REACHABLE {
		t.Fatalf("listener reachability = %v, want reachable", listener.GetReachability())
	}
	if listener.GetDetail() != "2 CLI clients connected" {
		t.Fatalf("listener detail = %q, want connected client count", listener.GetDetail())
	}
}

func TestBuildDesktopRuntimeStateFromListenerStarting(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
	})
	if state.GetStatusText() != "Starting" {
		t.Fatalf("status text = %q, want Starting", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_STARTING {
		t.Fatalf("health = %v, want starting", state.GetHealth())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_STARTING {
		t.Fatalf("listener reachability = %v, want starting", listener.GetReachability())
	}
	if listener.GetSocketPath() != "/run/spacewave.sock" {
		t.Fatalf("listener socket = %q, want configured path", listener.GetSocketPath())
	}
}

func TestBuildDesktopRuntimeStateFromListenerDisconnected(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{})
	if state.GetStatusText() != "Disconnected" {
		t.Fatalf("status text = %q, want Disconnected", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED {
		t.Fatalf("health = %v, want disconnected", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED {
		t.Fatalf("lifecycle = %v, want disconnected", state.GetLifecycle())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_UNREACHABLE {
		t.Fatalf("listener reachability = %v, want unreachable", listener.GetReachability())
	}
	if len(state.GetSessions()) != 0 || len(state.GetSpaces()) != 0 || len(state.GetActivity()) != 0 {
		t.Fatalf("bounded row lists must start empty")
	}
}

func TestPublishDesktopRuntimeStateSuppressesDuplicate(t *testing.T) {
	ctx := context.Background()
	publisher := &fakeDesktopRuntimePublisher{}
	current := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	})

	prev, sent, err := publishDesktopRuntimeState(ctx, publisher, nil, current)
	if err != nil {
		t.Fatal(err)
	}
	if !sent {
		t.Fatalf("first publish did not send")
	}
	if len(publisher.states) != 1 {
		t.Fatalf("published states = %d, want 1", len(publisher.states))
	}

	prev, sent, err = publishDesktopRuntimeState(ctx, publisher, prev, current.CloneVT())
	if err != nil {
		t.Fatal(err)
	}
	if sent {
		t.Fatalf("duplicate publish sent")
	}
	if len(publisher.states) != 1 {
		t.Fatalf("published states = %d, want 1", len(publisher.states))
	}
}

// _ is a type assertion
var _ desktopRuntimePublisher = ((*fakeDesktopRuntimePublisher)(nil))
