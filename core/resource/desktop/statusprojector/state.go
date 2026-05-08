package statusprojector

import (
	"slices"
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/provider"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

const maxProjectedSessions = 5

// SessionProjection contains tray-visible session rows and attention items.
type SessionProjection struct {
	Sessions       []*desktop_runtime.DesktopRuntimeNavigationItem
	Spaces         []*desktop_runtime.DesktopRuntimeNavigationItem
	Activity       []*desktop_runtime.DesktopRuntimeActivityItem
	Update         *desktop_runtime.DesktopRuntimeUpdateStatus
	AttentionItems []*desktop_runtime.DesktopRuntimeAttentionItem
}

type sessionProjectionRow struct {
	entry          *session.SessionListEntry
	metadata       *session.SessionMetadata
	accountStatus  provider.ProviderAccountStatus
	selfEnrollment *sessionSelfEnrollmentProjection
}

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

func buildSessionProjection(rows []*sessionProjectionRow) *SessionProjection {
	sorted := make([]*sessionProjectionRow, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.entry != nil {
			sorted = append(sorted, row)
		}
	}
	slices.SortFunc(sorted, func(a, b *sessionProjectionRow) int {
		aTime := a.metadata.GetCreatedAt()
		bTime := b.metadata.GetCreatedAt()
		if aTime != 0 && bTime != 0 && aTime != bTime {
			if aTime > bTime {
				return -1
			}
			return 1
		}
		if aTime != bTime {
			if aTime == 0 {
				return 1
			}
			return -1
		}
		aIdx := a.entry.GetSessionIndex()
		bIdx := b.entry.GetSessionIndex()
		if aIdx > bIdx {
			return -1
		}
		if aIdx < bIdx {
			return 1
		}
		return 0
	})

	out := &SessionProjection{
		Sessions:       []*desktop_runtime.DesktopRuntimeNavigationItem{},
		AttentionItems: []*desktop_runtime.DesktopRuntimeAttentionItem{},
	}
	for _, row := range sorted {
		if len(out.Sessions) < maxProjectedSessions {
			out.Sessions = append(out.Sessions, buildSessionNavigationItem(row))
		}
		if item := buildSessionAttentionItem(row); item != nil {
			out.AttentionItems = append(out.AttentionItems, item)
		}
	}
	return out
}

func buildSessionNavigationItem(row *sessionProjectionRow) *desktop_runtime.DesktopRuntimeNavigationItem {
	idx := row.entry.GetSessionIndex()
	return &desktop_runtime.DesktopRuntimeNavigationItem{
		Id:         "session-" + strconv.FormatUint(uint64(idx), 10),
		Label:      sessionLabel(row),
		Detail:     sessionDetail(row),
		Route:      sessionRoute(idx),
		Active:     row.accountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		StatusText: sessionStatusText(row),
	}
}

func buildSessionAttentionItem(row *sessionProjectionRow) *desktop_runtime.DesktopRuntimeAttentionItem {
	if row.accountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
		return &desktop_runtime.DesktopRuntimeAttentionItem{
			Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_AUTH_REQUIRED,
			Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING,
			Label:    "Sign in required",
			Detail:   sessionLabel(row),
			Route:    sessionRoute(row.entry.GetSessionIndex()),
		}
	}
	if isSessionStepUpRequired(row) {
		return &desktop_runtime.DesktopRuntimeAttentionItem{
			Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_STEP_UP_REQUIRED,
			Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING,
			Label:    "Unlock session key",
			Detail:   spaceNeedSessionKeyDetail(row.selfEnrollment.count),
			Route:    sessionRoute(row.entry.GetSessionIndex()),
		}
	}
	return nil
}

func sessionLabel(row *sessionProjectionRow) string {
	meta := row.metadata
	if meta.GetDisplayName() != "" {
		return meta.GetDisplayName()
	}
	if meta.GetCloudEntityId() != "" {
		return meta.GetCloudEntityId()
	}
	if meta.GetProviderAccountId() != "" {
		return meta.GetProviderAccountId()
	}
	return "Session " + strconv.FormatUint(uint64(row.entry.GetSessionIndex()), 10)
}

func sessionDetail(row *sessionProjectionRow) string {
	meta := row.metadata
	providerLabel := meta.GetProviderDisplayName()
	if providerLabel == "" {
		providerLabel = providerLabelFromID(meta.GetProviderId())
	}
	if providerLabel == "" {
		providerLabel = providerLabelFromRef(row.entry.GetSessionRef())
	}
	if meta.GetCloudEntityId() != "" && meta.GetCloudEntityId() != sessionLabel(row) {
		if providerLabel != "" {
			return providerLabel + " - " + meta.GetCloudEntityId()
		}
		return meta.GetCloudEntityId()
	}
	if providerLabel != "" {
		return providerLabel
	}
	return "Session " + strconv.FormatUint(uint64(row.entry.GetSessionIndex()), 10)
}

func providerLabelFromID(providerID string) string {
	switch providerID {
	case "spacewave":
		return "Cloud"
	case "local":
		return "Local"
	default:
		return providerID
	}
}

func providerLabelFromRef(ref *session.SessionRef) string {
	return providerLabelFromID(ref.GetProviderResourceRef().GetProviderId())
}

func sessionRoute(idx uint32) string {
	return "/u/" + strconv.FormatUint(uint64(idx), 10) + "/"
}

func sessionStatusText(row *sessionProjectionRow) string {
	if row.accountStatus != provider.ProviderAccountStatus_ProviderAccountStatus_READY {
		return accountStatusText(row.accountStatus)
	}
	if row.selfEnrollment != nil {
		if row.selfEnrollment.failed {
			return "Space connection failed"
		}
		if row.selfEnrollment.running {
			return "Connecting spaces"
		}
		if row.selfEnrollment.skipped {
			return "Connection skipped"
		}
		if isSessionStepUpRequired(row) {
			return "Unlock required"
		}
		if row.selfEnrollment.count != 0 {
			return "Spaces pending"
		}
	}
	return accountStatusText(row.accountStatus)
}

func accountStatusText(status provider.ProviderAccountStatus) string {
	switch status {
	case provider.ProviderAccountStatus_ProviderAccountStatus_READY:
		return "Ready"
	case provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED:
		return "Sign in required"
	case provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT:
		return "Inactive"
	case provider.ProviderAccountStatus_ProviderAccountStatus_DELETED:
		return "Deleted"
	case provider.ProviderAccountStatus_ProviderAccountStatus_FAILED:
		return "Account error"
	case provider.ProviderAccountStatus_ProviderAccountStatus_PENDING:
		return "Starting"
	default:
		return "Unknown"
	}
}

func isSessionStepUpRequired(row *sessionProjectionRow) bool {
	if row == nil || row.selfEnrollment == nil {
		return false
	}
	return row.selfEnrollment.count != 0 &&
		row.selfEnrollment.credentialRequired &&
		!row.selfEnrollment.running &&
		!row.selfEnrollment.skipped
}

func formatSpaceCount(count uint32) string {
	if count == 1 {
		return "1 space"
	}
	return strconv.FormatUint(uint64(count), 10) + " spaces"
}

func spaceNeedSessionKeyDetail(count uint32) string {
	if count == 1 {
		return "1 space needs this session key"
	}
	return formatSpaceCount(count) + " need this session key"
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
