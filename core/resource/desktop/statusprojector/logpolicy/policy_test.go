package logpolicy

import (
	"testing"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name      string
		prev      *desktop_runtime.DesktopRuntimeState
		current   *desktop_runtime.DesktopRuntimeState
		changed   bool
		wantLevel Level
	}{
		{
			name:      "first publish",
			current:   testRuntimeState("Running"),
			changed:   true,
			wantLevel: LevelInfo,
		},
		{
			name:      "unchanged",
			prev:      testRuntimeState("Running"),
			current:   testRuntimeState("Running"),
			changed:   false,
			wantLevel: LevelDebug,
		},
		{
			name: "routine row churn",
			prev: testRuntimeStateWithSession("old@example.com"),
			current: func() *desktop_runtime.DesktopRuntimeState {
				state := testRuntimeStateWithSession("new@example.com")
				state.Activity = []*desktop_runtime.DesktopRuntimeActivityItem{{
					Id:    "activity-1",
					Label: "Finished upload",
					State: desktop_runtime.DesktopRuntimeActivityState_DESKTOP_RUNTIME_ACTIVITY_STATE_DONE,
				}}
				return state
			}(),
			changed:   true,
			wantLevel: LevelDebug,
		},
		{
			name: "lifecycle transition",
			prev: &desktop_runtime.DesktopRuntimeState{
				StatusText: "Disconnected",
				Health:     desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_DISCONNECTED,
				Lifecycle:  desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_DISCONNECTED,
			},
			current:   testRuntimeState("Running"),
			changed:   true,
			wantLevel: LevelInfo,
		},
		{
			name: "attention transition",
			prev: testRuntimeStateWithAttention("Sign in required"),
			current: func() *desktop_runtime.DesktopRuntimeState {
				state := testRuntimeStateWithAttention("Unlock required")
				state.AttentionItems[0].Kind = desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_STEP_UP_REQUIRED
				return state
			}(),
			changed:   true,
			wantLevel: LevelInfo,
		},
		{
			name: "update transition",
			prev: testRuntimeState("Running"),
			current: func() *desktop_runtime.DesktopRuntimeState {
				state := testRuntimeState("Running")
				state.Update = &desktop_runtime.DesktopRuntimeUpdateStatus{
					Ready:   true,
					Version: "1.2.3",
					Label:   "Update ready",
				}
				return state
			}(),
			changed:   true,
			wantLevel: LevelInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := Classify(tt.prev, tt.current, tt.changed)
			if decision.Level != tt.wantLevel {
				t.Fatalf("level = %v, want %v", decision.Level, tt.wantLevel)
			}
		})
	}
}

func testRuntimeState(statusText string) *desktop_runtime.DesktopRuntimeState {
	return &desktop_runtime.DesktopRuntimeState{
		StatusText: statusText,
		Health:     desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_HEALTHY,
		Lifecycle:  desktop_runtime.DesktopRuntimeLifecycle_DESKTOP_RUNTIME_LIFECYCLE_RUNNING,
	}
}

func testRuntimeStateWithSession(label string) *desktop_runtime.DesktopRuntimeState {
	state := testRuntimeState("Running")
	state.Sessions = []*desktop_runtime.DesktopRuntimeNavigationItem{{
		Id:         "session-1",
		Label:      label,
		Route:      "/u/1/",
		StatusText: "Ready",
	}}
	return state
}

func testRuntimeStateWithAttention(label string) *desktop_runtime.DesktopRuntimeState {
	state := testRuntimeState("Needs attention")
	state.Health = desktop_runtime.DesktopRuntimeHealth_DESKTOP_RUNTIME_HEALTH_NEEDS_ATTENTION
	state.AttentionItems = []*desktop_runtime.DesktopRuntimeAttentionItem{{
		Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_AUTH_REQUIRED,
		Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_WARNING,
		Label:    label,
		Route:    "/u/1/",
	}}
	return state
}
