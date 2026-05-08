//go:build !js

package devtool

import (
	"strings"
	"testing"

	tui "github.com/grindlemire/go-tui"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestBuildDevtoolTUIDashboardElementIncludesDashboardPanes(t *testing.T) {
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

	text := collectDevtoolTUIText(BuildDevtoolTUIDashboardElement(snapshot))
	for _, want := range []string{
		"Command",
		"Manifest Fetch / Build",
		"Plugins",
		"Controllers",
		"Recent Errors / Attention",
		"Controls",
		"progress: [====>-------------] 4 active",
		"now: start web running",
		"health: running 1 | waiting 0 | errored 0 | unknown 0",
		"start web",
		"browser/wasm",
		"port already in use",
		"q: quit",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing %q:\n%s", want, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardElementPolishesStatusDetails(t *testing.T) {
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

	text := collectDevtoolTUIText(BuildDevtoolTUIDashboardElement(snapshot))
	for _, want := range []string{
		"progress: [====>-------------] 6 active",
		"now: build running | fetch alpha queued | fetch zeta running | build zeta running | +2 more",
		"log: .bldr/logs/20260508.log",
		"logs: .bldr/logs/20260508.log",
		"health: running 1 | waiting 0 | errored 1 | unknown 0",
		"manifest dependency changed: core",
		"7",
		"download plugin manifest: copy failed",
		"controller load failed",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing polished detail %q:\n%s", want, text)
		}
	}

	assertTextOrder(t, text, "alpha", "zeta")
	assertTextOrder(t, text, "plugin needs attention", "slow rebuild")
}

func TestBuildDevtoolTUIDashboardElementAnimatesProgressBar(t *testing.T) {
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

	first := collectDevtoolTUIText(buildDevtoolTUIDashboardElement(snapshot, 0))
	second := collectDevtoolTUIText(buildDevtoolTUIDashboardElement(snapshot, 3))
	text := first + second
	for _, want := range []string{
		"progress: [====>-------------] 2 active",
		"progress: [---====>----------] 2 active",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard text missing progress frame %q:\n%s", want, text)
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
	doneText := collectDevtoolTUIText(buildDevtoolTUIDashboardElement(done, 3))
	if !strings.Contains(doneText, "progress: [==================] done") {
		t.Fatalf("dashboard text missing done progress:\n%s", doneText)
	}
}

func TestBuildDevtoolTUIDashboardElementRendersLivePanes(t *testing.T) {
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

	el := BuildDevtoolTUIDashboardElement(snapshot)
	buf := tui.NewBuffer(140, 44)
	el.Calculate(140, 44)
	tui.RenderTree(buf, el)

	text := collectDevtoolTUIBufferText(buf)
	for _, want := range []string{
		"Manifest Fetch / Build",
		"Plugins / Controllers",
		"Recent Errors / Attention",
		"web-controller",
		"controller failed",
		"logs: .bldr/logs/20260508.log",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered dashboard missing %q:\n%s", want, text)
		}
	}
}

func TestBuildDevtoolTUIDashboardElementShowsEmptyStates(t *testing.T) {
	text := collectDevtoolTUIText(BuildDevtoolTUIDashboardElement(nil))
	for _, want := range []string{
		"no manifest fetches",
		"no manifest builds",
		"no plugins requested",
		"no controller activity",
		"no recent errors or attention",
		"progress: [------------------] idle",
		"now: no active work",
		"q: quit",
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

func collectDevtoolTUIText(el *tui.Element) string {
	var b strings.Builder
	var walk func(*tui.Element)
	walk = func(current *tui.Element) {
		if current == nil {
			return
		}
		if text := current.Text(); text != "" {
			b.WriteString(text)
			b.WriteByte('\n')
		}
		for _, child := range current.Children() {
			walk(child)
		}
	}
	walk(el)
	return b.String()
}

func collectDevtoolTUIBufferText(buf *tui.Buffer) string {
	var b strings.Builder
	for y := 0; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			r := buf.Cell(x, y).Rune
			if r == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(r)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
