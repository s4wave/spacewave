//go:build !js

package drivebench

import "testing"

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
				"name":       "write-drive-starter-guide-create",
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
