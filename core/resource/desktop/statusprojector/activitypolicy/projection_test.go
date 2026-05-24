package activitypolicy

import (
	"testing"
	"time"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
)

func TestBuildMapsActiveSync(t *testing.T) {
	projection := Build([]*Row{
		{
			SessionIndex:       4,
			SessionLabel:       "Cloud",
			State:              SyncStateActive,
			Direction:          SyncDirectionUpload,
			PendingUploadCount: 2,
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

func TestBuildSortsErrorsBeforeDone(t *testing.T) {
	doneAt := time.Unix(100, 0)
	projection := Build([]*Row{
		{
			SessionIndex:          1,
			SessionLabel:          "Done",
			State:                 SyncStateSynced,
			LastActivityAtUnixMs:  doneAt.UnixMilli(),
			HasLastActivityAtTime: true,
		},
		{
			SessionIndex: 2,
			SessionLabel: "Broken",
			State:        SyncStateError,
			LastError:    "sync failed",
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

func TestBuildBoundsRows(t *testing.T) {
	rows := make([]*Row, 0, maxProjectedActivity+1)
	for i := uint32(1); i <= maxProjectedActivity+1; i++ {
		rows = append(rows, &Row{
			SessionIndex:       i,
			State:              SyncStateActive,
			Direction:          SyncDirectionUpload,
			PendingUploadCount: 1,
		})
	}
	projection := Build(rows)
	if len(projection) != maxProjectedActivity {
		t.Fatalf("activity rows = %d, want %d", len(projection), maxProjectedActivity)
	}
}
