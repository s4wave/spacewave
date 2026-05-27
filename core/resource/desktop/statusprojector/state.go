package statusprojector

import (
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/projection"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// SessionProjection contains tray-visible session rows and attention items.
type SessionProjection = projection.SessionProjection

// BuildDesktopRuntimeStateFromListener maps listener status into tray status state.
func BuildDesktopRuntimeStateFromListener(status resource_listener.ListenerStatus) *desktop_runtime.DesktopRuntimeState {
	return projection.BuildDesktopRuntimeStateFromListener(status)
}

// BuildDesktopRuntimeState maps Spacewave runtime status into tray status state.
func BuildDesktopRuntimeState(
	status resource_listener.ListenerStatus,
	proj *SessionProjection,
) *desktop_runtime.DesktopRuntimeState {
	return projection.BuildDesktopRuntimeState(status, proj)
}
