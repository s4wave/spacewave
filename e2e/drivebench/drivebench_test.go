//go:build !js

package drivebench

import (
	"os"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

func TestParseStartupMarksPreservesDetail(t *testing.T) {
	marks := ParseStartupMarks([]any{
		map[string]any{
			"label":    "runtime.opfs-bridge-ready",
			"sequence": float64(12),
			"startMs":  float64(34),
			"mode":     "dedicated-worker",
			"source":   "browser",
			"detail": map[string]any{
				"documentId": "doc-1",
				"runtimeId":  "runtime-1",
				"workerId":   "runtime-1",
				"mode":       "dedicated-worker",
				"enabled":    false,
			},
		},
	})

	if len(marks) != 1 {
		t.Fatalf("marks = %d, want 1", len(marks))
	}
	mark := marks[0]
	if mark.Label != "runtime.opfs-bridge-ready" || mark.Sequence != 12 || mark.StartMs != 34 {
		t.Fatalf("mark identity = %+v", mark)
	}
	if mark.Mode != "dedicated-worker" || mark.Source != "browser" {
		t.Fatalf("mark flattened fields = %+v", mark)
	}
	if mark.Detail == nil {
		t.Fatalf("mark detail is nil")
	}
	gotEnabled, ok := mark.Detail["enabled"].(bool)
	if !ok || gotEnabled {
		t.Fatalf("detail enabled = %v (present %t), want false", gotEnabled, ok)
	}
	if got, _ := mark.Detail["workerId"].(string); got != "runtime-1" {
		t.Fatalf("detail workerId = %q, want runtime-1", got)
	}
}

func TestWriteRunPreservesStartupDetail(t *testing.T) {
	driveSeedStartedMs := 11
	runPath, err := WriteRun(t.TempDir(), Run{
		Timestamp:    "2026-06-30T08:41:13Z",
		Compiler:     "goscript",
		BuildMode:    "unbundled",
		RuntimeState: "cold",
		Cell:         "cold-unbundled",
		Milestones: Milestones{
			LiveAppMs:       10,
			RouteAcceptedMs: 20,
			UnixfsVisibleMs: 30,
			ContentReadyMs:  40,
		},
		Browser: Browser{
			ContentReadyMs:     40,
			QuickstartState:    "content-ready",
			StartupMarks:       []StartupMark{startupMarkWithInactiveBridge()},
			DriveSeedStartedMs: &driveSeedStartedMs,
		},
		ResourceConnection: ResourceConn{
			DurationMs: 25,
			Attempts:   1,
		},
		Trace: &Trace{
			Bytes:            123,
			RuntimeTracePath: "runtime.trace",
			TracetoolPath:    "tracetool.txt",
			UserTasks:        2,
			Tasks: []Task{{
				Type:    "core/plugin-space/fetch-manifest/process-resolvers",
				Count:   1,
				TotalUs: 700,
				MaxUs:   700,
			}},
		},
	})
	if err != nil {
		t.Fatalf("write run: %v", err)
	}

	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse run: %v", err)
	}

	if got := string(value.GetStringBytes("compiler")); got != "goscript" {
		t.Fatalf("compiler = %q, want goscript", got)
	}
	startupMarks := value.Get("browser").GetArray("startupMarks")
	if len(startupMarks) != 1 {
		t.Fatalf("startup marks = %d, want 1", len(startupMarks))
	}
	detail := startupMarks[0].Get("detail")
	if detail.Get("enabled").Type() != fastjson.TypeFalse {
		t.Fatalf("detail enabled type = %v, want false", detail.Get("enabled").Type())
	}
	if got := string(detail.GetStringBytes("workerId")); got != "runtime-1" {
		t.Fatalf("detail workerId = %q, want runtime-1", got)
	}
	tasks := value.Get("trace").GetArray("tasks")
	if len(tasks) != 1 {
		t.Fatalf("trace tasks = %d, want 1", len(tasks))
	}
}

func TestBrowserFromQuickstartTimingCountsDriveSeedResourceCalls(t *testing.T) {
	finished := 250
	timing := map[string]any{
		"state":           "content-ready",
		"progressReadyMs": float64(260),
		"contentReadyMs":  float64(300),
		"finishedMs":      float64(300),
		"phases": []any{
			map[string]any{
				"name":       "create-space",
				"startedMs":  float64(10),
				"finishedMs": float64(90),
				"elapsedMs":  float64(80),
			},
			map[string]any{
				"name":       "populate-space",
				"startedMs":  float64(100),
				"finishedMs": float64(finished),
				"elapsedMs":  float64(150),
			},
			map[string]any{
				"name":       "init-drive-unixfs",
				"startedMs":  float64(101),
				"finishedMs": float64(130),
				"elapsedMs":  float64(29),
			},
			map[string]any{
				"name":       "init-drive-unixfs-new-transaction",
				"startedMs":  float64(102),
				"finishedMs": float64(105),
				"elapsedMs":  float64(3),
			},
			map[string]any{
				"name":       "write-drive-starter-guide-upload",
				"startedMs":  float64(140),
				"finishedMs": float64(145),
				"elapsedMs":  float64(5),
			},
			map[string]any{
				"name":       "create-drive-settings-get-object",
				"startedMs":  float64(220),
				"finishedMs": float64(225),
				"elapsedMs":  float64(5),
			},
			map[string]any{
				"name":       "create-drive-settings-commit",
				"startedMs":  float64(230),
				"finishedMs": float64(240),
				"elapsedMs":  float64(10),
			},
		},
	}

	browser := BrowserFromQuickstartTiming(300, timing)
	if browser.QuickstartState != "content-ready" {
		t.Fatalf("quickstart state = %q", browser.QuickstartState)
	}
	if got := browser.DriveSeedResourceCalls; got != 4 {
		t.Fatalf("drive seed resource calls = %d, want 4", got)
	}
	if browser.DriveSeedStartedMs == nil || *browser.DriveSeedStartedMs != 100 {
		t.Fatalf("drive seed started = %v, want 100", browser.DriveSeedStartedMs)
	}
	if browser.DriveSeedFinishedMs == nil || *browser.DriveSeedFinishedMs != finished {
		t.Fatalf("drive seed finished = %v, want %d", browser.DriveSeedFinishedMs, finished)
	}
	if browser.DriveSeedElapsedMs == nil || *browser.DriveSeedElapsedMs != 150 {
		t.Fatalf("drive seed elapsed = %v, want 150", browser.DriveSeedElapsedMs)
	}
}

func TestWriteRunPreservesOperationShapeAndProfile(t *testing.T) {
	runPath, err := WriteRun(t.TempDir(), Run{
		Timestamp:    "2026-07-01T03:11:00Z",
		Compiler:     "goscript",
		BuildMode:    "unbundled",
		RuntimeState: "cold",
		Cell:         "cold-unbundled",
		Milestones:   Milestones{ContentReadyMs: 40},
		Browser:      Browser{ContentReadyMs: 40},
		ResourceConnection: ResourceConn{
			Attempts: 1,
		},
		OperationShape: &OperationShape{
			Operations: []OperationSummary{{
				Name:     "block-write",
				Count:    2,
				TotalUs:  30,
				MaxUs:    20,
				LogCount: 1,
				Fields: []OperationField{{
					Name:    "write-shape.encoded_blocks",
					Samples: 1,
					Sum:     3,
					Max:     3,
					Last:    3,
				}},
			}},
		},
		BrowserProfile: &BrowserProfile{
			Captured:      true,
			ProfilePath:   "browser-js-profile.cpuprofile",
			CaptureWindow: "drive-open",
			Bytes:         42,
			Buckets: []ProfileBucket{{
				Name:    "goscript-runtime",
				Count:   2,
				SelfUs:  11,
				TotalUs: 17,
			}},
		},
	})
	if err != nil {
		t.Fatalf("write run: %v", err)
	}

	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse run: %v", err)
	}
	operation := value.Get("operationShape").GetArray("operations")[0]
	if got := string(operation.GetStringBytes("name")); got != "block-write" {
		t.Fatalf("operation name = %q, want block-write", got)
	}
	fields := operation.GetArray("fields")
	if len(fields) != 1 {
		t.Fatalf("operation fields = %d, want 1", len(fields))
	}
	if got := string(fields[0].GetStringBytes("name")); got != "write-shape.encoded_blocks" {
		t.Fatalf("field name = %q", got)
	}
	profile := value.Get("browserProfile")
	if profile.Get("captured").Type() != fastjson.TypeTrue {
		t.Fatalf("profile captured type = %v, want true", profile.Get("captured").Type())
	}
	if got := string(profile.GetStringBytes("profilePath")); got != "browser-js-profile.cpuprofile" {
		t.Fatalf("profile path = %q", got)
	}
}

func TestWriteRunKeepsOperationShapeAndProfileOptional(t *testing.T) {
	runPath, err := WriteRun(t.TempDir(), Run{
		Timestamp:          "2026-07-01T03:11:00Z",
		Compiler:           "goscript",
		BuildMode:          "unbundled",
		RuntimeState:       "warm",
		Cell:               "warm-unbundled",
		Milestones:         Milestones{ContentReadyMs: 40},
		Browser:            Browser{ContentReadyMs: 40},
		ResourceConnection: ResourceConn{Attempts: 1},
	})
	if err != nil {
		t.Fatalf("write run: %v", err)
	}

	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse run without operation fields: %v", err)
	}
	if got := value.Get("operationShape"); got != nil {
		t.Fatalf("operationShape = %v, want absent", got)
	}
	if got := value.Get("browserProfile"); got != nil {
		t.Fatalf("browserProfile = %v, want absent", got)
	}
}

func startupMarkWithInactiveBridge() StartupMark {
	return StartupMark{
		Label:    "runtime.opfs-bridge-ready",
		Sequence: 12,
		StartMs:  34,
		Mode:     "dedicated-worker",
		Source:   "browser",
		Detail: map[string]any{
			"documentId": "doc-1",
			"runtimeId":  "runtime-1",
			"workerId":   "runtime-1",
			"mode":       "dedicated-worker",
			"enabled":    false,
		},
	}
}
