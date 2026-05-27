package activitypolicy

import (
	"slices"
	"strconv"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
)

const maxProjectedActivity = 5

// SyncState names the coarse sync state used for desktop activity projection.
type SyncState uint8

const (
	// SyncStateIdle means no tray-visible sync activity is active.
	SyncStateIdle SyncState = iota
	// SyncStateActive means sync is currently running.
	SyncStateActive
	// SyncStateSynced means sync finished and may have a completion timestamp.
	SyncStateSynced
	// SyncStateError means sync needs attention.
	SyncStateError
)

// SyncDirection names the active sync direction used in desktop copy.
type SyncDirection uint8

const (
	// SyncDirectionUnknown means sync is active but direction is not specific.
	SyncDirectionUnknown SyncDirection = iota
	// SyncDirectionUpload means local changes are being uploaded.
	SyncDirectionUpload
	// SyncDirectionDownload means remote updates are being downloaded.
	SyncDirectionDownload
	// SyncDirectionUploadDownload means sync is moving data in both directions.
	SyncDirectionUploadDownload
)

// Row contains one session sync status row to project into desktop activity.
type Row struct {
	SessionIndex          uint32
	SessionLabel          string
	State                 SyncState
	Direction             SyncDirection
	PendingUploadCount    uint64
	PendingDownloadCount  uint64
	InFlightUploadCount   uint64
	LastError             string
	LastActivityAtUnixMs  int64
	HasLastActivityAtTime bool
}

// Build maps session sync status rows into desktop runtime activity items.
func Build(rows []*Row) []*desktop_runtime.DesktopRuntimeActivityItem {
	items := make([]*desktop_runtime.DesktopRuntimeActivityItem, 0, min(len(rows), maxProjectedActivity))
	for _, row := range rows {
		item := buildItem(row)
		if item != nil {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b *desktop_runtime.DesktopRuntimeActivityItem) int {
		ap := statePriority(a.GetState())
		bp := statePriority(b.GetState())
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

func buildItem(row *Row) *desktop_runtime.DesktopRuntimeActivityItem {
	if row == nil {
		return nil
	}
	state := syncState(row)
	if state == desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_IDLE {
		return nil
	}
	updatedAt := updatedAtUnixMs(row)
	if state == desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE &&
		updatedAt == 0 {
		return nil
	}
	return &desktop_runtime.DesktopRuntimeActivityItem{
		Id:              "sync-" + strconv.FormatUint(uint64(row.SessionIndex), 10),
		Label:           label(row),
		Detail:          detail(row),
		State:           state,
		UpdatedAtUnixMs: updatedAt,
	}
}

func syncState(
	row *Row,
) desktop_runtime.DesktopRuntimeActivityState {
	switch row.State {
	case SyncStateError:
		return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_ERROR
	case SyncStateActive:
		return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING
	case SyncStateSynced:
		if row.HasLastActivityAtTime {
			return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE
		}
	}
	return desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_IDLE
}

func label(row *Row) string {
	switch row.State {
	case SyncStateError:
		return "Sync needs attention"
	case SyncStateActive:
		switch row.Direction {
		case SyncDirectionUpload:
			return "Uploading changes"
		case SyncDirectionDownload:
			return "Downloading updates"
		case SyncDirectionUploadDownload:
			return "Syncing changes"
		default:
			return "Sync active"
		}
	default:
		return "Synced"
	}
}

func detail(row *Row) string {
	if row.LastError != "" {
		return row.LastError
	}
	pending := row.PendingUploadCount + row.PendingDownloadCount + row.InFlightUploadCount
	if pending != 0 {
		return strconv.FormatUint(pending, 10) + " sync items"
	}
	if row.SessionLabel != "" {
		return row.SessionLabel
	}
	return "Session " + strconv.FormatUint(uint64(row.SessionIndex), 10)
}

func updatedAtUnixMs(row *Row) int64 {
	if !row.HasLastActivityAtTime {
		return 0
	}
	return row.LastActivityAtUnixMs
}

func statePriority(state desktop_runtime.DesktopRuntimeActivityState) int {
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
