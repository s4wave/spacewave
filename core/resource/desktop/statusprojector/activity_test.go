//go:build !goscript

package statusprojector

import (
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

func TestBuildActivityProjectionMapsActiveSync(t *testing.T) {
	projection := buildActivityProjection([]*activityProjectionRow{
		{
			sessionIndex: 4,
			sessionLabel: "Cloud",
			status: &s4wave_session.WatchSyncStatusResponse{
				State:              s4wave_session.SyncStatusState_SyncStatusState_ACTIVE,
				Direction:          s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD,
				PendingUploadCount: 2,
			},
		},
	})
	if len(projection) != 1 {
		t.Fatalf("activity rows = %d, want 1", len(projection))
	}
	row := projection[0]
	if row.GetLabel() != "Uploading changes" {
		t.Fatalf("label = %q, want upload activity", row.GetLabel())
	}
	if row.GetDetail() != "2 sync items" {
		t.Fatalf("detail = %q, want pending item count", row.GetDetail())
	}
	if row.GetState() != desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING {
		t.Fatalf("state = %v, want running", row.GetState())
	}
}

func TestBuildActivityProjectionSortsErrorsBeforeDone(t *testing.T) {
	doneAt := time.Unix(100, 0)
	projection := buildActivityProjection([]*activityProjectionRow{
		{
			sessionIndex: 1,
			sessionLabel: "Done",
			status: &s4wave_session.WatchSyncStatusResponse{
				State:          s4wave_session.SyncStatusState_SyncStatusState_SYNCED,
				LastActivityAt: timestamppb.New(doneAt),
			},
		},
		{
			sessionIndex: 2,
			sessionLabel: "Broken",
			status: &s4wave_session.WatchSyncStatusResponse{
				State:     s4wave_session.SyncStatusState_SyncStatusState_ERROR,
				LastError: "sync failed",
			},
		},
	})
	if len(projection) != 2 {
		t.Fatalf("activity rows = %d, want 2", len(projection))
	}
	if projection[0].GetState() != desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_ERROR {
		t.Fatalf("first state = %v, want error first", projection[0].GetState())
	}
	if projection[0].GetDetail() != "sync failed" {
		t.Fatalf("first detail = %q, want sync error", projection[0].GetDetail())
	}
	if projection[1].GetUpdatedAtUnixMs() != doneAt.UnixMilli() {
		t.Fatalf("done timestamp = %d, want %d", projection[1].GetUpdatedAtUnixMs(), doneAt.UnixMilli())
	}
}

func TestBuildActivityProjectionBoundsRows(t *testing.T) {
	rows := make([]*activityProjectionRow, 0, maxProjectedActivity+1)
	for i := uint32(1); i <= maxProjectedActivity+1; i++ {
		rows = append(rows, &activityProjectionRow{
			sessionIndex: i,
			status: &s4wave_session.WatchSyncStatusResponse{
				State:              s4wave_session.SyncStatusState_SyncStatusState_ACTIVE,
				Direction:          s4wave_session.SyncActivityDirection_SyncActivityDirection_UPLOAD,
				PendingUploadCount: 1,
			},
		})
	}
	projection := buildActivityProjection(rows)
	if len(projection) != maxProjectedActivity {
		t.Fatalf("activity rows = %d, want %d", len(projection), maxProjectedActivity)
	}
}
