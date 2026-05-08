package statusprojector

import (
	"testing"

	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

func TestBuildDesktopRuntimeStateFromListenerReachable(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath:       "/run/spacewave.sock",
		Listening:        true,
		ConnectedClients: 2,
	})
	if state.GetStatusText() != "Running" {
		t.Fatalf("status text = %q, want Running", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY {
		t.Fatalf("health = %v, want healthy", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING {
		t.Fatalf("lifecycle = %v, want running", state.GetLifecycle())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_REACHABLE {
		t.Fatalf("listener reachability = %v, want reachable", listener.GetReachability())
	}
	if listener.GetDetail() != "2 CLI clients connected" {
		t.Fatalf("listener detail = %q, want connected client count", listener.GetDetail())
	}
}

func TestBuildDesktopRuntimeStateFromListenerReachableWithoutClientsStaysCompact(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	})
	listener := state.GetListener()
	if listener.GetDetail() != "Ready" {
		t.Fatalf("listener detail = %q, want Ready", listener.GetDetail())
	}
	if listener.GetSocketPath() != "/run/spacewave.sock" {
		t.Fatalf("listener socket = %q, want configured path", listener.GetSocketPath())
	}
}

func TestBuildDesktopRuntimeStateFromListenerStarting(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
	})
	if state.GetStatusText() != "Starting" {
		t.Fatalf("status text = %q, want Starting", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_STARTING {
		t.Fatalf("health = %v, want starting", state.GetHealth())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_STARTING {
		t.Fatalf("listener reachability = %v, want starting", listener.GetReachability())
	}
	if listener.GetSocketPath() != "/run/spacewave.sock" {
		t.Fatalf("listener socket = %q, want configured path", listener.GetSocketPath())
	}
}

func TestBuildDesktopRuntimeStateFromListenerDisconnected(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{})
	if state.GetStatusText() != "Disconnected" {
		t.Fatalf("status text = %q, want Disconnected", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED {
		t.Fatalf("health = %v, want disconnected", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED {
		t.Fatalf("lifecycle = %v, want disconnected", state.GetLifecycle())
	}
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_UNREACHABLE {
		t.Fatalf("listener reachability = %v, want unreachable", listener.GetReachability())
	}
	if len(state.GetSessions()) != 0 || len(state.GetSpaces()) != 0 || len(state.GetActivity()) != 0 {
		t.Fatalf("bounded row lists must start empty")
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateIncludesNavigationRows(t *testing.T) {
	state := BuildDesktopRuntimeState(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	}, &SessionProjection{
		Sessions: []*desktop_runtime.DesktopRuntimeNavigationItem{
			{
				Id:         "session-1",
				Label:      "coolguy@spacewave.app",
				Detail:     "Cloud",
				Route:      "/u/1/",
				StatusText: "Ready",
			},
		},
		Spaces: []*desktop_runtime.DesktopRuntimeNavigationItem{
			{
				Id:    "space-1",
				Label: "My Drive",
				Route: "/u/1/so/space-1",
			},
		},
	})

	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	if !hasTrayEntryLabel(entries, "coolguy@spacewave.app - Cloud - Ready") {
		t.Fatalf("expected session row in tray entries")
	}
	if !hasTrayEntryLabel(entries, "My Drive") {
		t.Fatalf("expected space row in tray entries")
	}
	if hasTrayEntryLabel(entries, "No sessions") || hasTrayEntryLabel(entries, "No spaces") {
		t.Fatalf("did not expect empty navigation rows when entries exist")
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateRoutesSettingsToActiveSession(t *testing.T) {
	state := BuildDesktopRuntimeState(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	}, &SessionProjection{
		Sessions: []*desktop_runtime.DesktopRuntimeNavigationItem{
			{
				Id:     "session-1",
				Label:  "first@example.com",
				Route:  "/u/1/",
				Active: false,
			},
			{
				Id:     "session-2",
				Label:  "active@example.com",
				Route:  "/u/2/",
				Active: true,
			},
		},
	})

	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	entry := findTrayEntryByID(entries, "settings")
	if entry == nil {
		t.Fatalf("expected settings tray entry")
	}
	if entry.GetAction().GetRoute() != "/u/2/settings/cli" {
		t.Fatalf("settings route = %q, want active session cli settings", entry.GetAction().GetRoute())
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateOrdersMenuSections(t *testing.T) {
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	})

	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	want := []string{
		"Spacewave: Running",
		"",
		"Open Spacewave",
		"New Window",
	}
	for idx, label := range want {
		if entries[idx].GetLabel() != label {
			t.Fatalf("entry %d label = %q, want %q", idx, entries[idx].GetLabel(), label)
		}
		if entries[idx].GetOrder() != int32(idx) {
			t.Fatalf("entry %d order = %d, want %d", idx, entries[idx].GetOrder(), idx)
		}
	}
	if hasTrayEntryLabel(entries, "/run/spacewave.sock") {
		t.Fatalf("did not expect socket path in visible tray labels")
	}
}

func TestBuildSessionProjectionSortsAndFlagsAuth(t *testing.T) {
	projection := buildSessionProjection([]*sessionProjectionRow{
		{
			entry: testSessionEntry(1, "spacewave", "acct-1"),
			metadata: &session.SessionMetadata{
				DisplayName:         "old@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           1,
			},
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		},
		{
			entry: testSessionEntry(2, "spacewave", "acct-2"),
			metadata: &session.SessionMetadata{
				DisplayName:         "new@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           2,
			},
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED,
		},
	})
	if len(projection.Sessions) != 2 {
		t.Fatalf("session rows = %d, want 2", len(projection.Sessions))
	}
	if projection.Sessions[0].GetLabel() != "new@example.com" {
		t.Fatalf("first session label = %q, want newest session first", projection.Sessions[0].GetLabel())
	}
	if projection.Sessions[0].GetStatusText() != "Sign in required" {
		t.Fatalf("first session status = %q, want auth attention", projection.Sessions[0].GetStatusText())
	}
	if len(projection.AttentionItems) != 1 {
		t.Fatalf("attention items = %d, want 1", len(projection.AttentionItems))
	}
	attention := projection.AttentionItems[0]
	if attention.GetKind() != desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_AUTH_REQUIRED {
		t.Fatalf("attention kind = %v, want auth required", attention.GetKind())
	}
	if attention.GetRoute() != "/u/2/" {
		t.Fatalf("attention route = %q, want session route", attention.GetRoute())
	}

	state := BuildDesktopRuntimeState(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	}, projection)
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_NEEDS_ATTENTION {
		t.Fatalf("health = %v, want needs attention", state.GetHealth())
	}
	if state.GetStatusText() != "Needs attention" {
		t.Fatalf("status text = %q, want Needs attention", state.GetStatusText())
	}
}

func TestBuildSessionProjectionFlagsStepUp(t *testing.T) {
	projection := buildSessionProjection([]*sessionProjectionRow{
		{
			entry: testSessionEntry(7, "spacewave", "acct-7"),
			metadata: &session.SessionMetadata{
				DisplayName:         "cloud@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           7,
			},
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
			selfEnrollment: &provider_spacewave.SelfEnrollmentProjection{
				Count:              2,
				CredentialRequired: true,
			},
		},
	})
	if len(projection.Sessions) != 1 {
		t.Fatalf("session rows = %d, want 1", len(projection.Sessions))
	}
	if projection.Sessions[0].GetStatusText() != "Unlock required" {
		t.Fatalf("session status = %q, want unlock status", projection.Sessions[0].GetStatusText())
	}
	if len(projection.AttentionItems) != 1 {
		t.Fatalf("attention items = %d, want 1", len(projection.AttentionItems))
	}
	attention := projection.AttentionItems[0]
	if attention.GetKind() != desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_STEP_UP_REQUIRED {
		t.Fatalf("attention kind = %v, want step-up required", attention.GetKind())
	}
	if attention.GetDetail() != "2 spaces need this session key" {
		t.Fatalf("attention detail = %q, want space count", attention.GetDetail())
	}
	if attention.GetRoute() != "/u/7/" {
		t.Fatalf("attention route = %q, want session route", attention.GetRoute())
	}
}

func TestBuildSessionProjectionUsesSharedSelfEnrollmentProjection(t *testing.T) {
	tests := []struct {
		name       string
		projection *provider_spacewave.SelfEnrollmentProjection
		wantStatus string
	}{
		{
			name: "failure",
			projection: &provider_spacewave.SelfEnrollmentProjection{
				Failures: []*provider_spacewave.SelfEnrollmentRunFailure{{SharedObjectID: "so-1"}},
			},
			wantStatus: "Space connection failed",
		},
		{
			name:       "running",
			projection: &provider_spacewave.SelfEnrollmentProjection{Running: true},
			wantStatus: "Connecting spaces",
		},
		{
			name: "skipped",
			projection: &provider_spacewave.SelfEnrollmentProjection{
				Count:   1,
				Skipped: true,
			},
			wantStatus: "Connection skipped",
		},
		{
			name:       "pending",
			projection: &provider_spacewave.SelfEnrollmentProjection{Count: 1},
			wantStatus: "Spaces pending",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := &sessionProjectionRow{
				entry:          testSessionEntry(1, "spacewave", "acct-1"),
				metadata:       &session.SessionMetadata{},
				accountStatus:  provider.ProviderAccountStatus_ProviderAccountStatus_READY,
				selfEnrollment: tt.projection,
			}
			if got := sessionStatusText(row); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

func TestBuildDesktopRuntimeStateMarksRunningActivity(t *testing.T) {
	state := BuildDesktopRuntimeState(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	}, &SessionProjection{
		Activity: []*desktop_runtime.DesktopRuntimeActivityItem{
			{
				Label: "Uploading changes",
				State: desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_RUNNING,
			},
		},
	})
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_ACTIVE {
		t.Fatalf("health = %v, want active", state.GetHealth())
	}
	if state.GetStatusText() != "Syncing" {
		t.Fatalf("status text = %q, want Syncing", state.GetStatusText())
	}
}

func TestBuildSessionProjectionBoundsSessionRows(t *testing.T) {
	rows := make([]*sessionProjectionRow, 0, maxProjectedSessions+1)
	for i := uint32(1); i <= maxProjectedSessions+1; i++ {
		rows = append(rows, &sessionProjectionRow{
			entry:         testSessionEntry(i, "local", "local"),
			metadata:      &session.SessionMetadata{CreatedAt: int64(i)},
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		})
	}
	projection := buildSessionProjection(rows)
	if len(projection.Sessions) != maxProjectedSessions {
		t.Fatalf("session rows = %d, want %d", len(projection.Sessions), maxProjectedSessions)
	}
}

func testSessionEntry(idx uint32, providerID, accountID string) *session.SessionListEntry {
	return &session.SessionListEntry{
		SessionIndex: idx,
		SessionRef: &session.SessionRef{
			ProviderResourceRef: &provider.ProviderResourceRef{
				Id:                "session",
				ProviderId:        providerID,
				ProviderAccountId: accountID,
			},
		},
	}
}

func hasTrayEntryLabel(entries []*desktop_tray.DesktopTrayEntry, label string) bool {
	for _, entry := range entries {
		if entry.GetLabel() == label {
			return true
		}
	}
	return false
}

func findTrayEntryByID(entries []*desktop_tray.DesktopTrayEntry, id string) *desktop_tray.DesktopTrayEntry {
	for _, entry := range entries {
		if entry.GetId() == id {
			return entry
		}
	}
	return nil
}
