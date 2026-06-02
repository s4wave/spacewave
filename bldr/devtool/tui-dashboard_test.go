//go:build !js

package devtool

import (
	"strings"
	"testing"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestBuildDevtoolTUIDashboardIncludesDashboardSections(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "serving web runtime",
			LogFile: "/tmp/bldr.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ID:            "fetch:app",
			ManifestID:    "app",
			PlatformID:    "browser/wasm",
			State:         devtool_status.BldrDevtoolManifestStateRunning,
			ReadyRefCount: 1,
			Summary:       "fetching manifest",
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ID:           "build:app",
			BuildTargets: "web",
			ManifestID:   "app",
			PlatformID:   "browser/wasm",
			BuildType:    "dev",
			State:        devtool_status.BldrDevtoolManifestStateReady,
			HotRebuild:   true,
			Summary:      "built entrypoint",
		}},
		[]devtool_status.BldrDevtoolPluginRow{{
			ID:          "plugin:web/default",
			PluginID:    "web",
			InstanceKey: "default",
			State:       devtool_status.BldrDevtoolPluginStateRunning,
			Summary:     "plugin running",
		}},
		[]devtool_status.BldrDevtoolControllerRow{{
			ID:           "controller:exec:web",
			ControllerID: "web-controller",
			Kind:         "exec",
			State:        devtool_status.BldrDevtoolControllerStateRunning,
			Summary:      "directive attached",
		}},
		[]devtool_status.BldrDevtoolAttentionRow{{
			ID:       "attention:startup",
			Source:   "startup",
			Message:  "port already in use",
			Severity: devtool_status.BldrDevtoolAttentionSeverityWarning,
		}},
	)

	text := BuildDevtoolTUIDashboard(snapshot)
	for _, want := range []string{
		"DEVTOOL",
		"command    start web",
		"state      RUN",
		"work       4 active",
		"manifest   fetched 0/1   built 1/1",
		"plugins    ok 1   err 0",
		"attention  1",
		"ACTIVITY",
		"CONTROLS",
		"port already in use",
		"RUN    command    start web",
		"RUN    fetch      app",
		"RUN    plugin     web/default",
		"ctrl-c     stop",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing %q:\n%s", want, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardPolishesStatusDetails(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "build",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "building targets",
			LogFile: "/Users/cjs/repos/spacewave-bldr-tui/.bldr/logs/20260508.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ID:         "fetch:zeta",
			ManifestID: "zeta",
			PlatformID: "browser",
			State:      devtool_status.BldrDevtoolManifestStateRunning,
		}, {
			ID:         "fetch:alpha",
			ManifestID: "alpha",
			PlatformID: "native",
			State:      devtool_status.BldrDevtoolManifestStateQueued,
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ID:                      "build:zeta",
			BuildTargets:            "zeta",
			ManifestID:              "zeta",
			PlatformID:              "browser",
			BuildType:               "dev",
			State:                   devtool_status.BldrDevtoolManifestStateRunning,
			FullRebuild:             true,
			WatchedFileCount:        7,
			DependencyRebuildReason: "manifest dependency changed: core",
			Summary:                 "full rebuild",
		}, {
			ID:           "build:alpha",
			BuildTargets: "alpha",
			ManifestID:   "alpha",
			PlatformID:   "native",
			BuildType:    "dev",
			State:        devtool_status.BldrDevtoolManifestStateReady,
			CacheHit:     true,
			Summary:      "cache hit",
		}},
		[]devtool_status.BldrDevtoolPluginRow{{
			ID:          "plugin:zeta/right",
			PluginID:    "zeta",
			InstanceKey: "right",
			State:       devtool_status.BldrDevtoolPluginStateRunning,
			Summary:     "plugin running",
		}, {
			ID:          "plugin:alpha/left",
			PluginID:    "alpha",
			InstanceKey: "left",
			State:       devtool_status.BldrDevtoolPluginStateErrored,
			Error:       "download plugin manifest: copy failed",
			LastErrorAt: "2026-05-08T14:15:16Z",
		}},
		[]devtool_status.BldrDevtoolControllerRow{{
			ID:           "controller:exec:zeta",
			ControllerID: "zeta-controller",
			Kind:         "exec",
			State:        devtool_status.BldrDevtoolControllerStateRunning,
			Summary:      "directive attached",
		}, {
			ID:           "controller:load:alpha",
			ControllerID: "alpha-controller",
			Kind:         "load",
			State:        devtool_status.BldrDevtoolControllerStateError,
			Error:        "controller load failed",
		}},
		[]devtool_status.BldrDevtoolAttentionRow{{
			ID:       "attention:zeta",
			Source:   "zeta",
			Message:  "slow rebuild",
			Severity: devtool_status.BldrDevtoolAttentionSeverityWarning,
		}, {
			ID:       "attention:alpha",
			Source:   "alpha",
			Message:  "plugin needs attention",
			Severity: devtool_status.BldrDevtoolAttentionSeverityError,
		}},
	)

	text := BuildDevtoolTUIDashboard(snapshot)
	for _, want := range []string{
		"work       6 active",
		"manifest   fetched 0/2   built 1/2",
		"plugins    ok 1   err 1",
		"attention  2",
		"logs       .bldr/logs/20260508.log",
		"download plugin manifest: copy",
		"controller load failed",
		"plugin needs attention",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing polished detail %q:\n%s", want, text)
		}
	}

	assertTextOrder(t, text, "download plugin manifest: copy", "controller load failed")
}

func TestBuildDevtoolTUIDashboardRendersWorkStatus(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:  "build",
			State: devtool_status.BldrDevtoolCommandStateRunning,
		},
		nil,
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ID:           "build:web",
			BuildTargets: "web",
			State:        devtool_status.BldrDevtoolManifestStateRunning,
		}},
		nil,
		nil,
		nil,
	)

	first := buildDevtoolTUIDashboard(snapshot, 0)
	second := buildDevtoolTUIDashboard(snapshot, 3)
	text := first + second
	for _, want := range []string{
		"command    build",
		"state      RUN",
		"work       2 active",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing work status %q:\n%s", want, text)
		}
	}

	done := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:  "build",
			State: devtool_status.BldrDevtoolCommandStateDone,
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	doneText := buildDevtoolTUIDashboard(done, 3)
	if !strings.Contains(doneText, "work       done") {
		t.Fatalf("dashboard text missing done status:\n%s", doneText)
	}
}

func TestBuildDevtoolTUIDashboardAlignsRowsForFastScan(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   devtool_status.BldrDevtoolCommandStateStarting,
			Summary: "initializing web runtime",
			LogFile: ".bldr/logs/20260531-035147.log",
		},
		nil,
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ID:           "build:web",
			BuildTargets: "web",
			State:        devtool_status.BldrDevtoolManifestStateRunning,
			FullRebuild:  true,
		}, {
			ID:           "build:app",
			BuildTargets: "app",
			State:        devtool_status.BldrDevtoolManifestStateReady,
		}, {
			ID:           "build:core",
			BuildTargets: "core",
			State:        devtool_status.BldrDevtoolManifestStateReady,
		}},
		nil,
		[]devtool_status.BldrDevtoolControllerRow{{
			ID:           "controller:bldr/manifest/builder/controller",
			ControllerID: "bldr/manifest/builder/controller",
			State:        devtool_status.BldrDevtoolControllerStateRunning,
			Summary:      "1 controller value",
		}, {
			ID:           "controller:bldr/plugin/compiler/go",
			ControllerID: "bldr/plugin/compiler/go",
			State:        devtool_status.BldrDevtoolControllerStateRunning,
			Summary:      "1 controller value",
		}},
		nil,
	)

	text := BuildDevtoolTUIDashboard(snapshot)
	for _, want := range []string{
		"command    start web",
		"state      WAIT",
		"manifest   fetched 0/0   built 2/3",
		"summary    initializing web runtime",
		"RUN    build      web",
		"RUN    controller bldr/manifest/builder",
		"logs       .bldr/logs/20260531-035147.log",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing aligned row %q:\n%s", want, text)
		}
	}
	for line := range strings.SplitSeq(strings.TrimRight(text, "\n"), "\n") {
		if len([]rune(line)) > 80 {
			t.Fatalf("dashboard line too wide (%d): %q\n%s", len([]rune(line)), line, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardNormalizesBrowserURL(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "web runtime active on 127.0.0.1:5593",
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	text := BuildDevtoolTUIDashboard(snapshot)
	for _, want := range []string{
		"summary    web runtime active on http://127.0.0.1:5593",
		"RUN    command    start web                web runtime active on http://127...",
		"o          open http://127.0.0.1:5593",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing normalized URL %q:\n%s", want, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardRendersLiveSections(t *testing.T) {
	snapshot := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start desktop",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "desktop runtime active",
			LogFile: "/Users/cjs/repos/spacewave-bldr-tui/.bldr/logs/20260508.log",
		},
		[]devtool_status.BldrDevtoolManifestFetchRow{{
			ID:         "fetch:web",
			ManifestID: "web",
			PlatformID: "desktop/darwin/arm64",
			State:      devtool_status.BldrDevtoolManifestStateReady,
			Summary:    "ready",
		}},
		[]devtool_status.BldrDevtoolManifestBuildRow{{
			ID:           "build:web",
			BuildTargets: "web",
			PlatformID:   "desktop/darwin/arm64",
			BuildType:    "dev",
			State:        devtool_status.BldrDevtoolManifestStateReady,
			CacheHit:     true,
			Summary:      "cache hit",
		}},
		[]devtool_status.BldrDevtoolPluginRow{{
			ID:          "plugin:web/default",
			PluginID:    "web",
			InstanceKey: "default",
			State:       devtool_status.BldrDevtoolPluginStateRunning,
			Summary:     "plugin running",
		}},
		[]devtool_status.BldrDevtoolControllerRow{{
			ID:           "controller:exec:web",
			ControllerID: "web-controller",
			Kind:         "exec",
			State:        devtool_status.BldrDevtoolControllerStateError,
			Error:        "controller failed",
		}},
		nil,
	)

	text := BuildDevtoolTUIDashboard(snapshot)
	for _, want := range []string{
		"DEVTOOL",
		"ACTIVITY",
		"web-controller",
		"controller failed",
		"logs       .bldr/logs/20260508.log",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered dashboard missing %q:\n%s", want, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardShowsEmptyStates(t *testing.T) {
	text := BuildDevtoolTUIDashboard(nil)
	for _, want := range []string{
		"state      ?",
		"work       idle",
		"manifest   fetched 0/0   built 0/0",
		"plugins    ok 0   err 0",
		"attention  0",
		"clean - waiting for work",
		"ctrl-c     stop",
		".bldr/logs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing empty state %q:\n%s", want, text)
		}
	}
}

func assertTextOrder(t *testing.T, text, first, second string) {
	t.Helper()
	firstIdx := strings.Index(text, first)
	secondIdx := strings.Index(text, second)
	if firstIdx < 0 || secondIdx < 0 {
		t.Fatalf("expected text to contain %q and %q:\n%s", first, second, text)
	}
	if firstIdx > secondIdx {
		t.Fatalf("expected %q before %q:\n%s", first, second, text)
	}
}
