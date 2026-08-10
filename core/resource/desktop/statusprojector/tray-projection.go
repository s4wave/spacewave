package statusprojector

import (
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/projection"
	projection_tray "github.com/s4wave/spacewave/core/resource/desktop/statusprojector/projection/traymodel"
)

// BuildDesktopTrayEntriesFromRuntimeState maps runtime status into tray entries.
func BuildDesktopTrayEntriesFromRuntimeState(
	state *desktop_runtime.DesktopRuntimeState,
) []*desktop_tray.DesktopTrayEntry {
	return desktopTrayEntriesFromProjection(projection.BuildDesktopTrayEntriesFromRuntimeState(state))
}

func desktopTrayEntriesFromProjection(entries []*projection_tray.DesktopTrayEntry) []*desktop_tray.DesktopTrayEntry {
	out := make([]*desktop_tray.DesktopTrayEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, &desktop_tray.DesktopTrayEntry{
			Id:        entry.GetId(),
			Kind:      desktopTrayEntryKindFromProjection(entry.GetKind()),
			Label:     entry.GetLabel(),
			Active:    entry.GetActive(),
			Enabled:   entry.GetEnabled(),
			Action:    desktopTrayActionFromProjection(entry.GetAction()),
			Order:     entry.GetOrder(),
			IconState: desktopTrayIconStateFromProjection(entry.GetIconState()),
			Severity:  desktopTraySeverityFromProjection(entry.GetSeverity()),
		})
	}
	return out
}

func desktopTrayActionFromProjection(
	action *projection_tray.DesktopTrayAction,
) *desktop_tray.DesktopTrayAction {
	if action == nil {
		return nil
	}
	return &desktop_tray.DesktopTrayAction{
		Kind:  desktopTrayActionKindFromProjection(action.GetKind()),
		Route: action.GetRoute(),
		Value: action.GetValue(),
	}
}

func desktopTrayEntryKindFromProjection(
	kind projection_tray.DesktopTrayEntryKind,
) desktop_tray.DesktopTrayEntryKind {
	switch kind {
	case projection_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SECTION:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SECTION
	case projection_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SEPARATOR:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SEPARATOR
	case projection_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS
	case projection_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION
	case projection_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SUBMENU:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SUBMENU
	default:
		return desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_UNKNOWN
	}
}

func desktopTrayActionKindFromProjection(
	kind projection_tray.DesktopTrayActionKind,
) desktop_tray.DesktopTrayActionKind {
	switch kind {
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_COPY_TEXT:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_COPY_TEXT
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_REVEAL_PATH:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_REVEAL_PATH
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER
	case projection_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT
	default:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_UNKNOWN
	}
}

func desktopTrayIconStateFromProjection(
	state projection_tray.DesktopTrayIconState,
) desktop_tray.DesktopTrayIconState {
	switch state {
	case projection_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL
	case projection_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE
	case projection_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ATTENTION:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ATTENTION
	case projection_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_DISCONNECTED:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_DISCONNECTED
	case projection_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_QUITTING:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_QUITTING
	default:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNKNOWN
	}
}

func desktopTraySeverityFromProjection(
	severity projection_tray.DesktopTraySeverity,
) desktop_tray.DesktopTraySeverity {
	switch severity {
	case projection_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_INFO:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_INFO
	case projection_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_WARNING:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_WARNING
	case projection_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_CRITICAL:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_CRITICAL
	default:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNKNOWN
	}
}
