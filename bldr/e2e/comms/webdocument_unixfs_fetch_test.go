//go:build !skip_e2e && !js

package comms

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

const (
	webDocumentUnixFSFixturePath     = "fs/u/1/so/01kwd6qwtkjb3z1whtxys72s4s/-/files/-/what is this.mp4"
	webDocumentUnixFSFixtureBody     = "spacewave webdocument unixfs inline fixture\n"
	webDocumentJSPluginPath          = "b/pd/spacewave-web/plugin.mjs"
	webDocumentRouteFixtureTimeoutMs = 30000
	webDocumentJSPluginBody          = `export { default } from '/workers/goscript-webdocument-unixfs-plugin.js'
`
)

type webDocumentRouteFixtureVariant string

const (
	webDocumentRouteBaseline                      webDocumentRouteFixtureVariant = "baseline"
	webDocumentRouteDynamicRelay                  webDocumentRouteFixtureVariant = "dynamic-relay"
	webDocumentRouteReleaseGeneration             webDocumentRouteFixtureVariant = "release-generation"
	webDocumentRouteInFlightReload                webDocumentRouteFixtureVariant = "in-flight-reload"
	webDocumentRoutePluginHostReplacement         webDocumentRouteFixtureVariant = "plugin-host-replacement"
	webDocumentRouteServiceWorkerFetchRouteTiming webDocumentRouteFixtureVariant = "service-worker-fetch-route-timing"
	webDocumentRouteDeliberateWorkerReplacement   webDocumentRouteFixtureVariant = "deliberate-worker-replacement"
)

type webDocumentRouteFixtureTrace struct {
	results      map[string]any
	eventLines   []string
	failureLines []string
}

// TestGoScriptForegroundUnixFSFetchKeepsSpacewaveWebRuntimeRoute verifies that
// a foreground WebDocument keeps the spacewave-web GoScript/direct-JS runtime
// route alive across a same-origin UnixFS inline fetch.
func TestGoScriptForegroundUnixFSFetchKeepsSpacewaveWebRuntimeRoute(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	browsers := []string{"chromium", "firefox"}
	for _, browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			trace := runWebDocumentRouteFixture(t, browser, webDocumentRouteBaseline)
			assertWebDocumentRouteSurvived(t, trace, true)
			assertNoWebDocumentRouteEventContains(t, trace, "normal-close")
		})
	}
}

// TestGoScriptForegroundUnixFSFetchDynamicRelayKeepsRoute verifies that the
// foreground JS route survives when the UnixFS-looking request is served
// through the ServiceWorker runtime relay seam with delayed response headers.
func TestGoScriptForegroundUnixFSFetchDynamicRelayKeepsRoute(t *testing.T) {
	// Install the fixture assets and run the dynamic-relay variant.
	installWebDocumentRouteFixtureAssets(t)

	// Assert the relay fetch succeeded through the runtime route.
	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteDynamicRelay)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "dynamicRelayFetch", true)
	assertBoolResult(t, trace.results, "dynamicRelayUsed", true)
	t.Logf("dynamic relay events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

// TestGoScriptForegroundUnixFSFetchReleaseGenerationKeepsRoute
// verifies that caching a new release preserves an in-flight foreground fetch.
func TestGoScriptForegroundUnixFSFetchReleaseGenerationKeepsRoute(t *testing.T) {
	installWebDocumentRouteFixtureAssets(t)

	// Run the release-generation variant in each browser.
	for _, browser := range []string{"chromium", "webkit"} {
		t.Run(browser, func(t *testing.T) {
			// Run the release-generation fixture and fail on a failed pass.
			trace := runWebDocumentRouteFixture(t, browser, webDocumentRouteReleaseGeneration)
			results := trace.results
			if pass, ok := results["pass"].(bool); !ok || !pass {
				t.Fatalf("release-generation fixture failed: %v", results["detail"])
			}

			// Assert the release generation preserved the in-flight fetch.
			assertBoolResult(t, results, "releaseBroadcast", true)
			assertBoolResult(t, results, "reloadObserved", false)
			assertBoolResult(t, results, "reloadBeforeNormalClose", false)
			assertBoolResult(t, results, "reproduced", false)
			assertBoolResult(t, results, "restartSentinelStable", true)
			t.Logf("release-generation events: %s", strings.Join(webDocumentResultEventLog(results), " | "))
		})
	}
}

// TestGoScriptForegroundUnixFSFetchInFlightReloadZeroDocumentRace verifies that
// an in-flight plugin-side stream open waits across a transient zero-WebDocument
// reload window and resumes through the replacement WebDocument route.
func TestGoScriptForegroundUnixFSFetchInFlightReloadZeroDocumentRace(t *testing.T) {
	// Install the fixture assets and run the in-flight-reload variant.
	installWebDocumentRouteFixtureAssets(t)

	// Assert the in-flight open recovered through the replacement route.
	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteInFlightReload)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "zeroDocumentRace", true)
	assertBoolResult(t, trace.results, "replacementRoute", true)
	assertBoolResult(t, trace.results, "inFlightOpenRecovered", true)
	assertWebDocumentRouteEventContains(
		t,
		trace,
		"PluginWorker: plugin/spacewave-web: no WebDocument available, waiting for next WebDocument",
	)
	t.Logf("in-flight reload events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

// TestGoScriptForegroundUnixFSFetchPluginHostReplacementDuringFetch verifies
// that an explicit PluginHost worker replacement during a delayed fetch names
// the removal owner and re-establishes the spacewave-web route.
func TestGoScriptForegroundUnixFSFetchPluginHostReplacementDuringFetch(t *testing.T) {
	// Install the fixture assets and run the plugin-host-replacement variant.
	installWebDocumentRouteFixtureAssets(t)

	// Assert the replacement route recovered the in-flight fetch.
	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRoutePluginHostReplacement)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "pluginHostReplacement", true)
	assertBoolResult(t, trace.results, "replacementRoute", true)
	assertBoolResult(t, trace.results, "inFlightOpenRecovered", true)
	assertWebDocumentRouteEventContains(t, trace, "dynamic-relay-request")
	assertWebDocumentRouteEventContains(
		t,
		trace,
		"tracker-close-receipt owner=PluginHost.RemoveWebWorker",
	)
	t.Logf("plugin-host replacement events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

// TestGoScriptForegroundUnixFSFetchServiceWorkerRouteTiming records the
// ServiceWorker fetch-route timing relative to last-document removal.
func TestGoScriptForegroundUnixFSFetchServiceWorkerRouteTiming(t *testing.T) {
	// Install the fixture assets and run the route-timing variant.
	installWebDocumentRouteFixtureAssets(t)

	// Assert the route timing scenario recorded the removal receipt.
	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteServiceWorkerFetchRouteTiming)
	assertWebDocumentRouteSurvived(t, trace, true)
	assertBoolResult(t, trace.results, "serviceWorkerRouteTiming", true)
	assertBoolResult(t, trace.results, "replacementRoute", true)
	assertBoolResult(t, trace.results, "inFlightOpenRecovered", true)
	assertWebDocumentRouteEventContains(t, trace, "dynamic-relay-request")
	assertWebDocumentRouteEventContains(
		t,
		trace,
		"tracker-close-receipt owner=service-worker-fetch-route-timing webDocumentId=spacewave-web-foreground-doc remainingDocumentCount=0",
	)
	t.Logf("service-worker route timing events: %s", strings.Join(webDocumentResultEventLog(trace.results), " | "))
}

// TestGoScriptForegroundUnixFSFetchDeliberateWorkerReplacementFailsFast verifies
// that discarding the last WebDocument with a terminal close fails an orphaned
// in-flight runtime stream fast, surfacing a terminal client-closed error rather
// than a relay-rerouted retry that would hang waiting for a replacement route.
func TestGoScriptForegroundUnixFSFetchDeliberateWorkerReplacementFailsFast(t *testing.T) {
	// Install the fixture assets and run the deliberate-worker-replacement variant.
	installWebDocumentRouteFixtureAssets(t)

	// Assert the orphaned stream failed fast through a terminal error.
	trace := runWebDocumentRouteFixture(t, "chromium", webDocumentRouteDeliberateWorkerReplacement)
	results := trace.results
	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("deliberate worker replacement fixture failed: %v", results["detail"])
	}

	// Assert the orphaned stream failed fast through a terminal error.
	assertBoolResult(t, results, "deliberateWorkerReplacement", true)
	assertBoolResult(t, results, "orphanFailedFast", true)
	assertBoolResult(t, results, "orphanTerminalNotRerouted", true)
	assertNoWebDocumentRouteEventContains(
		t,
		trace,
		"PluginWorker: plugin/spacewave-web: no WebDocument available, exiting!",
	)
	if elapsed, ok := results["orphanElapsedMs"].(float64); ok {
		t.Logf("orphaned stream failed fast after %.1fms", elapsed)
	}
	t.Logf("deliberate worker replacement events: %s", strings.Join(webDocumentResultEventLog(results), " | "))
}

func installWebDocumentRouteFixtureAssets(t *testing.T) {
	t.Helper()
	writeWebDocumentRouteAsset(t, webDocumentUnixFSFixturePath, []byte(webDocumentUnixFSFixtureBody))
	writeWebDocumentRouteAsset(t, webDocumentJSPluginPath, []byte(webDocumentJSPluginBody))
}

func writeWebDocumentRouteAsset(t *testing.T, relPath string, body []byte) {
	t.Helper()
	path := filepath.Join(distDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture asset dir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture asset %s: %v", relPath, err)
	}
}

func assertWebDocumentRouteSurvived(t *testing.T, trace webDocumentRouteFixtureTrace, assertRestartSentinel bool) {
	// Collect the fixture's result object.
	t.Helper()
	results := trace.results

	// Fail unless the fixture passed.
	if pass, ok := results["pass"].(bool); !ok || !pass {
		t.Fatalf("WebDocument UnixFS route fixture failed: %v", results["detail"])
	}

	// Assert the runtime route completed every stage of the fetch.
	assertBoolResult(t, results, "workerReady", true)
	assertBoolResult(t, results, "startInfo", true)
	assertBoolResult(t, results, "pluginToHostStream", true)
	assertBoolResult(t, results, "preFetchStream", true)
	assertBoolResult(t, results, "fetchSuccess", true)
	assertBoolResult(t, results, "postFetchStream", true)
	if assertRestartSentinel {
		assertBoolResult(t, results, "restartSentinelStable", true)
	}

	// Fail on any reported runtime failure reason.
	if failureReason, _ := results["failureReason"].(string); failureReason != "" {
		t.Fatalf("unexpected runtime failure: %s", failureReason)
	}

	// Reject forbidden lifecycle events and browser failures.
	for _, line := range trace.eventLines {
		for _, forbidden := range webDocumentRouteForbiddenEvents() {
			if strings.Contains(line, forbidden) {
				t.Fatalf("forbidden lifecycle event observed: %s", line)
			}
		}
	}
	if len(trace.failureLines) > 0 {
		t.Fatalf("browser failures: %s", strings.Join(trace.failureLines, "; "))
	}

	// Log the fixture's detail line for the test output.
	t.Logf("detail: %s", results["detail"])
}

func runWebDocumentRouteFixture(
	t *testing.T,
	browserName string,
	variant webDocumentRouteFixtureVariant,
) webDocumentRouteFixtureTrace {
	// Attribute test failures to the caller.
	t.Helper()

	// Launch the browser, skipping unavailable platforms.
	bt := browserType(browserName)
	browser, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(true),
	})
	if err != nil {
		if shouldSkipBrowserLaunch(browserName, err) {
			t.Skipf("skip %s: %v", browserName, err)
		}
		t.Fatalf("launch %s: %v", browserName, err)
	}
	defer browser.Close()

	// Open a browser context and a page in it.
	ctx, err := browser.NewContext()
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	defer ctx.Close()
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}

	// Record each console line, collecting errors as failures.
	var eventMu sync.Mutex
	var eventLines []string
	var failureLines []string
	page.On("console", func(msg playwright.ConsoleMessage) {
		// Append the formatted console line and record error types.
		line := "[" + browserName + " console." + msg.Type() + "] " + msg.Text()
		t.Log(line)
		eventMu.Lock()
		eventLines = append(eventLines, line)
		if msg.Type() == "error" {
			failureLines = append(failureLines, "console.error: "+msg.Text())
		}
		eventMu.Unlock()
	})

	// Record each page error as an event and a failure.
	page.On("pageerror", func(err error) {
		// Append the formatted page-error line.
		line := "[" + browserName + " pageerror] " + err.Error()
		t.Log(line)
		eventMu.Lock()
		eventLines = append(eventLines, line)
		failureLines = append(failureLines, "pageerror: "+err.Error())
		eventMu.Unlock()
	})

	// Navigate the page to the fixture variant's URL.
	url := testServer.url + "/goscript-webdocument-unixfs-fetch.html"
	if variant != webDocumentRouteBaseline {
		url += "?variant=" + string(variant)
	}
	if _, err := page.Goto(url); err != nil {
		t.Fatalf("goto %s: %v", url, err)
	}

	// Wait for the fixture to report DONE in the log element.
	logSel := page.Locator("#log")
	if err := logSel.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(webDocumentRouteFixtureTimeoutMs),
	}); err != nil {
		t.Fatalf("wait for #log visible: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(logSel).ToContainText("DONE", playwright.LocatorAssertionsToContainTextOptions{
		Timeout: playwright.Float(webDocumentRouteFixtureTimeoutMs),
	}); err != nil {
		text, _ := logSel.TextContent()
		eventMu.Lock()
		failures := strings.Join(failureLines, "; ")
		eventMu.Unlock()
		if failures != "" {
			t.Fatalf("fixture did not complete (text=%q, browser failures=%s): %v", text, failures, err)
		}
		t.Fatalf("fixture did not complete (text=%q): %v", text, err)
	}

	// Read and decode the fixture's results object.
	results, err := page.Evaluate("window.__results")
	if err != nil {
		t.Fatalf("evaluate window.__results: %v", err)
	}
	resultsMap, ok := results.(map[string]any)
	if !ok {
		t.Fatalf("window.__results is not an object: %T", results)
	}

	// Snapshot the recorded event lines under the lock.
	eventMu.Lock()
	trace := webDocumentRouteFixtureTrace{
		results:      resultsMap,
		eventLines:   append([]string(nil), eventLines...),
		failureLines: append([]string(nil), failureLines...),
	}
	eventMu.Unlock()
	return trace
}

func webDocumentResultEventLog(results map[string]any) []string {
	// Collect the string entries of the event log array.
	raw, ok := results["eventLog"].([]any)
	if !ok {
		return nil
	}
	lines := make([]string, 0, len(raw))
	for _, value := range raw {
		line, ok := value.(string)
		if ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func webDocumentRouteForbiddenEvents() []string {
	return []string{
		"PluginWorker: plugin/spacewave-web: no WebDocument available, exiting!",
		"closed while waiting for WebDocument",
	}
}

func assertWebDocumentRouteEventContains(t *testing.T, trace webDocumentRouteFixtureTrace, expected string) {
	// Fail unless one event line contains the expected text.
	t.Helper()
	for _, line := range trace.eventLines {
		if strings.Contains(line, expected) {
			return
		}
	}
	t.Fatalf("expected lifecycle event containing %q", expected)
}

func assertNoWebDocumentRouteEventContains(t *testing.T, trace webDocumentRouteFixtureTrace, forbidden string) {
	// Fail if any event line contains the forbidden text.
	t.Helper()
	for _, line := range trace.eventLines {
		if strings.Contains(line, forbidden) {
			t.Fatalf("unexpected lifecycle event containing %q: %s", forbidden, line)
		}
	}
}
