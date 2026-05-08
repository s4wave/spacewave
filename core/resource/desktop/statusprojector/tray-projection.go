package statusprojector

import (
	"strings"

	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
)

// BuildDesktopTrayEntriesFromRuntimeState maps runtime status into tray entries.
func BuildDesktopTrayEntriesFromRuntimeState(
	state *desktop_runtime.DesktopRuntimeState,
) []*desktop_tray.DesktopTrayEntry {
	if len(state.GetAttentionItems()) != 0 {
		return orderedTrayEntries(buildAttentionTrayEntries(state))
	}
	return orderedTrayEntries(buildHealthyTrayEntries(state))
}

func buildHealthyTrayEntries(state *desktop_runtime.DesktopRuntimeState) []*desktop_tray.DesktopTrayEntry {
	return joinTrayEntries(
		[]*desktop_tray.DesktopTrayEntry{
			statusTrayEntryWithHints(
				"title",
				"Spacewave: "+runtimeStatusText(state),
				false,
				iconStateForRuntimeHealth(state.GetHealth()),
				desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED,
			),
			separatorTrayEntry("open-separator"),
			actionTrayEntry("open", "Open Spacewave", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE, "", "", true),
			actionTrayEntry("new-window", "New Window", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW, "", "", true),
			separatorTrayEntry("status-separator"),
		},
		buildStatusTraySection(state),
		buildNavigationTraySection("Sessions", state.GetSessions(), "No sessions"),
		buildNavigationTraySection("Spaces", state.GetSpaces(), "No spaces"),
		buildActivityTraySection(state.GetActivity()),
		buildActionTraySection(state),
		[]*desktop_tray.DesktopTrayEntry{
			separatorTrayEntry("app-separator"),
			actionTrayEntry("settings", "Settings...", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE, settingsTrayRoute(state), "", true),
			actionTrayEntry("about", "About Spacewave", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE, "/about", "", true),
			separatorTrayEntry("quit-separator"),
			actionTrayEntry("quit", "Quit", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT, "", "", true),
		},
	)
}

func buildAttentionTrayEntries(state *desktop_runtime.DesktopRuntimeState) []*desktop_tray.DesktopTrayEntry {
	item := selectPrimaryAttentionItem(state.GetAttentionItems())
	label := "Needs attention"
	var detail string
	if item != nil {
		label = item.GetLabel()
		detail = item.GetDetail()
	}
	entries := []*desktop_tray.DesktopTrayEntry{
		statusTrayEntryWithHints(
			"title",
			"Spacewave: "+runtimeStatusText(state),
			false,
			iconStateForRuntimeHealth(state.GetHealth()),
			desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED,
		),
		statusTrayEntryWithHints(
			"attention-primary",
			label,
			false,
			desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNSPECIFIED,
			severityFromRuntimeSeverity(item.GetSeverity()),
		),
	}
	if detail != "" {
		entries = append(entries, statusTrayEntry("attention-detail", detail))
	}
	entries = append(entries,
		separatorTrayEntry("open-separator"),
		actionTrayEntry("open", "Open Spacewave", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE, "", "", true),
	)
	if state.GetUpdate().GetReady() {
		entries = append(entries,
			separatorTrayEntry("quick-actions-separator"),
			sectionTrayEntry("quick-actions-section", "Quick Actions"),
			applyUpdateTrayEntry(state.GetUpdate()),
		)
	}
	entries = append(entries,
		separatorTrayEntry("quit-separator"),
		actionTrayEntry("quit", "Quit", desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT, "", "", true),
	)
	return entries
}

func buildStatusTraySection(state *desktop_runtime.DesktopRuntimeState) []*desktop_tray.DesktopTrayEntry {
	listener := state.GetListener()
	entries := []*desktop_tray.DesktopTrayEntry{
		sectionTrayEntry("status-section", "Status"),
		statusTrayEntry(
			"status-runtime",
			compactTrayLabel(listener.GetLabel(), listener.GetDetail(), state.GetStatusText()),
		),
	}
	if state.GetUpdate().GetLabel() != "" {
		entries = append(entries, statusTrayEntry(
			"status-update",
			compactTrayLabel("Update", state.GetUpdate().GetLabel(), state.GetUpdate().GetDetail()),
		))
	}
	return entries
}

func buildNavigationTraySection(
	title string,
	items []*desktop_runtime.DesktopRuntimeNavigationItem,
	emptyLabel string,
) []*desktop_tray.DesktopTrayEntry {
	entries := []*desktop_tray.DesktopTrayEntry{
		separatorTrayEntry(title + "-separator"),
		sectionTrayEntry(title+"-section", title),
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		entries = append(entries, buildNavigationTrayItem(item))
	}
	if len(items) == 0 {
		entries = append(entries, statusTrayEntry(title+"-empty", emptyLabel))
	}
	return entries
}

func buildActivityTraySection(
	items []*desktop_runtime.DesktopRuntimeActivityItem,
) []*desktop_tray.DesktopTrayEntry {
	if len(items) == 0 {
		return nil
	}
	entries := []*desktop_tray.DesktopTrayEntry{
		separatorTrayEntry("activity-separator"),
		sectionTrayEntry("activity-section", "Activity"),
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		label := compactTrayLabel(item.GetLabel(), item.GetDetail())
		entries = append(entries, statusTrayEntry("activity-"+fallbackTrayID(item.GetId(), label), label))
	}
	return entries
}

func buildActionTraySection(state *desktop_runtime.DesktopRuntimeState) []*desktop_tray.DesktopTrayEntry {
	actions := make([]*desktop_tray.DesktopTrayEntry, 0, len(state.GetActions())+3)
	if state.GetUpdate().GetReady() {
		actions = append(actions, applyUpdateTrayEntry(state.GetUpdate()))
	}
	for _, item := range buildSyntheticTrayActions(state) {
		if item == nil {
			continue
		}
		actions = append(actions, buildActionTrayItem(item))
	}
	for _, item := range state.GetActions() {
		if item == nil {
			continue
		}
		actions = append(actions, buildActionTrayItem(item))
	}
	if len(actions) == 0 {
		return nil
	}

	entries := []*desktop_tray.DesktopTrayEntry{
		separatorTrayEntry("quick-actions-separator"),
		sectionTrayEntry("quick-actions-section", "Quick Actions"),
	}
	return append(entries, actions...)
}

func buildNavigationTrayItem(item *desktop_runtime.DesktopRuntimeNavigationItem) *desktop_tray.DesktopTrayEntry {
	label := compactTrayLabel(item.GetLabel(), item.GetDetail(), item.GetStatusText())
	id := "navigation-" + fallbackTrayID(item.GetId(), label)
	if item.GetRoute() == "" {
		return statusTrayEntryWithHints(
			id,
			label,
			item.GetActive(),
			desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNSPECIFIED,
			desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED,
		)
	}
	return actionTrayEntryWithActive(
		id,
		label,
		desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE,
		item.GetRoute(),
		"",
		true,
		item.GetActive(),
	)
}

func settingsTrayRoute(state *desktop_runtime.DesktopRuntimeState) string {
	session := selectSettingsSession(state.GetSessions())
	if session == nil || session.GetRoute() == "" {
		return "/settings"
	}
	return strings.TrimRight(session.GetRoute(), "/") + "/settings/cli"
}

func selectSettingsSession(
	items []*desktop_runtime.DesktopRuntimeNavigationItem,
) *desktop_runtime.DesktopRuntimeNavigationItem {
	var fallback *desktop_runtime.DesktopRuntimeNavigationItem
	for _, item := range items {
		if item == nil || item.GetRoute() == "" {
			continue
		}
		if fallback == nil {
			fallback = item
		}
		if item.GetActive() {
			return item
		}
	}
	return fallback
}

func buildSyntheticTrayActions(state *desktop_runtime.DesktopRuntimeState) []*desktop_runtime.DesktopRuntimeActionItem {
	socketPath := state.GetListener().GetSocketPath()
	if socketPath == "" {
		return nil
	}
	return []*desktop_runtime.DesktopRuntimeActionItem{
		{
			Id:      "copy-cli-socket",
			Kind:    desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_COPY_TEXT,
			Label:   "Copy Socket Path",
			Value:   socketPath,
			Enabled: true,
		},
		{
			Id:      "copy-diagnostics",
			Kind:    desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_COPY_TEXT,
			Label:   "Copy Diagnostics",
			Value:   buildTrayDiagnosticText(state),
			Enabled: true,
		},
	}
}

func buildActionTrayItem(item *desktop_runtime.DesktopRuntimeActionItem) *desktop_tray.DesktopTrayEntry {
	label := compactTrayLabel(item.GetLabel(), item.GetDetail())
	return actionTrayEntry(
		"action-"+fallbackTrayID(item.GetId(), label),
		label,
		actionKindFromRuntimeActionKind(item.GetKind()),
		item.GetRoute(),
		item.GetValue(),
		item.GetEnabled(),
	)
}

func applyUpdateTrayEntry(update *desktop_runtime.DesktopRuntimeUpdateStatus) *desktop_tray.DesktopTrayEntry {
	entry := actionTrayEntry(
		"apply-update",
		"Install Update",
		desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_ATTACHED_HANDLER,
		"",
		update.GetVersion(),
		update.GetReady(),
	)
	entry.Severity = desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_INFO
	return entry
}

func statusTrayEntry(id, label string) *desktop_tray.DesktopTrayEntry {
	return statusTrayEntryWithHints(
		id,
		label,
		false,
		desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_UNSPECIFIED,
		desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED,
	)
}

func statusTrayEntryWithHints(
	id, label string,
	active bool,
	iconState desktop_tray.DesktopTrayIconState,
	severity desktop_tray.DesktopTraySeverity,
) *desktop_tray.DesktopTrayEntry {
	return &desktop_tray.DesktopTrayEntry{
		Id:        id,
		Kind:      desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
		Label:     label,
		Active:    active,
		IconState: iconState,
		Severity:  severity,
	}
}

func sectionTrayEntry(id, label string) *desktop_tray.DesktopTrayEntry {
	return &desktop_tray.DesktopTrayEntry{
		Id:    id,
		Kind:  desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SECTION,
		Label: label,
	}
}

func separatorTrayEntry(id string) *desktop_tray.DesktopTrayEntry {
	return &desktop_tray.DesktopTrayEntry{
		Id:   id,
		Kind: desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_SEPARATOR,
	}
}

func actionTrayEntry(
	id, label string,
	kind desktop_tray.DesktopTrayActionKind,
	route, value string,
	enabled bool,
) *desktop_tray.DesktopTrayEntry {
	return actionTrayEntryWithActive(id, label, kind, route, value, enabled, false)
}

func actionTrayEntryWithActive(
	id, label string,
	kind desktop_tray.DesktopTrayActionKind,
	route, value string,
	enabled, active bool,
) *desktop_tray.DesktopTrayEntry {
	return &desktop_tray.DesktopTrayEntry{
		Id:      id,
		Kind:    desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION,
		Label:   label,
		Active:  active,
		Enabled: enabled,
		Action: &desktop_tray.DesktopTrayAction{
			Kind:  kind,
			Route: route,
			Value: value,
		},
	}
}

func orderedTrayEntries(entries []*desktop_tray.DesktopTrayEntry) []*desktop_tray.DesktopTrayEntry {
	for idx, entry := range entries {
		if entry == nil {
			continue
		}
		entry.Order = int32(idx)
	}
	return entries
}

func joinTrayEntries(groups ...[]*desktop_tray.DesktopTrayEntry) []*desktop_tray.DesktopTrayEntry {
	var total int
	for _, group := range groups {
		total += len(group)
	}
	out := make([]*desktop_tray.DesktopTrayEntry, 0, total)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func runtimeStatusText(state *desktop_runtime.DesktopRuntimeState) string {
	if state.GetStatusText() != "" {
		return state.GetStatusText()
	}
	return "Running"
}

func compactTrayLabel(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nonEmpty = append(nonEmpty, part)
	}
	return strings.Join(nonEmpty, " - ")
}

func fallbackTrayID(id, label string) string {
	if id != "" {
		return id
	}
	return label
}

func selectPrimaryAttentionItem(
	items []*desktop_runtime.DesktopRuntimeAttentionItem,
) *desktop_runtime.DesktopRuntimeAttentionItem {
	var selected *desktop_runtime.DesktopRuntimeAttentionItem
	for _, item := range items {
		if item == nil {
			continue
		}
		if selected == nil {
			selected = item
			continue
		}
		if runtimeSeverityPriority(item.GetSeverity()) > runtimeSeverityPriority(selected.GetSeverity()) {
			selected = item
			continue
		}
		if runtimeSeverityPriority(item.GetSeverity()) == runtimeSeverityPriority(selected.GetSeverity()) &&
			item.GetLabel() < selected.GetLabel() {
			selected = item
		}
	}
	return selected
}

func runtimeSeverityPriority(severity desktop_runtime.DesktopRuntimeSeverity) int32 {
	if severity == desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_UNSPECIFIED {
		return int32(desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_INFO)
	}
	return int32(severity)
}

func severityFromRuntimeSeverity(
	severity desktop_runtime.DesktopRuntimeSeverity,
) desktop_tray.DesktopTraySeverity {
	switch severity {
	case desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_CRITICAL:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_CRITICAL
	case desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_WARNING
	case desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_INFO:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_INFO
	default:
		return desktop_tray.DesktopTraySeverity_DESKTOP_TRAY_SEVERITY_UNSPECIFIED
	}
}

func iconStateForRuntimeHealth(
	health desktop_runtime.DesktopRuntimeHealth,
) desktop_tray.DesktopTrayIconState {
	switch health {
	case desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_ACTIVE:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ACTIVE
	case desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_NEEDS_ATTENTION:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_ATTENTION
	case desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_DISCONNECTED
	case desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_QUITTING:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_QUITTING
	default:
		return desktop_tray.DesktopTrayIconState_DESKTOP_TRAY_ICON_STATE_NORMAL
	}
}

func actionKindFromRuntimeActionKind(
	kind desktop_runtime.DesktopRuntimeActionKind,
) desktop_tray.DesktopTrayActionKind {
	switch kind {
	case desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_OPEN_ROUTE:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE
	case desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_NEW_WINDOW:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_NEW_WINDOW
	case desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_COPY_TEXT:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_COPY_TEXT
	case desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_REVEAL_PATH:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_REVEAL_PATH
	case desktop_runtime.DesktopRuntimeActionKind_DESKTOP_RUNTIME_ACTION_KIND_QUIT:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_QUIT
	default:
		return desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_UNSPECIFIED
	}
}

func buildTrayDiagnosticText(state *desktop_runtime.DesktopRuntimeState) string {
	lines := []string{
		"Spacewave: " + runtimeStatusText(state),
		compactTrayLabel(state.GetListener().GetLabel(), state.GetListener().GetDetail()),
	}
	if state.GetListener().GetSocketPath() != "" {
		lines = append(lines, "Socket: "+state.GetListener().GetSocketPath())
	}
	return strings.Join(lines, "\n")
}
