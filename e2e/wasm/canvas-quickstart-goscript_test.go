//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const canvasQuickstartWaitMS = 240000

func TestGoScriptCanvasQuickstartCreateMutateReloadParity(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	sess := testHarness.NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during Canvas quickstart gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Canvas quickstart gate: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/canvas")
	waitForCanvasViewerReady(t, page)

	canvasHash := pageHash(t, page)
	if !strings.HasSuffix(canvasHash, "/canvas-1") {
		t.Fatalf("expected Canvas quickstart route to end at canvas-1, got %q\ndebug: %v", canvasHash, collectCanvasQuickstartDebug(page))
	}

	addCanvasTextNode(t, page, "GoScript Canvas Proof")
	waitForCanvasText(t, page, "GoScript Canvas Proof")

	assertCanvasRouteReloadAndReopen(t, page, canvasHash, func() {
		waitForCanvasViewerReady(t, page)
		waitForCanvasText(t, page, "GoScript Canvas Proof")
	})
}

func waitForCanvasViewerReady(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.WaitForFunction(`() => {
		const timing =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		if (timing?.state === 'error') {
			throw new Error('Canvas quickstart failed: ' + (timing.error ?? 'unknown error'))
		}
		const hash = window.location.hash
		if (!hash.includes('/u/') || !hash.includes('/so/') || !hash.endsWith('/canvas-1')) {
			return false
		}
		const viewport = document.querySelector('[data-testid="canvas-viewport"]')
		const demoNode = document.querySelector('[data-canvas-node="unixfs-demo"]')
		const text = document.body.textContent ?? ''
		if (text.includes('Setup Failed') || text.includes('Error loading')) {
			throw new Error(text.replace(/\s+/g, ' ').slice(0, 1000))
		}
		return !!viewport && !!demoNode
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(canvasQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Canvas viewer: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}
}

func addCanvasTextNode(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	if err := page.Locator("button[aria-label='Text (T)']").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(canvasQuickstartWaitMS)},
	); err != nil {
		t.Fatalf("select Canvas text tool: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}

	viewport := page.Locator("[data-testid='canvas-viewport']").First()
	box, err := viewport.BoundingBox()
	if err != nil {
		t.Fatalf("measure Canvas viewport: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}
	x := box.Width - 180
	if x < 80 {
		x = box.Width / 2
	}
	y := box.Height - 120
	if y < 80 {
		y = box.Height / 2
	}
	if err := viewport.Click(playwright.LocatorClickOptions{
		Position: &playwright.Position{X: x, Y: y},
		Timeout:  playwright.Float(canvasQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("place Canvas text node: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}

	editor := page.Locator("[data-testid='canvas-viewport'] textarea").First()
	if err := editor.Fill(text, playwright.LocatorFillOptions{
		Timeout: playwright.Float(canvasQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("fill Canvas text node: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}
	if err := editor.Press("Control+Enter"); err != nil {
		t.Fatalf("commit Canvas text node: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}
}

func waitForCanvasText(t testing.TB, page playwright.Page, text string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const want = Array.isArray(arg) ? arg[0] : arg
		const nodes = Array.from(document.querySelectorAll('[data-canvas-node]'))
		return nodes.some((node) => (node.textContent ?? '').includes(want))
	}`, []any{text}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(canvasQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Canvas text %q: %v\ndebug: %v", text, err, collectCanvasQuickstartDebug(page))
	}
}

func assertCanvasRouteReloadAndReopen(
	t testing.TB,
	page playwright.Page,
	hash string,
	assertReady func(),
) {
	t.Helper()

	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload Canvas route: %v", err)
	}
	WaitForApp(t, page)
	assertReady()

	NavigateHash(t, testHarness, page, "#/")
	NavigateHash(t, testHarness, page, hash)
	assertReady()
}

func pageHash(t testing.TB, page playwright.Page) string {
	t.Helper()

	raw, err := page.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read page hash: %v", err)
	}
	hash, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected hash value %T: %#v", raw, raw)
	}
	return hash
}

func collectCanvasQuickstartDebug(page playwright.Page) any {
	debug, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		timing: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		startup: globalThis.__swBootStatus ?? null,
		startupMarks: globalThis.__swStartupMarks ?? [],
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2200) ?? '',
		canvasNodes: Array.from(document.querySelectorAll('[data-canvas-node]')).map((node) => ({
			id: node.getAttribute('data-canvas-node'),
			text: node.textContent?.replace(/\s+/g, ' ').trim().slice(0, 220) ?? '',
		})),
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			text: button.textContent?.replace(/\s+/g, ' ').trim().slice(0, 120) ?? '',
			ariaLabel: button.getAttribute('aria-label') ?? '',
			title: button.getAttribute('title') ?? '',
			disabled: button.disabled,
		})),
		testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
			testid: el.getAttribute('data-testid'),
			text: el.textContent?.replace(/\s+/g, ' ').trim().slice(0, 160) ?? '',
		})),
	})`)
	if err != nil {
		return "failed to collect Canvas quickstart debug: " + err.Error()
	}
	if s, ok := debug.(string); ok {
		return strings.TrimSpace(s)
	}
	return debug
}
