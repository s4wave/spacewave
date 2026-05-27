package projection

import (
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// BuildDesktopRuntimeStateFromListener maps listener status into tray status state.
func BuildDesktopRuntimeStateFromListener(status resource_listener.ListenerStatus) *desktop_runtime.DesktopRuntimeState {
	return BuildDesktopRuntimeState(status, nil)
}

// BuildDesktopRuntimeState maps Spacewave runtime status into tray status state.
func BuildDesktopRuntimeState(
	status resource_listener.ListenerStatus,
	projection *SessionProjection,
) *desktop_runtime.DesktopRuntimeState {
	if projection == nil {
		projection = &SessionProjection{}
	}
	state := &desktop_runtime.DesktopRuntimeState{
		StatusText:     "Starting",
		Health:         desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_STARTING,
		Lifecycle:      desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_STARTING,
		Listener:       buildDesktopRuntimeListenerStatus(status),
		Sessions:       projection.Sessions,
		Spaces:         projection.Spaces,
		Activity:       projection.Activity,
		Update:         projection.Update,
		AttentionItems: projection.AttentionItems,
		Actions:        []*desktop_runtime.DesktopRuntimeActionItem{},
	}
	if state.Update == nil {
		state.Update = &desktop_runtime.DesktopRuntimeUpdateStatus{}
	}
	if status.Listening {
		state.StatusText = "Running"
		state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY
		state.Lifecycle = desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING
	} else if status.SocketPath == "" {
		state.StatusText = "Disconnected"
		state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED
		state.Lifecycle = desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED
	}
	if hasRunningActivity(state.GetActivity()) &&
		state.GetHealth() == desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY {
		state.StatusText = "Syncing"
		state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_ACTIVE
	}
	if len(state.GetAttentionItems()) != 0 &&
		(state.GetHealth() == desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY ||
			state.GetHealth() == desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_ACTIVE) {
		state.StatusText = "Needs attention"
		state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_NEEDS_ATTENTION
	}
	return state
}

func hasRunningActivity(items []*desktop_runtime.DesktopRuntimeActivityItem) bool {
	for _, item := range items {
		if item.GetState() == desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING {
			return true
		}
	}
	return false
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
		return "Ready"
	case 1:
		return "1 CLI client connected"
	default:
		return strconv.FormatUint(uint64(status.ConnectedClients), 10) + " CLI clients connected"
	}
}
