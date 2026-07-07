//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

const deviceQuickstartWaitMS = 240000
const deviceQuickstartProbeWaitMS = 120000

// STORY_DEV_001 / STORY_DEV_002: the Device quickstart opens Computers first,
// then the Add Device control launches the setup wizard.
func TestGoScriptDeviceQuickstartOpensComputersDashboard(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	h := harness(t)
	sess := h.NewCleanPageSession(t)
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()
	defer func() {
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during Device quickstart gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during Device quickstart gate: %+v", report)
		}
	}()

	page := sess.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/device")

	proof := waitForDeviceQuickstartSurface(t, page)
	if proof.Timeout {
		t.Fatalf("Device quickstart did not reach a typed surface before timeout; proof: %+v", proof)
	}
	if got := proof.RouteProbe.SpaceState.IndexPath; got != "computers" {
		t.Fatalf("Device quickstart indexPath = %q, want computers; proof: %+v", got, proof)
	}
	if got := proof.RouteObjectKey; got != "computers" {
		t.Fatalf("Device quickstart route object = %q, want computers; proof: %+v", got, proof)
	}
	if got := proof.RouteProbe.SpaceState.ComputersObjectType; got != "spacewave/computers" {
		t.Fatalf("Device quickstart computers object type = %q, want spacewave/computers; proof: %+v", got, proof)
	}
	if proof.HasWizardSurface {
		t.Fatalf("Device quickstart opened AddDeviceWizardViewer before the user clicked Add Device; proof: %+v", proof)
	}
	if !proof.HasDashboardSurface {
		t.Fatalf("Device quickstart did not render ComputersDashboardViewer; proof: %+v", proof)
	}
	if !strings.HasSuffix(proof.Hash, "/-/computers") {
		t.Fatalf("Device quickstart hash = %q, want canonical Computers object route; proof: %+v", proof.Hash, proof)
	}

	dashboardHash := proof.Hash
	seededDevice := seedDeviceQuickstartDevice(t, h, page)
	deviceObjectKey := stringField(seededDevice, "objectKey")
	if deviceObjectKey == "" {
		t.Fatalf("seed Device helper returned no object key: %#v", seededDevice)
	}
	waitForDeviceDashboardRow(t, page, deviceObjectKey)
	if err := page.Locator("button:has-text('" + deviceObjectKey + "')").First().Click(
		playwright.LocatorClickOptions{Timeout: playwright.Float(deviceQuickstartWaitMS)},
	); err != nil {
		t.Fatalf("click Device dashboard row %q: %v\ndebug: %v", deviceObjectKey, err, collectDeviceQuickstartDebug(page))
	}
	waitForDeviceViewer(t, page, deviceObjectKey)

	NavigateHash(t, h, page, dashboardHash)
	waitForDeviceDashboardRow(t, page, deviceObjectKey)

	addDevice := page.Locator("button:has-text('Add Device')").First()
	if err := addDevice.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(deviceQuickstartWaitMS),
	}); err != nil {
		t.Fatalf("click Device dashboard Add Device: %v\ndebug: %v", err, collectDeviceQuickstartDebug(page))
	}
	waitForAddDeviceWizard(t, page)
}

type deviceQuickstartSurfaceProof struct {
	Timeout             bool                       `json:"timeout"`
	Hash                string                     `json:"hash"`
	RouteObjectKey      string                     `json:"routeObjectKey"`
	TimingState         string                     `json:"timingState"`
	TimingError         string                     `json:"timingError"`
	HasDashboardSurface bool                       `json:"hasDashboardSurface"`
	HasWizardSurface    bool                       `json:"hasWizardSurface"`
	AppText             string                     `json:"appText"`
	Buttons             []deviceQuickstartButton   `json:"buttons"`
	RouteProbe          deviceQuickstartRouteProbe `json:"routeProbe"`
}

type deviceQuickstartButton struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type deviceQuickstartRouteProbe struct {
	Skipped      bool                       `json:"skipped"`
	HasDebugRoot bool                       `json:"hasDebugRoot"`
	Step         string                     `json:"step"`
	Error        string                     `json:"error"`
	SpaceState   deviceQuickstartSpaceState `json:"spaceState"`
}

type deviceQuickstartSpaceState struct {
	Ready               bool     `json:"ready"`
	IndexPath           string   `json:"indexPath"`
	ObjectKeys          []string `json:"objectKeys"`
	ComputersObjectType string   `json:"computersObjectType"`
}

func waitForDeviceQuickstartSurface(t testing.TB, page playwright.Page) deviceQuickstartSurfaceProof {
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

		function normalizedAppText() {
			return document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').trim() ?? ''
		}

		function addDeviceButtonVisible() {
			return Array.from(document.querySelectorAll('button')).some((button) =>
				(button.textContent ?? '').replace(/\s+/g, ' ').trim() === 'Add Device',
			)
		}

		function hasDashboardSurface(text) {
			return (
				text.includes('Computers') &&
				text.includes('Devices') &&
				text.includes('Hosts') &&
				text.includes('No Device objects yet') &&
				text.includes('No host entries yet') &&
				addDeviceButtonVisible()
			)
		}

		function hasWizardSurface(text) {
			return (
				text.includes('Add Device') &&
				text.includes('SpaceLink') &&
				text.includes('SSH Host') &&
				text.includes('Device Name')
			)
		}

		function routeObjectKey(hash) {
			const match = hash.match(/\/-\/(.+)$/)
			return match ? decodeURIComponent(match[1]) : ''
		}

		function buttons() {
			return Array.from(document.querySelectorAll('button')).map((button) => ({
				title: button.getAttribute('title') ?? '',
				text: (button.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 80),
			}))
		}

		function collect(timeout, routeProbe) {
			const timing =
				globalThis.__s4waveQuickstartTiming ??
				globalThis.__s4wave_debug?.quickstartTiming ??
				null
			const hash = window.location.hash
			const appText = normalizedAppText()
			return {
				timeout,
				hash,
				routeObjectKey: routeObjectKey(hash),
				timingState: timing?.state ?? '',
				timingError: timing?.error ?? '',
				hasDashboardSurface: hasDashboardSurface(appText),
				hasWizardSurface: hasWizardSurface(appText),
				appText: appText.slice(0, 2400),
				buttons: buttons(),
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
				const computersObject = objects.find((obj) => obj.objectKey === 'computers') ?? null
				return {
					skipped: false,
					hasDebugRoot: true,
					step,
					spaceState: {
						ready: !!state?.ready,
						indexPath: state?.settings?.indexPath ?? '',
						objectKeys: objects.map((obj) => obj.objectKey ?? ''),
						computersObjectType: computersObject?.objectType ?? '',
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
			const text = normalizedAppText()
			const surfaceVisible = hasDashboardSurface(text) || hasWizardSurface(text)
			const hasRoute = /^#\/u\/([0-9]+)\/so\/([^/]+)/.test(window.location.hash)
			const timing =
				globalThis.__s4waveQuickstartTiming ??
				globalThis.__s4wave_debug?.quickstartTiming ??
				null
			if (timing?.state === 'error') {
				lastProbe = await routeProbe()
				return JSON.stringify(collect(false, lastProbe))
			}
			if (hasRoute && surfaceVisible) {
				lastProbe = await routeProbe()
				if (!lastProbe?.skipped && !lastProbe?.error) {
					return JSON.stringify(collect(false, lastProbe))
				}
			}
			await new Promise((resolve) => setTimeout(resolve, 250))
		}
		if (!lastProbe || lastProbe.skipped) {
			lastProbe = await routeProbe()
		}
		return JSON.stringify(collect(true, lastProbe))
	}`, deviceQuickstartProbeWaitMS)
	if err != nil {
		t.Fatalf("Device quickstart surface probe evaluation failed: %v\ndebug: %v", err, collectDeviceQuickstartDebug(page))
	}
	text, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected Device quickstart surface probe result %T: %#v", raw, raw)
	}
	proof, err := parseDeviceQuickstartSurfaceProof(text)
	if err != nil {
		t.Fatalf("decode Device quickstart surface proof: %v\nraw: %s", err, text)
	}
	return proof
}

func parseDeviceQuickstartSurfaceProof(text string) (deviceQuickstartSurfaceProof, error) {
	var parser fastjson.Parser
	v, err := parser.Parse(text)
	if err != nil {
		return deviceQuickstartSurfaceProof{}, err
	}
	return deviceQuickstartSurfaceProof{
		Timeout:             v.GetBool("timeout"),
		Hash:                string(v.GetStringBytes("hash")),
		RouteObjectKey:      string(v.GetStringBytes("routeObjectKey")),
		TimingState:         string(v.GetStringBytes("timingState")),
		TimingError:         string(v.GetStringBytes("timingError")),
		HasDashboardSurface: v.GetBool("hasDashboardSurface"),
		HasWizardSurface:    v.GetBool("hasWizardSurface"),
		AppText:             string(v.GetStringBytes("appText")),
		Buttons:             parseDeviceQuickstartButtons(v.GetArray("buttons")),
		RouteProbe: deviceQuickstartRouteProbe{
			Skipped:      v.GetBool("routeProbe", "skipped"),
			HasDebugRoot: v.GetBool("routeProbe", "hasDebugRoot"),
			Step:         string(v.GetStringBytes("routeProbe", "step")),
			Error:        string(v.GetStringBytes("routeProbe", "error")),
			SpaceState: deviceQuickstartSpaceState{
				Ready:               v.GetBool("routeProbe", "spaceState", "ready"),
				IndexPath:           string(v.GetStringBytes("routeProbe", "spaceState", "indexPath")),
				ObjectKeys:          deviceQuickstartJSONStringSlice(v.GetArray("routeProbe", "spaceState", "objectKeys")),
				ComputersObjectType: string(v.GetStringBytes("routeProbe", "spaceState", "computersObjectType")),
			},
		},
	}, nil
}

func parseDeviceQuickstartButtons(values []*fastjson.Value) []deviceQuickstartButton {
	out := make([]deviceQuickstartButton, 0, len(values))
	for _, value := range values {
		out = append(out, deviceQuickstartButton{
			Title: string(value.GetStringBytes("title")),
			Text:  string(value.GetStringBytes("text")),
		})
	}
	return out
}

func deviceQuickstartJSONStringSlice(values []*fastjson.Value) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value.GetStringBytes()))
	}
	return out
}

func seedDeviceQuickstartDevice(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	raw, err := page.Evaluate(h.Script("device-quickstart.ts"), map[string]any{
		"action":     "seed-device",
		"objectKey":  "devices/e2e-build-host",
		"label":      "E2E Build Host",
		"peerId":     "12D3KooWE2EDevice",
		"deadlineMs": deviceQuickstartWaitMS,
	})
	if err != nil {
		t.Fatalf("seed Device object through browser world API: %v\ndebug: %v", err, collectDeviceQuickstartDebug(page))
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected Device seed helper result %T: %#v", raw, raw)
	}
	if got := stringField(result, "typeId"); got != "spacewave/device" {
		t.Fatalf("seed Device helper typeId = %q, want spacewave/device: %#v", got, result)
	}
	return result
}

func waitForDeviceDashboardRow(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const objectKey = Array.isArray(arg) ? arg[0] : arg
		const text = document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').trim() ?? ''
		const hasAddDeviceButton = Array.from(document.querySelectorAll('button')).some((button) =>
			(button.textContent ?? '').replace(/\s+/g, ' ').trim() === 'Add Device',
		)
		const hasDeviceRow = Array.from(document.querySelectorAll('button')).some((button) => {
			const buttonText = (button.textContent ?? '').replace(/\s+/g, ' ').trim()
			return buttonText.includes(objectKey) && buttonText.includes('Open')
		})
		return (
			window.location.hash.endsWith('/-/computers') &&
			text.includes('Computers') &&
			text.includes('Devices') &&
			text.includes('Hosts') &&
			hasAddDeviceButton &&
			hasDeviceRow
		)
	}`, []any{objectKey}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(deviceQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Device dashboard row %q: %v\ndebug: %v", objectKey, err, collectDeviceQuickstartDebug(page))
	}
}

func waitForDeviceViewer(t testing.TB, page playwright.Page, objectKey string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const objectKey = Array.isArray(arg) ? arg[0] : arg
		const text = document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').trim() ?? ''
		return (
			window.location.hash.includes('/-/' + objectKey) &&
			text.includes('Device') &&
			text.includes('Name') &&
			text.includes('Identity') &&
			text.includes('Last Status') &&
			text.includes('Capabilities') &&
			text.includes('No capabilities declared')
		)
	}`, []any{objectKey}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(deviceQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for DeviceViewer object=%q: %v\ndebug: %v", objectKey, err, collectDeviceQuickstartDebug(page))
	}
}

func waitForAddDeviceWizard(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.WaitForFunction(`() => {
		const text = document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').trim() ?? ''
		return (
			text.includes('Add Device') &&
			text.includes('SpaceLink') &&
			text.includes('SSH Host') &&
			text.includes('Device Name')
		)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(deviceQuickstartWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for Add Device wizard after dashboard click: %v\ndebug: %v", err, collectDeviceQuickstartDebug(page))
	}
}

func collectDeviceQuickstartDebug(page playwright.Page) any {
	debug, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		hasDebugRoot: !!globalThis.__s4wave_debug?.root,
		quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 2400) ?? '',
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			title: button.getAttribute('title'),
			testid: button.getAttribute('data-testid'),
			text: button.textContent?.replace(/\s+/g, ' ').slice(0, 100) ?? '',
		})),
		testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
			testid: el.getAttribute('data-testid'),
			text: el.textContent?.replace(/\s+/g, ' ').slice(0, 180) ?? '',
		})),
	})`)
	if err != nil {
		return "failed to collect Device quickstart debug: " + err.Error()
	}
	return debug
}
