package projection

import (
	"testing"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/selfenrollmentprojection"
	desktop_tray "github.com/s4wave/spacewave/core/resource/desktop/statusprojector/projection/traymodel"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

func TestBuildDesktopRuntimeStateFromListenerReachable(t *testing.T) {
	// Project a listening socket with two connected CLI clients.
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath:       "/run/spacewave.sock",
		Listening:        true,
		ConnectedClients: 2,
	})

	// Verify the runtime is healthy and running after its listener binds.
	if state.GetStatusText() != "Running" {
		t.Fatalf("status text = %q, want Running", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY {
		t.Fatalf("health = %v, want healthy", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING {
		t.Fatalf("lifecycle = %v, want running", state.GetLifecycle())
	}

	// Verify listener reachability includes the connected client count.
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
	// Project a configured socket whose listener has not yet bound.
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
	})

	// Verify the runtime remains starting until the listener binds.
	if state.GetStatusText() != "Starting" {
		t.Fatalf("status text = %q, want Starting", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_STARTING {
		t.Fatalf("health = %v, want starting", state.GetHealth())
	}

	// Verify the listener retains its socket path while starting.
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_STARTING {
		t.Fatalf("listener reachability = %v, want starting", listener.GetReachability())
	}
	if listener.GetSocketPath() != "/run/spacewave.sock" {
		t.Fatalf("listener socket = %q, want configured path", listener.GetSocketPath())
	}
}

func TestBuildDesktopRuntimeStateFromListenerDisconnected(t *testing.T) {
	// Project a listener with no available socket.
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{})

	// Verify the runtime projects the disconnected lifecycle and health.
	if state.GetStatusText() != "Disconnected" {
		t.Fatalf("status text = %q, want Disconnected", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED {
		t.Fatalf("health = %v, want disconnected", state.GetHealth())
	}
	if state.GetLifecycle() != desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED {
		t.Fatalf("lifecycle = %v, want disconnected", state.GetLifecycle())
	}

	// Verify the unavailable listener has no projected navigation or activity.
	listener := state.GetListener()
	if listener.GetReachability() != desktop_runtime.DesktopRuntimeReachability_DESKTOP_RUNTIME_REACHABILITY_UNREACHABLE {
		t.Fatalf("listener reachability = %v, want unreachable", listener.GetReachability())
	}
	if len(state.GetSessions()) != 0 || len(state.GetSpaces()) != 0 || len(state.GetActivity()) != 0 {
		t.Fatalf("bounded row lists must start empty")
	}
}

// TestBuildDesktopRuntimeStateWithoutListener checks a process that owns no
// listener, such as a hosted plugin, projects a running runtime instead of a
// disconnected listener.
func TestBuildDesktopRuntimeStateWithoutListener(t *testing.T) {
	// Project a runtime whose process owns no listener.
	state := BuildDesktopRuntimeState(nil, nil)

	// The runtime reads as running and healthy with no listener section.
	if state.GetStatusText() != "Running" {
		t.Fatalf("status text = %q, want Running", state.GetStatusText())
	}
	if state.GetHealth() != desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY {
		t.Fatalf("health = %v, want healthy", state.GetHealth())
	}
	if state.GetListener() != nil {
		t.Fatalf("listener = %v, want none", state.GetListener())
	}

	// The tray offers no socket path to copy.
	for _, entry := range BuildDesktopTrayEntriesFromRuntimeState(state) {
		if entry.GetId() == "action-copy-cli-socket" {
			t.Fatal("tray offers a socket path without a listener")
		}
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateIncludesNavigationRows(t *testing.T) {
	// Project a reachable runtime with one session and one space.
	state := BuildDesktopRuntimeState(&resource_listener.ListenerStatus{
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

	// The tray lists both rows and no empty placeholders.
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
	// Project two sessions where the second is active.
	state := BuildDesktopRuntimeState(&resource_listener.ListenerStatus{
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

	// The settings entry opens the active session's CLI settings.
	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	entry := findTrayEntryByID(entries, "settings")
	if entry == nil {
		t.Fatalf("expected settings tray entry")
	}
	if entry.GetAction().GetRoute() != "/u/2/settings/cli" {
		t.Fatalf("settings route = %q, want active session cli settings", entry.GetAction().GetRoute())
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateOpensAppForUpdate(t *testing.T) {
	// Project a reachable runtime with an update ready to install.
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	})
	state.Update = &desktop_runtime.DesktopRuntimeUpdateStatus{
		Ready:   true,
		Version: "1.2.3",
		Label:   "Update ready",
	}

	// Build the tray and verify the update action opens the app.
	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	entry := findTrayEntryByID(entries, "apply-update")
	if entry == nil {
		t.Fatalf("expected apply update tray entry")
	}
	if entry.GetKind() != desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_ACTION {
		t.Fatalf("kind = %v, want action", entry.GetKind())
	}
	if entry.GetAction().GetKind() != desktop_tray.DesktopTrayActionKind_DESKTOP_TRAY_ACTION_KIND_OPEN_ROUTE {
		t.Fatalf("action kind = %v, want open route", entry.GetAction().GetKind())
	}
	if !entry.GetEnabled() {
		t.Fatalf("enabled = false, want true")
	}
}

func TestBuildDesktopTrayEntriesFromRuntimeStateOrdersMenuSections(t *testing.T) {
	// Project a reachable runtime for the default tray menu.
	state := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{
		SocketPath: "/run/spacewave.sock",
		Listening:  true,
	})

	// Build the tray and define its opening navigation rows.
	entries := BuildDesktopTrayEntriesFromRuntimeState(state)
	want := []string{
		"Spacewave: Running",
		"",
		"Open Spacewave",
		"New Window",
	}

	// Verify tray navigation labels and order match the menu sequence.
	for idx, label := range want {
		if entries[idx].GetLabel() != label {
			t.Fatalf("entry %d label = %q, want %q", idx, entries[idx].GetLabel(), label)
		}
		if entries[idx].GetOrder() != int32(idx) {
			t.Fatalf("entry %d order = %d, want %d", idx, entries[idx].GetOrder(), idx)
		}
	}

	// Verify the CLI socket path stays out of visible tray labels.
	if hasTrayEntryLabel(entries, "/run/spacewave.sock") {
		t.Fatalf("did not expect socket path in visible tray labels")
	}
}

func TestBuildSessionProjectionSortsAndFlagsAuth(t *testing.T) {
	// Project an older ready session and a newer unauthenticated one.
	projection := BuildSessionProjection([]*SessionProjectionRow{
		{
			Entry: testSessionEntry(1, "spacewave", "acct-1"),
			Metadata: &session.SessionMetadata{
				DisplayName:         "old@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           1,
			},
			AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		},
		{
			Entry: testSessionEntry(2, "spacewave", "acct-2"),
			Metadata: &session.SessionMetadata{
				DisplayName:         "new@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           2,
			},
			AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED,
		},
	})

	// The newest session sorts first and raises one sign-in attention item.
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

	// The attention item marks a reachable runtime as needing attention.
	state := BuildDesktopRuntimeState(&resource_listener.ListenerStatus{
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
	// Project a ready Session whose spaces require its credential.
	projection := BuildSessionProjection([]*SessionProjectionRow{
		{
			Entry: testSessionEntry(7, "spacewave", "acct-7"),
			Metadata: &session.SessionMetadata{
				DisplayName:         "cloud@example.com",
				ProviderDisplayName: "Cloud",
				CreatedAt:           7,
			},
			AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
			SelfEnrollment: &selfenrollmentprojection.Projection{
				Count:              2,
				CredentialRequired: true,
			},
		},
	})

	// Verify the Session reports the credential requirement.
	if len(projection.Sessions) != 1 {
		t.Fatalf("session rows = %d, want 1", len(projection.Sessions))
	}
	if projection.Sessions[0].GetStatusText() != "Unlock required" {
		t.Fatalf("session status = %q, want unlock status", projection.Sessions[0].GetStatusText())
	}

	// Verify the attention item identifies the spaces and their Session route.
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
		name              string
		projection        *selfenrollmentprojection.Projection
		wantStatus        string
		wantAttentionKind desktop_runtime.DesktopRuntimeAttentionKind
	}{
		{
			name: "failure",
			projection: &selfenrollmentprojection.Projection{
				Failures: []*selfenrollmentprojection.RunFailure{{SharedObjectID: "so-1"}},
			},
			wantStatus: "Space connection failed",
		},
		{
			name:       "running",
			projection: &selfenrollmentprojection.Projection{Running: true},
			wantStatus: "Connecting spaces",
		},
		{
			name: "skipped",
			projection: &selfenrollmentprojection.Projection{
				Count:   1,
				Skipped: true,
			},
			wantStatus: "Connection skipped",
		},
		{
			name:       "pending",
			projection: &selfenrollmentprojection.Projection{Count: 1},
			wantStatus: "Spaces pending",
		},
		{
			name: "waiting-for-step-up",
			projection: &selfenrollmentprojection.Projection{
				Count:              1,
				CredentialRequired: true,
			},
			wantStatus:        "Unlock required",
			wantAttentionKind: desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_STEP_UP_REQUIRED,
		},
		{
			name:       "ready",
			projection: &selfenrollmentprojection.Projection{},
			wantStatus: "Ready",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a ready Session with the case's shared enrollment projection.
			row := &SessionProjectionRow{
				Entry:          testSessionEntry(1, "spacewave", "acct-1"),
				Metadata:       &session.SessionMetadata{},
				AccountStatus:  provider.ProviderAccountStatus_ProviderAccountStatus_READY,
				SelfEnrollment: tt.projection,
			}

			// Project the Session and verify its enrollment status text.
			projection := BuildSessionProjection([]*SessionProjectionRow{row})
			if len(projection.Sessions) != 1 {
				t.Fatalf("session rows = %d, want 1", len(projection.Sessions))
			}
			if got := projection.Sessions[0].GetStatusText(); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}

			// Verify enrollment states without a credential requirement add no attention.
			if tt.wantAttentionKind == desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_UNSPECIFIED {
				if len(projection.AttentionItems) != 0 {
					t.Fatalf("attention items = %d, want 0", len(projection.AttentionItems))
				}
				return
			}

			// Verify credential-required enrollment adds the expected attention kind.
			if len(projection.AttentionItems) != 1 {
				t.Fatalf("attention items = %d, want 1", len(projection.AttentionItems))
			}
			if got := projection.AttentionItems[0].GetKind(); got != tt.wantAttentionKind {
				t.Fatalf("attention kind = %v, want %v", got, tt.wantAttentionKind)
			}
		})
	}
}

func TestBuildDesktopRuntimeStateMarksRunningActivity(t *testing.T) {
	state := BuildDesktopRuntimeState(&resource_listener.ListenerStatus{
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
	rows := make([]*SessionProjectionRow, 0, maxProjectedSessions+1)
	for i := uint32(1); i <= maxProjectedSessions+1; i++ {
		rows = append(rows, &SessionProjectionRow{
			Entry:         testSessionEntry(i, "local", "local"),
			Metadata:      &session.SessionMetadata{CreatedAt: int64(i)},
			AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		})
	}
	projection := BuildSessionProjection(rows)
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
