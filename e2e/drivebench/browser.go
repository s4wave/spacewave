//go:build !js

package drivebench

import (
	"strings"

	"github.com/aperturerobotics/fastjson"
)

// Browser carries the browser-side quickstart timing reported by the Drive
// viewer, independent of the Go-side wall clock.
type Browser struct {
	// ContentReadyMs is when the Drive viewer reported content ready.
	ContentReadyMs int `json:"contentReadyMs"`
	// QuickstartState is the last quickstart state the page published.
	QuickstartState string `json:"quickstartState"`
	// QuickstartProgressReadyMs is when quickstart progress became ready.
	QuickstartProgressReadyMs *int `json:"quickstartProgressReadyMs,omitempty"`
	// QuickstartContentReadyMs is when the quickstart content became ready.
	QuickstartContentReadyMs *int `json:"quickstartContentReadyMs,omitempty"`
	// QuickstartFinishedMs is when the quickstart finished.
	QuickstartFinishedMs *int `json:"quickstartFinishedMs,omitempty"`
	// QuickstartPhases lists each quickstart phase the page timed.
	QuickstartPhases []Phase `json:"quickstartPhases,omitempty"`
	// DriveSeedResourceCalls counts the Resource calls the Drive seed phases made.
	DriveSeedResourceCalls int `json:"driveSeedResourceCalls,omitempty"`
	// DriveSeedStartedMs is when the first Drive seed phase started.
	DriveSeedStartedMs *int `json:"driveSeedStartedMs,omitempty"`
	// DriveSeedFinishedMs is when the last Drive seed phase finished.
	DriveSeedFinishedMs *int `json:"driveSeedFinishedMs,omitempty"`
	// DriveSeedElapsedMs is the Drive seed window from first start to last finish.
	DriveSeedElapsedMs *int `json:"driveSeedElapsedMs,omitempty"`

	// StartupMarks lists the startup-boundary marks on the page timeline.
	StartupMarks []StartupMark `json:"startupMarks,omitempty"`
}

// BrowserFromQuickstartTiming builds the browser timing artifact from the
// quickstart timing object published by app/quickstart/create.ts.
func BrowserFromQuickstartTiming(contentReadyMs int, raw map[string]any) Browser {
	// Initialize the browser artifact when quickstart timing is unavailable.
	browser := Browser{ContentReadyMs: contentReadyMs}
	if raw == nil {
		return browser
	}

	// Recover the quickstart state and milestone timestamps.
	browser.QuickstartState, _ = raw["state"].(string)
	browser.QuickstartProgressReadyMs = optionalInt(raw, "progressReadyMs")
	browser.QuickstartContentReadyMs = optionalInt(raw, "contentReadyMs")
	browser.QuickstartFinishedMs = optionalInt(raw, "finishedMs")

	// Summarize the quickstart phases and Drive seed window.
	browser.QuickstartPhases = parsePhases(raw["phases"])
	browser.DriveSeedResourceCalls = CountDriveSeedResourceCalls(browser.QuickstartPhases)
	setDriveSeedWindow(&browser)
	return browser
}

// CountDriveSeedResourceCalls counts the timed Resource SDK calls that the
// Drive quickstart seed issues after populate-space begins and before content is
// ready. The populate-space wrapper is omitted because its leaf phases carry
// the actual RPC calls.
func CountDriveSeedResourceCalls(phases []Phase) int {
	count := 0
	for _, phase := range phases {
		if _, ok := driveSeedResourcePhases[phase.Name]; ok {
			count++
		}
	}
	return count
}

// driveSeedResourcePhases is the set of quickstart phase names whose Resource
// SDK calls make up the Drive seed.
var driveSeedResourcePhases = map[string]struct{}{
	"init-drive-batch":                 {},
	"write-drive-starter-guide-access": {},
	"write-drive-starter-guide-upload": {},
}

func setDriveSeedWindow(browser *Browser) {
	// Use the populate-space phase as the Drive seed window when available.
	for _, phase := range browser.QuickstartPhases {
		if phase.Name == "populate-space" {
			browser.DriveSeedStartedMs = &phase.StartedMs
			browser.DriveSeedFinishedMs = phase.FinishedMs
			browser.DriveSeedElapsedMs = phase.ElapsedMs
			return
		}
	}

	// Find the earliest and latest Drive seed phase timestamps.
	var start *int
	var finish *int
	for _, phase := range browser.QuickstartPhases {
		if !strings.HasPrefix(phase.Name, "init-drive-") &&
			!strings.HasPrefix(phase.Name, "write-drive-starter-guide-") {
			continue
		}
		started := phase.StartedMs
		if start == nil || started < *start {
			start = &started
		}
		if phase.FinishedMs != nil && (finish == nil || *phase.FinishedMs > *finish) {
			finished := *phase.FinishedMs
			finish = &finished
		}
	}

	// Publish the recovered Drive seed window and its elapsed duration.
	browser.DriveSeedStartedMs = start
	browser.DriveSeedFinishedMs = finish
	if start != nil && finish != nil {
		elapsed := *finish - *start
		browser.DriveSeedElapsedMs = &elapsed
	}
}

func marshalBrowserValue(arena *fastjson.Arena, browser Browser) *fastjson.Value {
	// Encode the browser's content and quickstart milestone timing.
	obj := arena.NewObject()
	obj.Set("contentReadyMs", arena.NewNumberInt(browser.ContentReadyMs))
	obj.Set("quickstartState", arena.NewString(browser.QuickstartState))
	setOptionalIntJSONField(arena, obj, "quickstartProgressReadyMs", browser.QuickstartProgressReadyMs)
	setOptionalIntJSONField(arena, obj, "quickstartContentReadyMs", browser.QuickstartContentReadyMs)
	setOptionalIntJSONField(arena, obj, "quickstartFinishedMs", browser.QuickstartFinishedMs)
	if len(browser.QuickstartPhases) != 0 {
		obj.Set("quickstartPhases", marshalPhasesValue(arena, browser.QuickstartPhases))
	}

	// Encode the browser's Drive seed call count and timing window.
	if browser.DriveSeedResourceCalls != 0 {
		obj.Set("driveSeedResourceCalls", arena.NewNumberInt(browser.DriveSeedResourceCalls))
	}
	setOptionalIntJSONField(arena, obj, "driveSeedStartedMs", browser.DriveSeedStartedMs)
	setOptionalIntJSONField(arena, obj, "driveSeedFinishedMs", browser.DriveSeedFinishedMs)
	setOptionalIntJSONField(arena, obj, "driveSeedElapsedMs", browser.DriveSeedElapsedMs)

	// Include the browser's startup marks when captured.
	if len(browser.StartupMarks) != 0 {
		obj.Set("startupMarks", marshalStartupMarksValue(arena, browser.StartupMarks))
	}
	return obj
}
