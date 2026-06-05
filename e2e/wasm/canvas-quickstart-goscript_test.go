//go:build !skip_e2e && !js

package wasm

import (
	"encoding/json"
	"strings"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const canvasQuickstartWaitMS = 240000
const canvasQuickstartProbeWaitMS = 120000

func TestGoScriptCanvasQuickstartRouteResourceProbe(t *testing.T) {
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
			t.Errorf("unexpected browser/WASM crash report during Canvas route probe: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Canvas route probe: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/canvas")

	probe := waitForCanvasRouteResourceProbe(t, page)
	if probe.Timeout {
		t.Fatalf("Canvas route/resource probe timed out: %+v", probe)
	}
	if !strings.HasSuffix(probe.Hash, "/-/canvas-1") {
		t.Fatalf("Canvas route = %q, want canonical /-/canvas-1 route; probe: %+v", probe.Hash, probe)
	}
	if probe.RouteProbe.SpaceState.ObjectType != "canvas" {
		t.Fatalf("Canvas object type = %q, want canvas; probe: %+v", probe.RouteProbe.SpaceState.ObjectType, probe)
	}
	if probe.RouteProbe.CanvasAccess.TypeID != "canvas" {
		t.Fatalf("Canvas access type = %q, want canvas; probe: %+v", probe.RouteProbe.CanvasAccess.TypeID, probe)
	}
	if probe.RouteProbe.CanvasAccess.ResourceID == 0 {
		t.Fatalf("Canvas access returned no resource id; probe: %+v", probe)
	}
	if !probe.HasViewport || !probe.HasDemoNode {
		t.Fatalf("Canvas viewer did not render seeded viewport/node; probe: %+v", probe)
	}
}

func TestGoScriptCanvasQuickstartCreateMutate(t *testing.T) {
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
			t.Errorf("unexpected browser/WASM crash report during Canvas create/mutate gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Canvas create/mutate gate: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, testHarness, page, "#/quickstart/canvas")

	probe := waitForCanvasRouteResourceProbe(t, page)
	if probe.Timeout {
		t.Fatalf("Canvas route/resource probe timed out before mutation: %+v", probe)
	}
	if !strings.HasSuffix(probe.Hash, "/-/canvas-1") {
		t.Fatalf("Canvas route = %q, want canonical /-/canvas-1 route; probe: %+v", probe.Hash, probe)
	}

	addCanvasTextNode(t, page, "GoScript Canvas Proof")
	waitForCanvasText(t, page, "GoScript Canvas Proof")
}

type canvasRouteResourceProbe struct {
	Timeout     bool                    `json:"timeout"`
	Hash        string                  `json:"hash"`
	TimingState string                  `json:"timingState"`
	HasViewport bool                    `json:"hasViewport"`
	HasDemoNode bool                    `json:"hasDemoNode"`
	AppText     string                  `json:"appText"`
	RouteProbe  canvasRouteResourceData `json:"routeProbe"`
}

type canvasRouteResourceData struct {
	Skipped      bool                   `json:"skipped"`
	HasDebugRoot bool                   `json:"hasDebugRoot"`
	Step         string                 `json:"step"`
	Error        string                 `json:"error"`
	SpaceState   canvasRouteSpaceState  `json:"spaceState"`
	CanvasAccess canvasRouteCanvasState `json:"canvasAccess"`
}

type canvasRouteSpaceState struct {
	Ready      bool     `json:"ready"`
	IndexPath  string   `json:"indexPath"`
	ObjectKeys []string `json:"objectKeys"`
	ObjectType string   `json:"objectType"`
}

type canvasRouteCanvasState struct {
	TypeID     string `json:"typeId"`
	ResourceID int64  `json:"resourceId"`
}

func waitForCanvasRouteResourceProbe(t testing.TB, page playwright.Page) canvasRouteResourceProbe {
	t.Helper()

	raw, err := page.Evaluate(`async (timeoutMS) => {
		const deadline = Date.now() + timeoutMS

		function firstStreamValue(stream) {
			return (async () => {
				for await (const value of stream) {
					return value
				}
				return null
			})()
		}

		function collect(timeout, routeProbe) {
			const timing =
				globalThis.__s4waveQuickstartTiming ??
				globalThis.__s4wave_debug?.quickstartTiming ??
				null
			const hash = window.location.hash
			return {
				timeout,
				hash,
				timingState: timing?.state ?? '',
				hasViewport: !!document.querySelector('[data-testid="canvas-viewport"]'),
				hasDemoNode: !!document.querySelector('[data-canvas-node="unixfs-demo"]'),
				appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2200) ?? '',
				routeProbe,
			}
		}

		async function routeProbe() {
			const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
			const root = globalThis.__s4wave_debug?.root
			if (!match || !root) {
				return { skipped: true, hasDebugRoot: !!root, step: 'match-route' }
			}
			const sessionIdx = Number(match[1])
			const sharedObjectId = decodeURIComponent(match[2])
			const mountSpace = globalThis.__s4wave_debug?.mountSpace
			const cleanups = []
			let step = 'mountSessionByIdx'
			const cleanup = (value) => {
				if (value?.release) {
					cleanups.push(() => value.release())
				}
				return value
			}
			try {
				const abort = AbortSignal.timeout(15000)
				const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
				const session = cleanup(mounted?.session ?? null)
				if (!session) return { skipped: false, hasDebugRoot: true, step, error: 'missing session' }
				if (!mountSpace) return { skipped: false, hasDebugRoot: true, step: 'mountSpace', error: 'missing debug mountSpace helper' }
				step = 'mountSpace'
				const space = await mountSpace({
					session,
					spaceResp: {
						sharedObjectRef: {
							providerResourceRef: {
								id: sharedObjectId,
							},
						},
					},
					abortSignal: abort,
					cleanup,
				})
				step = 'watchSpaceState'
				const state = await firstStreamValue(space.watchSpaceState({}, abort))
				const objects = state?.worldContents?.objects ?? []
				const canvasObject = objects.find((obj) => obj.objectKey === 'canvas-1') ?? null
				step = 'accessWorldState'
				const world = cleanup(await space.accessWorldState(true, abort))
				step = 'accessTypedObject'
				const access = await world.accessTypedObject('canvas-1', abort)
				const canvasRef = access?.resourceId
					? cleanup(world.getResourceRef().createRef(access.resourceId))
					: null
				return {
					skipped: false,
					hasDebugRoot: true,
					step,
					spaceState: {
						ready: !!state?.ready,
						indexPath: state?.settings?.indexPath ?? '',
						objectKeys: objects.map((obj) => obj.objectKey ?? ''),
						objectType: canvasObject?.objectType ?? '',
					},
					canvasAccess: {
						typeId: access.typeId ?? '',
						resourceId: canvasRef ? access.resourceId : 0,
					},
				}
			} catch (err) {
				return {
					skipped: false,
					hasDebugRoot: true,
					step,
					error: String(err?.stack ?? err),
				}
			} finally {
				for (let i = cleanups.length - 1; i >= 0; i--) {
					try {
						cleanups[i]()
					} catch {}
				}
			}
		}

		let lastProbe = { skipped: true, hasDebugRoot: !!globalThis.__s4wave_debug?.root, step: 'not-started' }
		while (Date.now() < deadline) {
			const hash = window.location.hash
			const canonicalRoute = hash.endsWith('/-/canvas-1')
			const hasViewport = !!document.querySelector('[data-testid="canvas-viewport"]')
			const hasDemoNode = !!document.querySelector('[data-canvas-node="unixfs-demo"]')
			if (canonicalRoute) {
				lastProbe = await routeProbe()
				if (
					lastProbe?.spaceState?.objectType === 'canvas' &&
					lastProbe?.canvasAccess?.typeId === 'canvas' &&
					!!lastProbe?.canvasAccess?.resourceId &&
					hasViewport &&
					hasDemoNode
				) {
					return JSON.stringify(collect(false, lastProbe))
				}
			}
			await new Promise((resolve) => setTimeout(resolve, 250))
		}
		if (!lastProbe || lastProbe.skipped) {
			lastProbe = await routeProbe()
		}
		return JSON.stringify(collect(true, lastProbe))
	}`, canvasQuickstartProbeWaitMS)
	if err != nil {
		t.Fatalf("Canvas route/resource probe evaluation failed: %v\ndebug: %v", err, collectCanvasQuickstartDebug(page))
	}
	text, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected Canvas route/resource probe result %T: %#v", raw, raw)
	}
	var probe canvasRouteResourceProbe
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		t.Fatalf("decode Canvas route/resource probe: %v\nraw: %s", err, text)
	}
	return probe
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
