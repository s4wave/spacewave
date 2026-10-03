//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

// TestLocalCDNCanvasReopen measures same-document Changelog-to-Canvas navigation
// after creating a local Canvas. Setup and Changelog dwell are outside each sample.
func TestLocalCDNCanvasReopen(t *testing.T) {
	// Require the isolated release fixture used by the cold Drive benchmark.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}
	page := testHarness.newPage(t)
	server := testHarness.server.Handler.(*localCDNHandler)
	dir := filepath.Join(testHarness.artifactDir, "canvas-reopen-"+testHarness.browserName+"-"+strconv.FormatInt(time.Now().UnixMilli(), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Logf("Canvas reopen artifacts: %s", dir)

	// Seed through the ordinary Canvas quickstart, then require its saved file node.
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/canvas"); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	waitForQuickstartAppRoute(t, page)
	canvas := page.Locator("[data-testid='canvas-viewport']:visible [data-testid='unixfs-browser']:visible").First()
	if err := canvas.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		t.Fatalf("seeded Canvas file node did not render: %v", err)
	}
	if err := canvas.GetByText("This folder is empty", playwright.LocatorGetByTextOptions{Exact: new(true)}).WaitFor(); err != nil {
		t.Fatalf("seeded Canvas folder did not finish loading: %v", err)
	}

	// Read and validate the saved Canvas route for later reopens.
	raw, err := page.Evaluate(`() => location.hash`)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := raw.(string)
	if !ok || !strings.HasPrefix(route, "#/u/") || !strings.HasSuffix(route, "/-/canvas-1") {
		t.Fatalf("Canvas quickstart returned an unexpected route: %v", raw)
	}
	if _, err := page.Evaluate(`() => { globalThis.__canvasReopenDocument = document }`); err != nil {
		t.Fatal(err)
	}

	// Keep one document and browser cache across the first and three later reopens.
	for sample := 1; sample <= 4; sample++ {
		// A six-second visit exceeds Quickstart's five-second handoff grace.
		// It is workload dwell, not a substitute for the Changelog readiness check.
		if _, err := page.Evaluate(`() => { location.hash = '/changelog' }`); err != nil {
			t.Fatal(err)
		}
		if err := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Changelog", Exact: new(true)}).First().WaitFor(); err != nil {
			t.Fatalf("Changelog did not render: %v", err)
		}
		if err := canvas.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
			t.Fatalf("Canvas remained visible on Changelog: %v", err)
		}
		if _, err := page.Evaluate(`() => new Promise(resolve => setTimeout(resolve, 6000))`); err != nil {
			t.Fatal(err)
		}

		// Browser clocks delimit navigation and rendered content without Go RPC latency.
		packs, roots := server.packs.Load(), server.roots.Load()
		raw, err := page.Evaluate(measureCanvasReopen, route)
		if err != nil {
			t.Fatalf("measure Canvas reopen %d: %v", sample, err)
		}
		data, ok := raw.(string)
		if !ok {
			t.Fatalf("Canvas reopen returned an invalid measurement: %v", raw)
		}
		if err := os.WriteFile(filepath.Join(dir, "sample-"+strconv.Itoa(sample)+".json"), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := fastjson.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Canvas reopen %d: ready=%t elapsed=%.1fms packs=%d roots=%d", sample, result.GetBool("ready"), result.GetFloat64("elapsedMs"), server.packs.Load()-packs, server.roots.Load()-roots)
		if !result.GetBool("ready") {
			t.Fatalf("Canvas reopen %d failed: %s (see saved sample)", sample, result.GetStringBytes("error"))
		}
		if !result.GetBool("sameDocument") {
			t.Fatal("reopen replaced the setup document")
		}
		if result.GetInt("handoffs") != 0 {
			t.Fatal("ordinary reopen consumed a Quickstart handoff")
		}
		if result.GetInt("workerStarts") != 0 {
			t.Fatal("reopen constructed a new runtime or plugin worker")
		}
	}

	// Reuse the runtime trace collector; its timestamps span setup and all samples.
	if releaseStartupTraceEnabled() {
		data, err := captureReleaseStartupTrace(t.Context(), testHarness.browser)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "runtime.trace"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// measureCanvasReopen records foreground loading states and existing mount marks
// until the saved Canvas file node renders. Failed samples retain partial evidence.
const measureCanvasReopen = `async route => {
	const origin = performance.timeOrigin
	const start = performance.now()
	const phases = []
	let lastPhase = ''
	let ready = false
	let error = ''
	const visible = element => !!element && element.getClientRects().length > 0
	location.hash = route
	while (performance.now() - start < 60000) {
		await new Promise(resolve => requestAnimationFrame(resolve))
		const viewport = Array.from(document.querySelectorAll('[data-testid="canvas-viewport"]')).find(visible)
		const files = viewport?.querySelector('[data-testid="unixfs-browser"]')
		const contentReady = visible(files) && files.innerText.includes('This folder is empty')
		const text = document.body.innerText
		const phase = contentReady ? 'canvas-content' : visible(viewport) ? 'canvas-frame' :
			text.includes('Connecting to the session.') ? 'session' :
			text.includes('Opening your space') ? 'space' :
			text.includes('Resolving object type and preparing the viewer.') ? 'object' : 'other'
		if (phase !== lastPhase) {
			phases.push({ phase, atMs: performance.now() - start })
			lastPhase = phase
		}
		if (location.hash === route && contentReady) {
			ready = true
			break
		}
	}
	const end = performance.now()
	if (!ready) error = 'Canvas and its saved file node did not render within 60 seconds'
	const marks = performance.getEntriesByType('mark')
		.filter(mark => mark.startTime >= start && mark.startTime <= end && mark.name.startsWith('spacewave.startup.'))
		.map(mark => ({ name: mark.name, atMs: mark.startTime - start, detail: mark.detail }))
	const resources = performance.getEntriesByType('resource')
		.filter(entry => entry.startTime >= start && entry.startTime <= end)
		.map(entry => ({ name: entry.name, atMs: entry.startTime - start, durationMs: entry.duration,
			transferSize: entry.transferSize, encodedBodySize: entry.encodedBodySize }))
	return JSON.stringify({ route, ready, error, timeOrigin: origin, startMs: start, endMs: end,
		sameDocument: globalThis.__canvasReopenDocument === document,
		elapsedMs: end - start, phases, marks, resources,
		handoffs: marks.filter(mark => mark.name.includes('-handoff-used')).length,
		workerStarts: marks.filter(mark => /worker.construct-start|runtime.worker-created/.test(mark.name)).length,
		body: ready ? undefined : document.body.innerText })
}`
