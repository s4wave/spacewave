//go:build !skip_e2e && !js

package electron

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
)

// TestCanvasReferenceCapture captures the real Canvas quickstart in an isolated
// Electron session when a visual comparison is explicitly requested.
// TIER: nightly
func TestCanvasReferenceCapture(t *testing.T) {
	// Keep screenshot production opt-in for ordinary Electron test runs.
	output := os.Getenv("E2E_CANVAS_CAPTURE_DIR")
	if output == "" {
		t.Skip("set E2E_CANVAS_CAPTURE_DIR to capture Canvas")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}

	// Use the desktop harness's isolated native backend and ordinary quickstart.
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	t.Cleanup(cancel)
	page, err := waitForShellPage(ctx, testHarness)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.SetViewportSize(1372, 880); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Keep Canvas failure artifacts limited to a failed reference capture.
		if !t.Failed() {
			return
		}

		// Capture the Canvas renderer image for failure inspection.
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path: new(filepath.Join(output, "failure.png")),
		}); err != nil {
			t.Error(err)
		}

		// Save the Canvas renderer text alongside its failure image.
		state, err := page.Locator("body").InnerText()
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(output, "failure.txt"), []byte(state), 0o644); err != nil {
			t.Error(err)
		}
	})
	if _, err := page.Evaluate(`() => { window.location.hash = '#/quickstart/canvas' }`); err != nil {
		t.Fatal(err)
	}

	// Wait for actual Canvas content rather than accepting the route alone.
	if err := page.Locator("[data-testid='canvas-viewport']").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(240000),
	}); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("[data-canvas-node='unixfs-demo']").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(120000),
	}); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("This folder is empty", playwright.PageGetByTextOptions{Exact: new(true)}).First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: new(filepath.Join(output, "canvas-live.png")),
	}); err != nil {
		t.Fatal(err)
	}

	// Compose two real object routes through the shell's existing grid format.
	route, err := page.Evaluate(`() => window.location.hash.slice(1)`)
	if err != nil {
		t.Fatal(err)
	}

	// Construct the split Canvas and UnixFS shell layout.
	left := shellGridTabset("grid-left", "grid-home", "Canvas 1")
	right := shellGridTabset("grid-right", "grid-blog", "UnixFS Viewer")
	left.GetTabSet().Weight = 55
	right.GetTabSet().Weight = 45
	snapshot := &s4wave_layout.LayoutSnapshot{
		Model: &s4wave_layout.LayoutModel{
			Layout: &s4wave_layout.RowDef{
				Weight:   100,
				Children: []*s4wave_layout.RowOrTabSetDef{left, right},
			},
		},
		LocalState: &s4wave_layout.LayoutLocalState{
			ActiveTabSetId: "grid-left",
			TabSetSelections: map[string]string{
				"grid-left":  "grid-home",
				"grid-right": "grid-blog",
			},
		},
	}

	// Encode the split shell layout for the Canvas reference route.
	data, err := snapshot.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Open the Canvas and UnixFS routes through the encoded shell layout.
	if _, err := page.Evaluate(`([route, layout]) => {
		const records = [
			{ id: 'grid-home', name: 'Canvas 1', path: route, creationSequence: 1 },
			{ id: 'grid-blog', name: 'UnixFS Viewer', path: route.replace(/canvas-1$/, 'files'), creationSequence: 2 },
		]
		localStorage.setItem('browser-shell-tabs', JSON.stringify({ schemaVersion: 1, epoch: 1, revision: 1, records }))
		sessionStorage.setItem('shell-document-state', JSON.stringify({ incarnation: 'canvas-reference', activeTabId: 'grid-home' }))
		sessionStorage.removeItem('shell-tabs-layout')
		window.location.hash = '#/g/' + layout
	}`, []any{route, base64.RawURLEncoding.EncodeToString(data)}); err != nil {
		t.Fatal(err)
	}

	// Reload the split shell and wait for both object views to finish loading.
	if _, err := page.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("[data-testid='canvas-viewport']").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(120000),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const viewports = document.querySelectorAll('[data-testid="canvas-viewport"]')
		return viewports.length === 1 && !document.body.innerText.includes('Loading files') &&
			(document.body.innerText.match(/This folder is empty/g) ?? []).length === 2
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(120000)}); err != nil {
		t.Fatal(err)
	}

	// Capture the loaded split Canvas and UnixFS reference image.
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: new(filepath.Join(output, "canvas-split.png")),
	}); err != nil {
		t.Fatal(err)
	}

	// Allow a deliberate pointer dwell to expose the breadcrumb's hover behavior.
	breadcrumb := page.GetByText("My Canvas", playwright.PageGetByTextOptions{Exact: new(true)}).First()
	if err := breadcrumb.Hover(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => new Promise(resolve => setTimeout(resolve, 600))`); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: new(filepath.Join(output, "canvas-breadcrumb-hover.png")),
	}); err != nil {
		t.Fatal(err)
	}

	// Capture how the existing Space details control affects the Canvas pane.
	if err := breadcrumb.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("[role='dialog']:visible").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: new(filepath.Join(output, "canvas-space-details.png")),
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("Canvas reference captures: %s", output)
}
