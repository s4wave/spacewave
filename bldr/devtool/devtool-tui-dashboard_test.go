//go:build !js

package devtool

import (
	"strconv"
	"strings"
	"testing"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// TestRenderDevtoolTUIDashboardHeaderAndSectionsInOrder keeps the command and serving address above build detail.
func TestRenderDevtoolTUIDashboardHeaderAndSectionsInOrder(t *testing.T) {
	// Render the dashboard from a healthy running command snapshot.
	dashboard := renderDevtoolTUIDashboard(representativeDevtoolTUIStatus(), "http://127.0.0.1:8080", 100, false)

	// Check that the command header and section order stay intact.
	assertContains(t, dashboard, "Bldr · dev")
	assertContains(t, dashboard, "RUNNING")
	assertContains(t, dashboard, "serving web app")
	assertSectionsInOrder(t, dashboard, []string{"SERVING", "TARGETS", "RUNTIME"})

	// A healthy run has no failures section and never shows a bottom error pile.
	assertNotContains(t, dashboard, "FAILURES")

	// Serving surfaces the live URL, not just the free-text command summary.
	assertContains(t, dashboard, "➜ http://127.0.0.1:8080")
	assertContains(t, dashboard, "o open browser")
}

// TestRenderDevtoolTUIDashboardSurfacesFailingTargetErrorText retains actionable errors and their log paths.
func TestRenderDevtoolTUIDashboardSurfacesFailingTargetErrorText(t *testing.T) {
	// Render the dashboard from a snapshot with build and plugin failures.
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

// TestRenderDevtoolTUIDashboardTargetsActiveFirst places failed and active work before ready targets.
func TestRenderDevtoolTUIDashboardTargetsActiveFirst(t *testing.T) {
	// Render the dashboard without a serving address.
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "", 100, false)

	// Check that failed, active, and ready targets appear in that order.
	failedIdx := strings.Index(dashboard, "spacewave-app")
	compilingIdx := strings.Index(dashboard, "spacewave-core")
	readyIdx := strings.Index(dashboard, "spacewave-web")
	if failedIdx >= compilingIdx || compilingIdx >= readyIdx {
		t.Fatalf("targets not ordered failed<active<ready in:\n%s", dashboard)
	}

	// Check that the active target's detail columns stay visible.
	assertContains(t, dashboard, "hot rebuild")
	assertContains(t, dashboard, "5 refs")
}

// TestRenderDevtoolTUIDashboardRuntimeCollapsesToCounts keeps controller internals out of the overview.
func TestRenderDevtoolTUIDashboardRuntimeCollapsesToCounts(t *testing.T) {
	// Render the dashboard from the failing snapshot.
	dashboard := renderDevtoolTUIDashboard(failingDashboardStatus(), "", 100, false)

	// Check that the runtime section collapses plugins and controllers to counts.
	assertContains(t, dashboard, "plugins")
	assertContains(t, dashboard, "2 running · 1 errored")
	assertContains(t, dashboard, "controllers")
	assertContains(t, dashboard, "2 running · 1 idle")

	// Controller internals are not enumerated row by row.
	assertNotContains(t, dashboard, "bldr/plugin-host")
}

// TestRenderDevtoolTUIDashboardRespectsWidthWithColor bounds styled failure output.
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

// TestRenderDevtoolTUIDashboardCapsManyTargets reports omitted targets without overflowing the table.
func TestRenderDevtoolTUIDashboardCapsManyTargets(t *testing.T) {
	// Build more ready target rows than the table can display.
	rows := make([]devtool_status.BldrDevtoolManifestBuildRow, 0, devtoolTUIMaxTargetRows+3)
	for idx := range devtoolTUIMaxTargetRows + 3 {
		rows = append(rows, devtool_status.BldrDevtoolManifestBuildRow{
			ManifestID: "m-" + string(rune('A'+idx)),
			PlatformID: "web/js/wasm",
			BuildType:  "dev",
			State:      devtool_status.BldrDevtoolManifestStateReady,
		})
	}

	// Assemble a running command snapshot carrying those rows.
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{Name: "dev", State: devtool_status.BldrDevtoolCommandStateRunning},
		nil, rows, nil, nil, nil,
	)

	// Render the dashboard and check the target count and omission notice.
	dashboard := renderDevtoolTUIDashboard(snapshot, "", 100, false)
	assertContains(t, dashboard, "TARGETS · "+strconv.Itoa(devtoolTUIMaxTargetRows+3)+"/"+strconv.Itoa(devtoolTUIMaxTargetRows+3)+" ready")
	assertContains(t, dashboard, "… 3 more targets")
}

// TestRenderDevtoolTUIDashboardHandlesNilSnapshot renders before the first status arrives.
func TestRenderDevtoolTUIDashboardHandlesNilSnapshot(t *testing.T) {
	// Render the dashboard before any status snapshot arrives.
	dashboard := renderDevtoolTUIDashboard(nil, "", 80, false)

	// Check that the placeholder keeps the quit hint and omits empty sections.
	assertContains(t, dashboard, "UNKNOWN")
	assertNotContains(t, dashboard, "FAILURES")
	assertNotContains(t, dashboard, "TARGETS")
	assertContains(t, dashboard, "ctrl-c quit")
}

// TestWrapTextCapsLinesAndKeepsSubstance bounds verbose errors.
func TestWrapTextCapsLinesAndKeepsSubstance(t *testing.T) {
	// Wrap a long error string at the test width.
	long := strings.Repeat("word ", 200)
	lines := wrapText(long, 40)

	// Check that the wrapped output stays within the line and width caps.
	if len(lines) > devtoolTUIMaxErrorLines {
		t.Fatalf("wrapText returned %d lines, want <= %d", len(lines), devtoolTUIMaxErrorLines)
	}
	for _, line := range lines {
		if visibleWidth(line) > 40 {
			t.Fatalf("wrapped line exceeds width: %q", line)
		}
	}
}

// representativeDevtoolTUIStatus provides mixed platforms and active runtime components.
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

// failingDashboardStatus provides command, build, and plugin failures together.
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

// assertSectionsInOrder checks the visible information hierarchy.
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

// assertContains checks that useful text remains visible.
func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

// assertNotContains checks that redundant or unavailable information stays hidden.
func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected output not to contain %q, got:\n%s", want, got)
	}
}

// TestDesktopDashboard keeps shared settings and live progress readable at terminal widths.
func TestDesktopDashboard(t *testing.T) {
	rows := []devtool_status.BldrDevtoolManifestBuildRow{
		{ManifestID: "web", PlatformID: "desktop/darwin/arm64", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateRunning, Summary: "full rebuild", FullRebuild: true},
		{ManifestID: "sample-core", PlatformID: "desktop/darwin/arm64", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateRunning, Summary: "full rebuild", FullRebuild: true},
		{ManifestID: "sample-web", PlatformID: "desktop/darwin/arm64", BuildType: "dev", State: devtool_status.BldrDevtoolManifestStateReady, Summary: "build complete", FullRebuild: true},
	}
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{Name: "start desktop", State: devtool_status.BldrDevtoolCommandStateStarting, Summary: "initializing desktop runtime"},
		nil, rows, nil, nil, nil,
	).WithProject(devtool_status.BldrDevtoolProjectStatus{WebStartupPath: "web/startup.tsx"})

	// Startup must not advertise a server or an unavailable keyboard action.
	for _, width := range []int{32, 40, 56, 80, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			// Render the dashboard at this width from the starting snapshot.
			dashboard := renderDevtoolTUIDashboard(snapshot, "", width, false)

			// Check that the startup summary stays readable without a server.
			assertContains(t, dashboard, "STARTING")
			assertContains(t, dashboard, "TARGETS · 1/3 ready")
			assertContains(t, dashboard, "building · full rebuild")
			assertNotContains(t, dashboard, "SERVING")
			assertNotContains(t, dashboard, "open browser")
			assertNotContains(t, dashboard, "full rebuild · full rebuild")
			if got := strings.Count(dashboard, "desktop/darwin/arm64 dev"); got != 1 {
				t.Fatalf("shared platform repeated %d times:\n%s", got, dashboard)
			}

			// Check that color-enabled output also respects the width.
			for _, color := range []bool{false, true} {
				for line := range strings.SplitSeq(renderDevtoolTUIDashboard(snapshot, "", width, color), "\n") {
					if visibleWidth(line) > width {
						t.Fatalf("line exceeds %d columns: %q", width, line)
					}
				}
			}

			// Log the widest tested dashboard for manual inspection.
			if width == 80 {
				t.Log("\n" + dashboard)
			}
		})
	}
}

// TestDashboardLongTargetKeepsStatus prevents identifiers from consuming the status column.
func TestDashboardLongTargetKeepsStatus(t *testing.T) {
	row := tuiTarget{manifest: strings.Repeat("long-target-", 8), detail: "build failed", kind: tuiStatusError, platform: "desktop/darwin/arm64", buildKit: "dev"}
	for _, width := range []int{40, 80} {
		line := targetLine(tuiTheme{}, row, 24, width)
		assertContains(t, line, "build failed")
		assertContains(t, line, "desktop/darwin/arm64 dev")
	}
}
