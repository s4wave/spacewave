package statusprojector

import (
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// BuildDesktopRuntimeStateFromListener maps listener status into tray status state.
func BuildDesktopRuntimeStateFromListener(status resource_listener.ListenerStatus) *desktop_runtime.DesktopRuntimeState {
	state := &desktop_runtime.DesktopRuntimeState{
		StatusText:     "Starting",
		Health:         desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_STARTING,
		Lifecycle:      desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_STARTING,
		Listener:       buildDesktopRuntimeListenerStatus(status),
		Sessions:       []*desktop_runtime.DesktopRuntimeNavigationItem{},
		Spaces:         []*desktop_runtime.DesktopRuntimeNavigationItem{},
		Activity:       []*desktop_runtime.DesktopRuntimeActivityItem{},
		Update:         &desktop_runtime.DesktopRuntimeUpdateStatus{},
		AttentionItems: []*desktop_runtime.DesktopRuntimeAttentionItem{},
		Actions:        []*desktop_runtime.DesktopRuntimeActionItem{},
	}
	if status.Listening {
		state.StatusText = "Running"
		state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY
		state.Lifecycle = desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING
		return state
	}
	if status.SocketPath != "" {
		return state
	}
	state.StatusText = "Disconnected"
	state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED
	state.Lifecycle = desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED
	return state
}

func buildDesktopRuntimeListenerStatus(status resource_listener.ListenerStatus) *desktop_runtime.DesktopRuntimeListenerStatus {
	out := &desktop_runtime.DesktopRuntimeListenerStatus{
		Reachability:     desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_STARTING,
		Label:            "CLI starting",
		Detail:           "Waiting for listener",
		SocketPath:       status.SocketPath,
		ConnectedClients: status.ConnectedClients,
	}
	if status.Listening {
		out.Reachability = desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_REACHABLE
		out.Label = "CLI reachable"
		out.Detail = listenerReachableDetail(status)
		return out
	}
	if status.SocketPath != "" {
		return out
	}
	out.Reachability = desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_UNREACHABLE
	out.Label = "CLI unavailable"
	out.Detail = "Listener socket unavailable"
	return out
}

func listenerReachableDetail(status resource_listener.ListenerStatus) string {
	switch status.ConnectedClients {
	case 0:
		return status.SocketPath
	case 1:
		return "1 CLI client connected"
	default:
		return strconv.FormatUint(uint64(status.ConnectedClients), 10) + " CLI clients connected"
	}
}
