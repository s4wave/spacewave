package projection

import (
	"slices"
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/selfenrollmentprojection"
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

// SessionProjectionRow carries one Session row snapshot into
// BuildSessionProjection.
type SessionProjectionRow struct {
	Entry          *session.SessionListEntry
	Metadata       *session.SessionMetadata
	AccountStatus  provider.ProviderAccountStatus
	SelfEnrollment *selfenrollmentprojection.Projection
}

// BuildSessionProjection builds the desktop-visible Session projection.
func BuildSessionProjection(rows []*SessionProjectionRow) *SessionProjection {
	sorted := make([]*SessionProjectionRow, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.Entry != nil {
			sorted = append(sorted, row)
		}
	}
	slices.SortFunc(sorted, func(a, b *SessionProjectionRow) int {
		aTime := a.Metadata.GetCreatedAt()
		bTime := b.Metadata.GetCreatedAt()
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
		aIdx := a.Entry.GetSessionIndex()
		bIdx := b.Entry.GetSessionIndex()
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

func buildSessionNavigationItem(row *SessionProjectionRow) *desktop_runtime.DesktopRuntimeNavigationItem {
	idx := row.Entry.GetSessionIndex()
	return &desktop_runtime.DesktopRuntimeNavigationItem{
		Id:         "session-" + strconv.FormatUint(uint64(idx), 10),
		Label:      SessionLabel(row),
		Detail:     sessionDetail(row),
		Route:      SessionRoute(idx),
		Active:     row.AccountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		StatusText: sessionStatusText(row),
	}
}

func buildSessionAttentionItem(row *SessionProjectionRow) *desktop_runtime.DesktopRuntimeAttentionItem {
	if row.AccountStatus == provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
		return &desktop_runtime.DesktopRuntimeAttentionItem{
			Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_AUTH_REQUIRED,
			Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING,
			Label:    "Sign in required",
			Detail:   SessionLabel(row),
			Route:    SessionRoute(row.Entry.GetSessionIndex()),
		}
	}
	if isSessionStepUpRequired(row) {
		return &desktop_runtime.DesktopRuntimeAttentionItem{
			Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_STEP_UP_REQUIRED,
			Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING,
			Label:    "Unlock session key",
			Detail:   spaceNeedSessionKeyDetail(row.SelfEnrollment.Count),
			Route:    SessionRoute(row.Entry.GetSessionIndex()),
		}
	}
	return nil
}

// SessionLabel returns the user-visible label for a Session row.
func SessionLabel(row *SessionProjectionRow) string {
	meta := row.Metadata
	if meta.GetDisplayName() != "" {
		return meta.GetDisplayName()
	}
	if meta.GetCloudEntityId() != "" {
		return meta.GetCloudEntityId()
	}
	if meta.GetProviderAccountId() != "" {
		return meta.GetProviderAccountId()
	}
	return "Session " + strconv.FormatUint(uint64(row.Entry.GetSessionIndex()), 10)
}

func sessionDetail(row *SessionProjectionRow) string {
	meta := row.Metadata
	providerLabel := meta.GetProviderDisplayName()
	if providerLabel == "" {
		providerLabel = providerLabelFromID(meta.GetProviderId())
	}
	if providerLabel == "" {
		providerLabel = providerLabelFromRef(row.Entry.GetSessionRef())
	}
	if meta.GetCloudEntityId() != "" && meta.GetCloudEntityId() != SessionLabel(row) {
		if providerLabel != "" {
			return providerLabel + " - " + meta.GetCloudEntityId()
		}
		return meta.GetCloudEntityId()
	}
	if providerLabel != "" {
		return providerLabel
	}
	return "Session " + strconv.FormatUint(uint64(row.Entry.GetSessionIndex()), 10)
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

// SessionRoute returns the app route for a Session index.
func SessionRoute(idx uint32) string {
	return "/u/" + strconv.FormatUint(uint64(idx), 10) + "/"
}

func sessionStatusText(row *SessionProjectionRow) string {
	if row.AccountStatus != provider.ProviderAccountStatus_ProviderAccountStatus_READY {
		return accountStatusText(row.AccountStatus)
	}
	if row.SelfEnrollment != nil {
		if len(row.SelfEnrollment.Failures) != 0 {
			return "Space connection failed"
		}
		if row.SelfEnrollment.Running {
			return "Connecting spaces"
		}
		if row.SelfEnrollment.Skipped {
			return "Connection skipped"
		}
		if isSessionStepUpRequired(row) {
			return "Unlock required"
		}
		if row.SelfEnrollment.Count != 0 {
			return "Spaces pending"
		}
	}
	return accountStatusText(row.AccountStatus)
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

func isSessionStepUpRequired(row *SessionProjectionRow) bool {
	if row == nil || row.SelfEnrollment == nil {
		return false
	}
	return row.SelfEnrollment.Count != 0 &&
		row.SelfEnrollment.CredentialRequired &&
		!row.SelfEnrollment.Running &&
		!row.SelfEnrollment.Skipped
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
