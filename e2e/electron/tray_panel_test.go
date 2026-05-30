//go:build !skip_e2e && !js

package electron

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TIER: nightly
func TestTrayPanelScreenshotEvidence(t *testing.T) {
	if os.Getenv("BLDR_ELECTRON_DESKTOP_TRAY_POPOVER") != "1" {
		t.Skip("set BLDR_ELECTRON_DESKTOP_TRAY_POPOVER=1 before harness boot")
	}
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		resetTrayPanelFixtures(t, h)
	})

	cases := []trayPanelScreenshotCase{
		{
			name:       "normal-light",
			theme:      "light",
			statusText: "Running",
			state:      normalTrayPanelState(),
			contains: []string{
				"Running",
				"Healthy",
				"Project Alpha",
			},
			excludes: []string{
				"Syncing a very long status label",
				"Sign in required",
				"Disconnected",
			},
		},
		{
			name:       "attention-light",
			theme:      "light",
			statusText: "Needs attention",
			state:      attentionTrayPanelState(),
			contains: []string{
				"Needs attention",
				"Sign in required",
				"Install Update",
			},
			excludes: []string{
				"Disconnected",
				"Syncing a very long status label",
			},
		},
		{
			name:       "disconnected-light",
			theme:      "light",
			statusText: "Disconnected",
			state:      disconnectedTrayPanelState(),
			contains: []string{
				"Disconnected",
				"Offline",
				"CLI unavailable",
			},
			excludes: []string{
				"Sign in required",
				"Syncing a very long status label",
			},
		},
		{
			name:       "long-label-light",
			theme:      "light",
			statusText: "Syncing a very long status label",
			state:      longLabelTrayPanelState(),
			contains: []string{
				"Syncing a very long status label",
				"coolguy@spacewave.app with a long hosted session label",
				"Project Alpha With An Extremely Long Space Name",
			},
			excludes: []string{
				"Disconnected",
				"Sign in required",
			},
		},
		{
			name:       "normal-dark",
			theme:      "dark",
			statusText: "Running",
			state:      normalTrayPanelState(),
			contains: []string{
				"Running",
				"Healthy",
				"Project Alpha",
			},
			excludes: []string{
				"Syncing a very long status label",
				"Sign in required",
				"Disconnected",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			captureTrayPanelScreenshotCase(t, ctx, h, tc)
		})
	}
}

type trayPanelScreenshotCase struct {
	name       string
	theme      string
	statusText string
	state      map[string]any
	contains   []string
	excludes   []string
}

func captureTrayPanelScreenshotCase(
	t *testing.T,
	ctx context.Context,
	h *Harness,
	tc trayPanelScreenshotCase,
) {
	t.Helper()
	if err := h.SetNativeThemeSource(ctx, tc.theme); err != nil {
		t.Fatal(err)
	}
	if err := h.SetDesktopState(ctx, tc.state); err != nil {
		t.Fatal(err)
	}
	if _, err := waitForDesktopTrayState(ctx, h, func(state *desktopTrayStateSnapshot) bool {
		return state.StatusText == tc.statusText
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.OpenTrayPopover(ctx); err != nil {
		t.Fatal(err)
	}
	inspection, err := h.InspectTrayPopover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Theme != tc.theme {
		t.Fatalf("tray popover theme = %q, want %q", inspection.Theme, tc.theme)
	}
	for _, value := range tc.contains {
		if !strings.Contains(inspection.Text, value) {
			t.Fatalf("tray popover text does not contain %q:\n%s", value, inspection.Text)
		}
	}
	for _, value := range tc.excludes {
		if strings.Contains(inspection.Text, value) {
			t.Fatalf("tray popover text unexpectedly contains %q:\n%s", value, inspection.Text)
		}
	}
	path, err := h.CaptureTrayPopoverScreenshot(
		ctx,
		"tray-panel-"+strings.ReplaceAll(tc.name, "/", "-")+".png",
	)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatalf("empty tray panel screenshot: %s", path)
	}
	t.Logf("tray panel screenshot: %s", path)
}

func resetTrayPanelFixtures(t *testing.T, h *Harness) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.ResetDesktopState(ctx); err != nil {
		t.Errorf("reset desktop tray fixture state: %v", err)
	}
	if err := h.SetNativeThemeSource(ctx, "system"); err != nil {
		t.Errorf("reset native theme source: %v", err)
	}
}

func normalTrayPanelState() map[string]any {
	return map[string]any{
		"statusText": "Running",
		"health":     2,
		"listener": map[string]any{
			"label":      "CLI reachable",
			"detail":     "Local daemon is accepting commands",
			"socketPath": "/tmp/spacewave-e2e.sock",
		},
		"sessions": []map[string]any{
			{
				"label":      "coolguy@spacewave.app",
				"detail":     "Cloud session",
				"statusText": "Ready",
				"route":      "/u/coolguy/",
				"active":     true,
			},
		},
		"spaces": []map[string]any{
			{
				"label":      "Project Alpha",
				"detail":     "Shared Space",
				"statusText": "Active",
				"route":      "/u/coolguy/so/project-alpha",
				"active":     true,
			},
		},
		"activity": []map[string]any{
			{
				"label":  "Synced workspace index",
				"detail": "2 changes just now",
			},
		},
	}
}

func attentionTrayPanelState() map[string]any {
	return map[string]any{
		"statusText": "Needs attention",
		"health":     4,
		"attentionItems": []map[string]any{
			{
				"label":    "Sign in required",
				"detail":   "coolguy@spacewave.app needs a refreshed session",
				"severity": 3,
			},
		},
		"update": map[string]any{
			"ready":   true,
			"version": "1.2.3",
			"label":   "Install update",
			"detail":  "Version 1.2.3 is ready",
		},
	}
}

func disconnectedTrayPanelState() map[string]any {
	return map[string]any{
		"statusText": "Disconnected",
		"health":     5,
		"listener": map[string]any{
			"label":  "CLI unavailable",
			"detail": "Daemon socket is not reachable",
		},
	}
}

func longLabelTrayPanelState() map[string]any {
	state := normalTrayPanelState()
	state["statusText"] = "Syncing a very long status label"
	state["health"] = 3
	state["sessions"] = []map[string]any{
		{
			"label":      "coolguy@spacewave.app with a long hosted session label that must truncate",
			"detail":     "Remote session with a verbose detail string",
			"statusText": "Synchronizing a long status chip",
			"route":      "/u/coolguy/",
			"active":     true,
		},
	}
	state["spaces"] = []map[string]any{
		{
			"label":      "Project Alpha With An Extremely Long Space Name That Must Stay Inside The Panel",
			"detail":     "Shared with a very large project team",
			"statusText": "Active",
			"route":      "/u/coolguy/so/project-alpha",
			"active":     true,
		},
	}
	return state
}
