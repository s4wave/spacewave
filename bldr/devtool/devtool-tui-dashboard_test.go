//go:build !js

package devtool

import (
	"strings"
	"testing"
	"unicode/utf8"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestRenderDevtoolTUIDashboardProjectsFixedRegionsAndCommandLog(t *testing.T) {
	snapshot := representativeDevtoolTUIStatus()

	dashboard := renderDevtoolTUIDashboard(snapshot, 120)
	regions := buildDevtoolTUIRegions(snapshot, 120)

	assertContains(t, dashboard, "Bldr Devtool - dev [running]")
	assertRegionHeadersInOrder(t, dashboard, []string{
		"[command]",
		"[manifest fetch]",
		"[manifest builds]",
		"[plugins]",
		"[controllers]",
		"[recent errors]",
	})
	assertContains(t, dashboard, "  dev running")
	assertContains(t, dashboard, "  serving web app")
	assertContains(t, dashboard, "  log .bldr/logs/devtool.log")
	assertNotContains(t, dashboard, "/tmp/project/.bldr/logs/devtool.log")
	assertFieldsEqual(t, mustDevtoolTUIRegion(t, regions, "manifest fetch").Lines[0], []string{"ready", "web", "js", "dev", "local-cache", "3", "ready", "refs"})
	assertFieldsEqual(t, mustDevtoolTUIRegion(t, regions, "manifest builds").Lines[0], []string{"running", "api", "linux/amd64", "release", "remote-builder", "compiling;", "hot", "rebuild"})
	assertFieldsEqual(t, mustDevtoolTUIRegion(t, regions, "plugins").Lines[0], []string{"running", "js-compiler", "instance-a"})
	assertFieldsEqual(t, mustDevtoolTUIRegion(t, regions, "controllers").Lines[0], []string{"idle", "controllerbus", "exec"})
}

func TestRenderDevtoolTUIDashboardTruncatesNarrowWidth(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "dev-command-with-a-very-long-name",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "serving a dashboard summary that is intentionally longer than the terminal width",
			LogFile: "/workspace/app/.bldr/logs/devtool-with-a-long-log-file-name.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ManifestID:    "manifest-with-a-very-long-identifier",
			PlatformID:    "web/js/wasm",
			BuildType:     "development-with-extra-context",
			RemoteID:      "remote-builder-with-a-long-name",
			State:         devtool_status.BldrDevtoolManifestStateRunning,
			ReadyRefCount: 12,
		}},
		nil,
		nil,
		nil,
		nil,
	)

	const width = 48
	dashboard := renderDevtoolTUIDashboard(snapshot, width)

	assertContains(t, dashboard, "…")
	for line := range strings.SplitSeq(strings.TrimSuffix(dashboard, "\n"), "\n") {
		if runes := utf8.RuneCountInString(line); runes > width {
			t.Fatalf("dashboard line exceeds width %d: got %d runes in %q", width, runes, line)
		}
	}
	assertContains(t, dashboard, "  running manifest-with-a-very-long-identifier")
}

func TestDevtoolTUIDashboardCompactsRegionsOverRowCap(t *testing.T) {
	rows := make([]devtool_status.BldrDevtoolPluginRow, 0, devtoolTUIMaxRows+2)
	for idx := range devtoolTUIMaxRows + 2 {
		rows = append(rows, devtool_status.BldrDevtoolPluginRow{
			PluginID: "plugin-" + string(rune('A'+idx)),
			State:    devtool_status.BldrDevtoolPluginStateRunning,
			Summary:  "ready",
		})
	}
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{Name: "dev", State: devtool_status.BldrDevtoolCommandStateRunning},
		nil,
		nil,
		rows,
		nil,
		nil,
	)

	plugins := mustDevtoolTUIRegion(t, buildDevtoolTUIRegions(snapshot, 100), "plugins")

	if got := len(plugins.Lines); got != devtoolTUIMaxRows {
		t.Fatalf("compacted plugin row count = %d, want %d", got, devtoolTUIMaxRows)
	}
	assertLineContains(t, plugins.Lines[0], "plugin-A")
	assertLineContains(t, plugins.Lines[devtoolTUIMaxRows-2], "plugin-E")
	if got, want := plugins.Lines[devtoolTUIMaxRows-1], "3 more rows"; got != want {
		t.Fatalf("compaction summary = %q, want %q", got, want)
	}
	for _, line := range plugins.Lines {
		if strings.Contains(line, "plugin-F") || strings.Contains(line, "plugin-G") || strings.Contains(line, "plugin-H") {
			t.Fatalf("compacted plugin region kept hidden row in %q", line)
		}
	}
}

func TestDevtoolTUIDashboardAggregatesRecentErrors(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:  "dev",
			State: devtool_status.BldrDevtoolCommandStateError,
			Error: "command failed",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ManifestID: "web",
			State:      devtool_status.BldrDevtoolManifestStateError,
			Error:      "fetch failed",
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ManifestID: "api",
			State:      devtool_status.BldrDevtoolManifestStateError,
			Error:      "build failed",
		}},
		[]devtool_status.BldrDevtoolPluginRow{{
			PluginID: "compiler",
			State:    devtool_status.BldrDevtoolPluginStateErrored,
			Error:    "plugin failed",
		}},
		[]devtool_status.BldrDevtoolControllerRow{{
			ControllerID: "controllerbus",
			State:        devtool_status.BldrDevtoolControllerStateError,
			Error:        "controller failed",
		}},
		[]devtool_status.BldrDevtoolAttentionRow{{
			Severity: devtool_status.BldrDevtoolAttentionSeverityError,
			Message:  "attention needed",
			Detail:   "restart required",
		}},
	)

	recentErrors := mustDevtoolTUIRegion(t, buildDevtoolTUIRegions(snapshot, 100), "recent errors")
	want := []string{
		"command: command failed",
		"web: fetch failed",
		"api: build failed",
		"compiler: plugin failed",
		"controllerbus: controller failed",
		"error: attention needed; restart required",
	}
	assertStringSlicesEqual(t, recentErrors.Lines, want)
}

func TestRenderDevtoolTUIDashboardHandlesNilSnapshot(t *testing.T) {
	dashboard := renderDevtoolTUIDashboard(nil, 80)

	if !strings.HasPrefix(dashboard, "Bldr Devtool\n") {
		t.Fatalf("nil snapshot title = %q", firstLine(dashboard))
	}
	assertRegionHeadersInOrder(t, dashboard, []string{
		"[command]",
		"[manifest fetch]",
		"[manifest builds]",
		"[plugins]",
		"[controllers]",
		"[recent errors]",
	})
	assertContains(t, dashboard, "  command unknown")
	if got, want := strings.Count(dashboard, "  "+devtoolTUIEmptyMessage), 5; got != want {
		t.Fatalf("nil snapshot empty region count = %d, want %d in:\n%s", got, want, dashboard)
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
			RemoteID:      "local-cache",
			State:         devtool_status.BldrDevtoolManifestStateReady,
			ReadyRefCount: 3,
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ManifestID: "api",
			PlatformID: "linux/amd64",
			BuildType:  "release",
			RemoteID:   "remote-builder",
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

func mustDevtoolTUIRegion(t *testing.T, regions []devtoolTUIRegion, title string) devtoolTUIRegion {
	t.Helper()
	for _, region := range regions {
		if region.Title == title {
			return region
		}
	}
	t.Fatalf("missing dashboard region %q in %#v", title, regions)
	return devtoolTUIRegion{}
}

func assertRegionHeadersInOrder(t *testing.T, dashboard string, headers []string) {
	t.Helper()
	last := -1
	for _, header := range headers {
		idx := strings.Index(dashboard, header)
		if idx < 0 {
			t.Fatalf("dashboard missing region header %q in:\n%s", header, dashboard)
		}
		if idx <= last {
			t.Fatalf("dashboard header %q appeared out of order in:\n%s", header, dashboard)
		}
		last = idx
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected %q not to contain %q", got, want)
	}
}

func assertLineContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("line = %q, want it to contain %q", got, want)
	}
}

func assertFieldsEqual(t *testing.T, got string, want []string) {
	t.Helper()
	gotFields := strings.Fields(got)
	assertStringSlicesEqual(t, gotFields, want)
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("line %d = %q, want %q; all lines %#v", idx, got[idx], want[idx], got)
		}
	}
}

func firstLine(value string) string {
	line, _, _ := strings.Cut(value, "\n")
	return line
}
