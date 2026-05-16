package statusprojector

import (
	"slices"
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

const maxProjectedActivity = 5

type activityProjectionRow struct {
	sessionIndex uint32
	sessionLabel string
	status       *s4wave_session.WatchSyncStatusResponse
}

func buildActivityProjection(rows []*activityProjectionRow) []*desktop_runtime.DesktopRuntimeActivityItem {
	items := make([]*desktop_runtime.DesktopRuntimeActivityItem, 0, min(len(rows), maxProjectedActivity))
	for _, row := range rows {
		item := buildActivityItem(row)
		if item != nil {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b *desktop_runtime.DesktopRuntimeActivityItem) int {
		ap := activityStatePriority(a.GetState())
		bp := activityStatePriority(b.GetState())
		if ap != bp {
			return ap - bp
		}
		if a.GetUpdatedAtUnixMs() > b.GetUpdatedAtUnixMs() {
			return -1
		}
		if a.GetUpdatedAtUnixMs() < b.GetUpdatedAtUnixMs() {
			return 1
		}
		return 0
	})
	if len(items) > maxProjectedActivity {
		return items[:maxProjectedActivity]
	}
	return items
}

func buildActivityItem(row *activityProjectionRow) *desktop_runtime.DesktopRuntimeActivityItem {
	if row == nil || row.status == nil {
		return nil
	}
	status := row.status
	state := syncActivityState(status)
	if state == desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_IDLE {
		return nil
	}
	updatedAt := syncActivityUpdatedAtUnixMs(status)
	if state == desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE &&
		updatedAt == 0 {
		return nil
	}
	return &desktop_runtime.DesktopRuntimeActivityItem{
		Id:              "sync-" + strconv.FormatUint(uint64(row.sessionIndex), 10),
		Label:           syncActivityLabel(status),
		Detail:          syncActivityDetail(row),
		State:           state,
		UpdatedAtUnixMs: updatedAt,
	}
}

func syncActivityState(
	status *s4wave_session.WatchSyncStatusResponse,
) desktop_runtime.DesktopRuntimeActivityState {
	switch status.GetState() {
	case s4wave_session.SyncStatusState_SyncStatusState_ERROR:
		return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_ERROR
	case s4wave_session.SyncStatusState_SyncStatusState_ACTIVE:
		return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING
	case s4wave_session.SyncStatusState_SyncStatusState_SYNCED:
		if status.GetLastActivityAt() != nil && !status.GetLastActivityAt().GetEmpty() {
			return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE
		}
	}
	return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_IDLE
}

func syncActivityLabel(status *s4wave_session.WatchSyncStatusResponse) string {
	switch status.GetState() {
	case s4wave_session.SyncStatusState_SyncStatusState_ERROR:
		return "Sync needs attention"
	case s4wave_session.SyncStatusState_SyncStatusState_ACTIVE:
		switch status.GetDirection() {
		case s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD:
			return "Uploading changes"
		case s4wave_session.SyncActivityDirection_SyncActivityDirection_DOWNLOAD:
			return "Downloading updates"
		case s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD_DOWNLOAD:
			return "Syncing changes"
		default:
			return "Sync active"
		}
	default:
		return "Synced"
	}
}

func syncActivityDetail(row *activityProjectionRow) string {
	status := row.status
	if status.GetLastError() != "" {
		return status.GetLastError()
	}
	pending := status.GetPendingUploadCount() + status.GetPendingDownloadCount() +
		status.GetInFlightUploadCount()
	if pending != 0 {
		return strconv.FormatUint(uint64(pending), 10) + " sync items"
	}
	if row.sessionLabel != "" {
		return row.sessionLabel
	}
	return "Session " + strconv.FormatUint(uint64(row.sessionIndex), 10)
}

func syncActivityUpdatedAtUnixMs(status *s4wave_session.WatchSyncStatusResponse) int64 {
	ts := status.GetLastActivityAt()
	if ts == nil || ts.GetEmpty() {
		return 0
	}
	return ts.AsTime().UnixMilli()
}

func activityStatePriority(state desktop_runtime.DesktopRuntimeActivityState) int {
	switch state {
	case desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_ERROR,
		desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_ATTENTION:
		return 0
	case desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING:
		return 1
	case desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE:
		return 2
	default:
		return 3
	}
}
