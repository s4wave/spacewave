//go:build !js

package devtool

import (
	"strconv"
	"strings"
	"testing"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestRenderDevtoolTUIDashboardHeaderAndSectionsInOrder(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(representativeDevtoolTUIStatus(), "http://127.0.0.1:8080", 100, false)

	assertContains(t, dashboard, "Bldr Devtool · dev · RUNNING")
	assertContains(t, dashboard, "serving web app")
	assertSectionsInOrder(t, dashboard, []string{"SERVING", "TARGETS", "RUNTIME"})
	// A healthy run has no failures section and never shows a bottom error pile.
	assertNotContains(t, dashboard, "FAILURES")
	// Serving surfaces the live URL, not just the free-text command summary.
	assertContains(t, dashboard, "➜ http://127.0.0.1:8080")
	assertContains(t, dashboard, "press o to open browser")
}

func TestRenderDevtoolTUIDashboardSurfacesFailingTargetErrorText(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "http://127.0.0.1:8080", 100, false)

	// Falsifier: the failing build's full error text must be readable on screen.
	assertContains(t, dashboard, "undefined: RenderRoot")
	assertContains(t, dashboard, "did you mean RenderRootView?")
	assertContains(t, dashboard, "build spacewave-app · web/js/wasm dev")
	assertContains(t, dashboard, "worker exited: exit status 2")
	// Failures are surfaced above the target table, not buried at the bottom.
	assertSectionsInOrder(t, dashboard, []string{"FAILURES · 3", "TARGETS", "RUNTIME"})
	// The command log path is reachable from the failing surface.
	assertContains(t, dashboard, "log .bldr/logs/devtool.log")
	assertNotContains(t, dashboard, "/home/dev/spacewave/.bldr/logs/devtool.log")
}

func TestRenderDevtoolTUIDashboardTargetsActiveFirst(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "", 100, false)

	failedIdx := strings.Index(dashboard, "spacewave-app")
	compilingIdx := strings.Index(dashboard, "spacewave-core")
	readyIdx := strings.Index(dashboard, "spacewave-web")
	if failedIdx >= compilingIdx || compilingIdx >= readyIdx {
		t.Fatalf("targets not ordered failed<active<ready in:\n%s", dashboard)
	}
	assertContains(t, dashboard, "hot rebuild")
	assertContains(t, dashboard, "5 refs")
}

func TestRenderDevtoolTUIDashboardRuntimeCollapsesToCounts(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "", 100, false)

	assertContains(t, dashboard, "plugins")
	assertContains(t, dashboard, "2 running · 1 errored")
	assertContains(t, dashboard, "controllers")
	assertContains(t, dashboard, "2 running · 1 idle")
	// Controller internals are not enumerated row by row.
	assertNotContains(t, dashboard, "bldr/plugin-host")
}

func TestRenderDevtoolTUIDashboardRespectsWidthWithColor(t *testing.T) {
	const width = 56
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "http://127.0.0.1:8080", width, true)

	for line := range strings.SplitSeq(strings.TrimSuffix(dashboard, "\n"), "\n") {
		if got := visibleWidth(line); got > width {
			t.Fatalf("line exceeds width %d: got %d cells in %q", width, got, line)
		}
	}
	// Even color-enabled output keeps the failing error legible.
	assertContains(t, dashboard, "undefined: RenderRoot")
}

func TestRenderDevtoolTUIDashboardCapsManyTargets(t *testing.T) {
	rows := make([]devtool_status.BldrDevtoolManifestBuildRow, 0, devtoolTUIMaxTargetRows+3)
	for idx := range devtoolTUIMaxTargetRows + 3 {
		rows = append(rows, devtool_status.BldrDevtoolManifestBuildRow{
			ManifestID: "m-" + string(rune('A'+idx)),
			PlatformID: "web/js/wasm",
			BuildType:  "dev",
			State:      devtool_status.BldrDevtoolManifestStateReady,
		})
	}
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{Name: "dev", State: devtool_status.BldrDevtoolCommandStateRunning},
		nil, rows, nil, nil, nil,
	)

	dashboard := renderDevtoolTUIDashboard(snapshot, "", 100, false)
	assertContains(t, dashboard, "TARGETS · "+strconv.Itoa(devtoolTUIMaxTargetRows+3))
	assertContains(t, dashboard, "… 3 more targets")
}

func TestRenderDevtoolTUIDashboardHandlesNilSnapshot(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(nil, "", 80, false)

	assertContains(t, dashboard, "Bldr Devtool · UNKNOWN")
	assertNotContains(t, dashboard, "FAILURES")
	assertNotContains(t, dashboard, "TARGETS")
	assertContains(t, dashboard, "ctrl-c quit")
}

func TestWrapTextCapsLinesAndKeepsSubstance(t *testing.T) {
	long := strings.Repeat("word ", 200)
	lines := wrapText(long, 40)
	if len(lines) > devtoolTUIMaxErrorLines {
		t.Fatalf("wrapText returned %d lines, want <= %d", len(lines), devtoolTUIMaxErrorLines)
	}
	for _, line := range lines {
		if visibleWidth(line) > 40 {
			t.Fatalf("wrapped line exceeds width: %q", line)
		}
	}
}

func TestTerminalCellMeasurementAndTruncation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
		want  string
	}{
		{name: "narrow exact", value: "abcd", width: 4, want: "abcd"},
		{name: "narrow one cell short", value: "abcd", width: 3, want: "ab…"},
		{name: "wide exact", value: "界界", width: 4, want: "界界"},
		{name: "wide one cell short", value: "界界", width: 3, want: "界…"},
		{name: "combining exact", value: "e\u0301x", width: 2, want: "e\u0301x"},
		{name: "combining one cell short", value: "e\u0301x", width: 1, want: "…"},
		{name: "emoji variation selector exact", value: "❤️x", width: 3, want: "❤️x"},
		{name: "emoji variation selector one cell short", value: "❤️x", width: 2, want: "…"},
		{name: "emoji sequence exact", value: "👩‍💻x", width: 3, want: "👩‍💻x"},
		{name: "emoji sequence one cell short", value: "👩‍💻x", width: 2, want: "…"},
		{name: "ANSI styled exact", value: "\x1b[31m界界\x1b[0m", width: 4, want: "\x1b[31m界界\x1b[0m"},
		{name: "ANSI styled one cell short", value: "\x1b[31m界界\x1b[0m", width: 3, want: "\x1b[31m界…\x1b[0m"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := truncateDisplay(test.value, test.width)
			if got != test.want {
				t.Fatalf("truncateDisplay(%q, %d) = %q, want %q", test.value, test.width, got, test.want)
			}
			if gotWidth := visibleWidth(got); gotWidth > test.width {
				t.Fatalf("visible width = %d, want <= %d", gotWidth, test.width)
			}
		})
	}

	for _, test := range []struct {
		value string
		want  int
	}{
		{value: "界", want: 2},
		{value: "e\u0301", want: 1},
		{value: "👩‍💻", want: 2},
		{value: "\x1b[32m界\x1b[0m", want: 2},
	} {
		if got := visibleWidth(test.value); got != test.want {
			t.Errorf("visibleWidth(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestRenderDevtoolTUIDashboardRespectsTerminalCellWidth(t *testing.T) {
	const width = 40
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "界界界",
			State:   devtool_status.BldrDevtoolCommandStateError,
			Summary: strings.Repeat("界", width),
			Error:   strings.Repeat("e\u0301", width),
		},
		nil,
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ManifestID: strings.Repeat("👩‍💻", width),
			PlatformID: "web/js/wasm",
			BuildType:  "dev",
			State:      devtool_status.BldrDevtoolManifestStateError,
		}},
		nil,
		nil,
		nil,
	)
	dashboard := renderDevtoolTUIDashboard(snapshot, "https://界界界.example", width, true)
	for line := range strings.SplitSeq(strings.TrimSuffix(dashboard, "\n"), "\n") {
		if got := visibleWidth(line); got > width {
			t.Fatalf("line exceeds width %d: got %d cells in %q", width, got, line)
		}
	}
}

func TestRenderDevtoolTUIDashboardTargetsPreserveStableUrgencyOrder(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{},
		nil,
		[]devtool_status.BldrDevtoolManifestBuildRow{
			{ManifestID: "ready-first", State: devtool_status.BldrDevtoolManifestStateReady},
			{ManifestID: "error", State: devtool_status.BldrDevtoolManifestStateError},
			{ManifestID: "ready-second", State: devtool_status.BldrDevtoolManifestStateReady},
			{ManifestID: "active", State: devtool_status.BldrDevtoolManifestStateRunning},
		},
		nil,
		nil,
		nil,
	)
	dashboard := renderDevtoolTUIDashboard(snapshot, "", 100, false)

	previous := -1
	for _, manifestID := range []string{"error", "active", "ready-first", "ready-second"} {
		current := strings.Index(dashboard, manifestID)
		if current < 0 || current <= previous {
			t.Fatalf("target %q is out of stable urgency order in:\n%s", manifestID, dashboard)
		}
		previous = current
	}
}

func representativeDevtoolTUIStatus() *devtool_status.BldrDevtoolStatus {
	return devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "dev",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "serving web app",
			LogFile: "/tmp/project/.bldr/logs/devtool.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ManifestID:    "web",
			PlatformID:    "js",
			BuildType:     "dev",
			State:         devtool_status.BldrDevtoolManifestStateReady,
			ReadyRefCount: 3,
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ManifestID: "api",
			PlatformID: "linux/amd64",
			BuildType:  "release",
			State:      devtool_status.BldrDevtoolManifestStateRunning,
			Summary:    "compiling",
			HotRebuild: true,
		}},
		[]devtool_status.BldrDevtoolPluginRow{{
			PluginID:    "js-compiler",
			InstanceKey: "instance-a",
			State:       devtool_status.BldrDevtoolPluginStateRunning,
		}},
		[]devtool_status.BldrDevtoolControllerRow{{
			ControllerID: "controllerbus",
			Kind:         "exec",
			State:        devtool_status.BldrDevtoolControllerStateIdle,
		}},
		nil,
	)
}

func failingDashboardStatus() *devtool_status.BldrDevtoolStatus {
	return devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   devtool_status.BldrDevtoolCommandStateError,
			Summary: "web runtime active on 127.0.0.1:8080",
			Error:   "one target failed to build",
			LogFile: "/home/dev/spacewave/.bldr/logs/devtool.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{
			{ManifestID: "spacewave-web", PlatformID: "web/js/wasm", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateReady, ReadyRefCount: 5},
		},
		[]devtool_status.BldrDevtoolManifestBuildRow{
			{ID: "b1", ManifestID: "spacewave-core", PlatformID: "web/js/wasm", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateRunning, Summary: "compiling", HotRebuild: true, WatchedFileCount: 214},
			{ID: "b2", ManifestID: "spacewave-app", PlatformID: "web/js/wasm", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateError, Error: "./src/app/main.go:42:13: undefined: RenderRoot (did you mean RenderRootView?)"},
		},
		[]devtool_status.BldrDevtoolPluginRow{
			{PluginID: "spacewave-web", InstanceKey: "root", State: devtool_status.BldrDevtoolPluginStateRunning},
			{PluginID: "js-compiler", State: devtool_status.BldrDevtoolPluginStateRunning},
			{PluginID: "goscript-web", State: devtool_status.BldrDevtoolPluginStateErrored, Error: "worker exited: exit status 2", LastErrorAt: "12:04:51"},
		},
		[]devtool_status.BldrDevtoolControllerRow{
			{ControllerID: "controllerbus/exec", Kind: "exec", State: devtool_status.BldrDevtoolControllerStateRunning},
			{ControllerID: "bldr/plugin-host", Kind: "loader", State: devtool_status.BldrDevtoolControllerStateRunning},
			{ControllerID: "bldr/watch", Kind: "watch", State: devtool_status.BldrDevtoolControllerStateIdle},
		},
		nil,
	)
}

func assertSectionsInOrder(t *testing.T, dashboard string, sections []string) {
	t.Helper()
	last := -1
	for _, section := range sections {
		idx := strings.Index(dashboard, section)
		if idx < 0 {
			t.Fatalf("dashboard missing section %q in:\n%s", section, dashboard)
		}
		if idx <= last {
			t.Fatalf("dashboard section %q out of order in:\n%s", section, dashboard)
		}
		last = idx
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected output not to contain %q, got:\n%s", want, got)
	}
}
