//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

const (
	browserWaitMS                         = 420000
	foregroundResumeReadyRecordMS         = 10000
	quickstartContentReadyRecordMS        = 60000
	quickstartPostLoadSOOperationCount    = 25
	quickstartPostLoadSOWorkloadTimeoutMS = 120000
)

const (
	sonnerModulePath     = "/b/pkg/sonner/dist/index.mjs"
	webPkgArtifactRelDir = ".bldr-dist/build/js/spacewave-web/assets/bldr-web-pkgs"
)

// waitForPluginWorkersRunning waits for a plugin.running mark from each
// logical plugin worker. A physical worker id extends the logical id with its
// instance and manifest generation, so the logical id matches as a path prefix.
func waitForPluginWorkersRunning(t *testing.T, page playwright.Page, workerIDs []string) {
	// Wait for each logical plugin worker to reach its running state.
	t.Helper()
	_, err := page.WaitForFunction(`(workerIds) => {
		const marks = globalThis.__swStartupMarks ?? []
		return workerIds.every((workerId) =>
			marks.some((mark) =>
				mark.label === 'plugin.running' &&
				(mark.detail?.workerId === workerId ||
					mark.detail?.workerId?.startsWith(workerId + '/')),
			),
		)
	}`, workerIDs, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for plugin workers running %v: %v", workerIDs, err)
	}
}

type quickstartRuntimeTraceCapture struct {
	started bool
	stopped bool
	info    map[string]any
}

func beginQuickstartRuntimeTrace(t *testing.T, page playwright.Page) *quickstartRuntimeTraceCapture {
	// Prepare the quickstart runtime trace record.
	t.Helper()
	path := testHarness.quickstartRuntimeTraceArtifactPath(t)
	info := map[string]any{
		"kind":                   "chromium-devtools-runtime-trace",
		"captured":               false,
		"captureWindow":          "before-page-goto-through-foreground-resume-probe",
		"startupPerformanceGate": "frame-ready",
		"seedCompletionGate":     "drive-content-ready",
		"postLoadWorkloadGate":   "sequential-shared-object-operations",
		"foregroundResumeGate":   "web-document.resume-ready",
		"path":                   path,
	}
	c := &quickstartRuntimeTraceCapture{info: info}

	// Require Chromium and the trace opt-in before capturing runtime activity.
	if testHarness.browserName != "chromium" {
		info["skippedReason"] = "Chromium tracing is only available for the chromium release WASM browser"
		return c
	}
	if os.Getenv("E2E_RELEASE_WASM_RUNTIME_TRACE") != "1" {
		info["skippedReason"] = "set E2E_RELEASE_WASM_RUNTIME_TRACE=1 to capture a Chromium runtime trace"
		return c
	}

	// Remove the previous trace artifact and start a fresh browser trace.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove previous runtime trace artifact: %v", err)
	}
	screenshots := false
	if err := testHarness.browser.StartTracing(playwright.BrowserStartTracingOptions{
		Page:        page,
		Path:        &path,
		Screenshots: &screenshots,
	}); err != nil {
		t.Fatalf("start quickstart runtime trace: %v", err)
	}

	// Record that tracing began before quickstart navigation.
	c.started = true
	info["captured"] = true
	info["startedBefore"] = "page.goto('/quickstart/drive')"
	return c
}

func (c *quickstartRuntimeTraceCapture) stop(t *testing.T) map[string]any {
	// Leave unstarted or already stopped runtime traces unchanged.
	t.Helper()
	if !c.started || c.stopped {
		return c.info
	}

	// Stop the browser runtime trace and verify it contains data.
	data, err := testHarness.browser.StopTracing()
	c.stopped = true
	if err != nil {
		t.Fatalf("stop quickstart runtime trace: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty quickstart runtime trace")
	}

	// Record the runtime trace's size and capture endpoint.
	c.info["bytes"] = len(data)
	c.info["stoppedAfter"] = "foreground resume probe"
	t.Logf("quickstart runtime trace written to %s (%d bytes)", c.info["path"], len(data))
	return c.info
}

func (c *quickstartRuntimeTraceCapture) cleanup(t *testing.T) {
	// Leave unstarted or already stopped traces alone during cleanup.
	t.Helper()
	if !c.started || c.stopped {
		return
	}

	// Stop an abandoned runtime trace and mark it stopped.
	if _, err := testHarness.browser.StopTracing(); err != nil {
		t.Logf("stop abandoned quickstart runtime trace: %v", err)
	}
	c.stopped = true
}

func waitForQuickstartAppRoute(t *testing.T, page playwright.Page) {
	t.Helper()

	// Release boot can preserve the prerender pathname/query while setting the
	// app hash, and a fast quickstart may already be on the created Space route.
	_, err := page.WaitForFunction(`() => {
		const hash = window.location.hash
		if (hash === '#/quickstart/drive') return true
		return /^#\/u\/\d+\/so\/[^/?#]+(?:\/.*)?$/.test(hash)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(30000),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart app route: %v", err)
	}
}

// bootFailureProbeJS returns the page's terminal startup failure text, or
// null. The boot shell's error state and the loading screen's failure are
// terminal. A module error the boundary is still retrying is not.
const bootFailureProbeJS = `() => {
	const status = globalThis.__swBootStatus
	if (status?.state === 'error') {
		return 'boot status error: ' + (status.error || status.detail || status.phase || '')
	}
	const text = document.body?.innerText || ''
	if (text.includes('Unable to open Spacewave')) {
		return text.slice(0, 2000)
	}
	if (/Failed to load module/.test(text) && !text.includes('Retrying in')) {
		return text.slice(0, 2000)
	}
	return null
}`

// waitForDriveFrameReady waits for the visible Drive file browser and returns
// early with the failure when startup reaches a terminal error, so a failed
// boot does not wait out the full browser deadline.
func waitForDriveFrameReady(page playwright.Page, timeoutMS float64) error {
	// Poll until the file browser is visible or startup reports a failure.
	raw, err := page.WaitForFunction(`() => {
		const failure = (`+bootFailureProbeJS+`)()
		if (failure) return { failure }
		for (const el of document.querySelectorAll("[data-testid='unixfs-browser']")) {
			if (el.checkVisibility ? el.checkVisibility() : el.getClientRects().length > 0) {
				return { ready: true }
			}
		}
		return false
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: new(timeoutMS),
	})
	if err != nil {
		return err
	}

	// Return the startup failure the probe found, if any.
	value, err := raw.JSONValue()
	if err != nil {
		return err
	}
	if result, ok := value.(map[string]any); ok {
		if failure, ok := result["failure"].(string); ok {
			return errors.New("startup failed: " + failure)
		}
	}
	return nil
}

func waitForLiveApp(t *testing.T, page playwright.Page) {
	// Wait for the live application within the release browser deadline.
	t.Helper()
	deadline := time.Now().Add(time.Duration(browserWaitMS) * time.Millisecond)
	for {

		// Check the remaining live application readiness deadline.
		timeoutMS := int(time.Until(deadline) / time.Millisecond)
		if timeoutMS <= 0 {
			dumpPageState(t, page)
			t.Fatal("wait for live app: timed out after navigation retries")
		}

		// Evaluate live application readiness in the current document.
		_, err := page.Evaluate(`async (timeoutMs) => {
		const deadline = performance.now() + timeoutMs
		const remaining = () => Math.max(0, deadline - performance.now())
		const timeout = () =>
			new Promise((_, reject) => {
				setTimeout(() => reject(new Error('runtime did not become ready')), remaining())
			})
		await Promise.race([
			globalThis.__swReady,
			timeout(),
		])
		while (document.querySelector('#bldr-root')?.hasAttribute('data-prerendered')) {
			if (performance.now() > deadline) {
				throw new Error('prerender did not switch to live app')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`, timeoutMS)

		// Accept readiness or report a failure unrelated to navigation.
		if err == nil {
			return
		}
		if !isNavigationEvaluationError(err) {
			dumpPageState(t, page)
			t.Fatalf("wait for live app: %v", err)
		}
	}
}

func isNavigationEvaluationError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Execution context was destroyed") ||
		strings.Contains(msg, "navigation")
}

func waitForPrerenderRoot(t *testing.T, page playwright.Page) {
	// Wait for the prerendered release root to appear.
	t.Helper()
	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (!document.querySelector('#bldr-root[data-prerendered]')) {
			if (performance.now() > deadline) {
				throw new Error('missing prerendered bldr root')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for prerender root: %v", err)
	}
}

func waitForPrerenderRootOrLiveApp(t *testing.T, page playwright.Page) {
	// Wait for the release root or live application to appear.
	t.Helper()
	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (true) {
			const root = document.querySelector('#bldr-root')
			if (root?.hasAttribute('data-prerendered')) return true
			if (root && !root.hasAttribute('data-prerendered')) return true
			if (globalThis.__swReady) return true
			if (performance.now() > deadline) {
				throw new Error('missing prerendered or live bldr root')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`)
	if err != nil {
		t.Fatalf("wait for prerender or live app: %v", err)
	}
}

func waitForBootFunction(t *testing.T, page playwright.Page) {
	// Wait for the production boot function and readiness promise.
	t.Helper()
	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 30000
		while (typeof globalThis.__swBoot !== 'function' || !globalThis.__swReady) {
			if (performance.now() > deadline) {
				throw new Error('production boot function did not initialize')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for boot function: %v", err)
	}
}

func assertRuntimeWorkerMode(t *testing.T, page playwright.Page, want string) {
	// Read the selected runtime worker mode from startup marks.
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		for (let i = marks.length - 1; i >= 0; i--) {
			const mark = marks[i]
			if (mark.label === 'runtime.mode-selected') {
				return {
					mode: mark.detail?.mode ?? null,
					mark,
				}
			}
		}
		return null
	}`, nil)
	if err != nil {
		t.Fatalf("read runtime worker mode: %v", err)
	}

	// Verify the runtime mode mark matches the expected worker mode.
	item, ok := raw.(map[string]any)
	if !ok {
		dumpPageState(t, page)
		t.Fatalf("runtime mode mark missing or invalid: %#v", raw)
	}
	if got, _ := item["mode"].(string); got != want {
		dumpPageState(t, page)
		t.Fatalf("runtime worker mode: got %q, want %q; mark=%#v", got, want, item["mark"])
	}
}

func dumpPageState(t *testing.T, page playwright.Page) {
	// Collect the page's routing, storage and startup diagnostics.
	t.Helper()
	state, err := page.Evaluate(`() => {
		const startupPrefix = 'spacewave.startup.'
		const startupMarks = (globalThis.__swStartupMarks ?? []).map((mark) => ({
			label: mark.label,
			sequence: mark.sequence,
			detail: mark.detail,
		}))
		const state = {
			href: window.location.href,
			hash: window.location.hash,
			pathname: window.location.pathname,
			title: document.title,
			text: document.body?.innerText?.slice(0, 4000) ?? '',
			rootHtml: document.querySelector('#bldr-root')?.outerHTML?.slice(0, 4000) ?? '',
			hasDebugRoot: !!globalThis.__s4wave_debug?.root,
			bootStatus: globalThis.__swBootStatus ?? null,
			quickstartTiming:
				globalThis.__s4waveQuickstartTiming ??
				globalThis.__s4wave_debug?.quickstartTiming ??
				null,
			browserShellTabs: localStorage.getItem('browser-shell-tabs'),
			shellLayout: sessionStorage.getItem('shell-tabs-layout'),
			layoutTabs: Array.from(document.querySelectorAll('[data-tab-id]')).map((el) => ({
				id: el.getAttribute('data-tab-id'),
				text: el.textContent?.slice(0, 200) ?? '',
			})),
			testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
				testid: el.getAttribute('data-testid'),
				text: el.textContent?.slice(0, 200) ?? '',
			})),
			startupMarks,
			performanceStartupMarks: performance
				.getEntriesByType('mark')
				.filter((entry) => entry.name.startsWith(startupPrefix))
				.map((entry) => ({
					label: entry.name.slice(startupPrefix.length),
					startTimeMs: Math.round(entry.startTime),
					detail: entry.detail ?? null,
				})),
			globalDebugKeys: Object.keys(globalThis).filter((key) =>
				key.startsWith('__s4wave') || key.startsWith('__sw'),
			),
		}
		return JSON.stringify(state, null, 2)
	}`)
	if err != nil {
		t.Logf("dump page state: %v", err)
		return
	}

	// Decode the page state diagnostic payload.
	stateStr, ok := state.(string)
	if !ok {
		t.Logf("page state: unexpected payload type %T", state)
		return
	}

	// Preserve and report the page state diagnostics.
	writePageStateArtifact(t, stateStr)
	t.Logf("page state: %s", stateStr)
}

func writePageStateArtifact(t *testing.T, state string) {
	// Require a configured artifact directory for page diagnostics.
	t.Helper()
	if testHarness == nil || testHarness.artifactDir == "" {
		return
	}

	// Create the page state artifact's parent directory.
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	path := filepath.Join(
		testHarness.artifactDir,
		replacer.Replace(strings.ToLower(t.Name()))+"-page-state.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("write page state artifact mkdir %s: %v", path, err)
		return
	}

	// Write and report the page state artifact.
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		t.Logf("write page state artifact %s: %v", path, err)
		return
	}
	t.Logf("page state artifact: %s", path)
}

func enableQuickstartTimingLogs(t *testing.T, page playwright.Page) {
	// Enable browser-side quickstart timing logs.
	t.Helper()
	_, err := page.Evaluate(`() => {
		globalThis.__s4waveLogQuickstartTiming = true
	}`)
	if err != nil {
		t.Fatalf("enable quickstart timing logs: %v", err)
	}
}

func logQuickstartTiming(t *testing.T, page playwright.Page) {
	// Read and report the browser's quickstart timing record.
	t.Helper()
	timing, err := page.Evaluate(`() => JSON.stringify(globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null)`)
	if err != nil {
		t.Logf("quickstart timing: %v", err)
		return
	}
	t.Logf("quickstart timing: %v", timing)
}

func assertReturnVisitorBodyRouteSpaceState(t testing.TB, page playwright.Page) {
	// Probe the return visitor body route Space state.
	t.Helper()

	// Read the Space state and a World listing page in the browser.
	raw, err := page.Evaluate(`async () => {
		async function firstStreamValue(stream) {
			for await (const value of stream) {
				return value
			}
			return null
		}
		const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
		const debug = globalThis.__s4wave_debug
		const root = debug?.root
		const mountSpace = debug?.mountSpace
		if (!match || !root || !mountSpace) {
			return { error: 'missing direct SharedObject route or debug root' }
		}
		const sessionIdx = Number(match[1])
		const sharedObjectId = decodeURIComponent(match[2])
		const mountedResources = {
			session: null,
			space: null,
		}
		const cleanupStack = []
		const cleanup = (resource) => {
			cleanupStack.push(resource)
			return resource
		}
		try {
			const abort = AbortSignal.timeout(15000)
			const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
			mountedResources.session = mounted?.session ?? null
			if (!mountedResources.session) return { error: 'mountSessionByIdx returned no session' }
			mountedResources.space = await mountSpace({
				session: mountedResources.session,
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
			const state = await firstStreamValue(mountedResources.space.watchSpaceState({}, abort))
			const world = cleanup(await mountedResources.space.accessWorldState(true, abort))
			const listing = await world.listObjects({ limit: 100 }, abort)
			return {
				ready: !!state?.ready,
				indexPath: state?.settings?.indexPath ?? '',
				objectKeys: (listing.objects ?? [])
					.map((obj) => obj.objectKey ?? '')
					.filter((key) => !key.startsWith('types/')),
			}
		} catch (err) {
			return { error: String(err?.stack ?? err) }
		} finally {
			while (cleanupStack.length) {
				cleanupStack.pop()?.release?.()
			}
			mountedResources.session?.release?.()
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read return visitor body route Space state: %v", err)
	}

	// Verify the return visitor's Space is ready and has World contents.
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected return visitor body route Space state %T: %#v", raw, raw)
	}
	if errMsg := releaseStringField(result, "error"); errMsg != "" {
		t.Fatalf("return visitor body route Space state probe failed: %s", errMsg)
	}
	if !releaseBoolField(result, "ready") {
		t.Fatalf("return visitor body route Space state was not ready: %#v", result)
	}
	objectKeys, ok := result["objectKeys"].([]any)
	if !ok || len(objectKeys) == 0 {
		t.Fatalf("return visitor body route Space state had no world contents: %#v", result)
	}
}

func releaseStringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func releaseBoolField(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func waitForQuickstartDriveContentReady(t *testing.T, page playwright.Page) (*int, string) {
	// Wait for the seeded Drive file to appear.
	t.Helper()
	err := page.Locator("[data-testid='unixfs-browser']:visible").Locator(`text="getting-started.md"`).First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(quickstartContentReadyRecordMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		return nil, err.Error()
	}

	// Record the browser timestamp when Drive content becomes ready.
	driveContentReadyMs := browserNowMs(t, page)
	return &driveContentReadyMs, ""
}

func exerciseQuickstartDriveGoldenPath(t *testing.T, page playwright.Page) (*int, string) {
	// Wait for the Drive welcome guidance.
	t.Helper()
	if err := page.Locator("[data-testid='drive-welcome']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(quickstartContentReadyRecordMS)},
	); err != nil {
		dumpPageState(t, page)
		return nil, "drive welcome guidance did not appear: " + err.Error()
	}

	// Open the Drive invitation dialog.
	if err := page.Locator("[data-testid='drive-invite-cta']:not([disabled])").First().Click(); err != nil {
		dumpPageState(t, page)
		return nil, "click drive invite CTA: " + err.Error()
	}
	dialog := page.Locator("[role='dialog']:has-text('Add User')").First()
	if err := dialog.WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(quickstartContentReadyRecordMS)},
	); err != nil {
		dumpPageState(t, page)
		return nil, "Add User dialog did not open: " + err.Error()
	}

	// Record invitation readiness and close the Add User dialog.
	driveGoldenPathReadyMs := browserNowMs(t, page)
	if err := page.Keyboard().Press("Escape"); err != nil {
		return nil, "close Add User dialog: " + err.Error()
	}
	if err := dialog.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		return nil, "Add User dialog did not close: " + err.Error()
	}
	return &driveGoldenPathReadyMs, ""
}

func completeQuickstartDriveIntroIfPresent(t *testing.T, page playwright.Page) {
	// Complete the Drive intro until its seeded file browser appears.
	t.Helper()
	_, err := page.Evaluate(`async () => {
		const deadline = Date.now() + 120000
		const actionLabels = ['Next', 'Got it, start exploring', 'Open files']
		for (;;) {
			let action = null
			for (const button of document.querySelectorAll('button')) {
				if (!button.disabled && actionLabels.includes(button.textContent?.trim() ?? '')) {
					action = button
					break
				}
			}
			// Finish only after quickstart has seeded the starter file. The
			// wizard and seed writes otherwise race on a fresh profile.
			if (
				action?.textContent?.trim() === 'Got it, start exploring' &&
				!document.body?.innerText?.includes('getting-started.md')
			) {
				action = null
			}
			if (action) {
				action.click()
				await new Promise((resolve) => requestAnimationFrame(resolve))
				continue
			}
			const browser = document.querySelector('[data-testid="unixfs-browser"]')
			if (browser && !window.location.hash.includes('/wizard/')) return null
			if (Date.now() > deadline) {
				throw new Error('Drive intro or file browser did not appear')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("complete quickstart drive intro if present: %v", err)
	}
}

func runQuickstartPostLoadSOWorkload(t *testing.T, page playwright.Page, contentReady bool) map[string]any {
	// Skip the SharedObject workload when Drive content never became ready.
	t.Helper()
	if !contentReady {
		return map[string]any{
			"scenario":      "quickstart-post-load-shared-object-throughput",
			"skipped":       true,
			"skippedReason": "drive content-ready was not reached",
		}
	}

	// Run the browser's sequential post-load SharedObject workload.
	raw, err := page.Evaluate(`async (args) => {
		const debug = globalThis.__s4wave_debug
		if (!debug?.root) {
			throw new Error('debug root is not initialized')
		}
		if (typeof debug.runPostLoadSOPerfTest !== 'function') {
			throw new Error('runPostLoadSOPerfTest is not available')
		}
		const controller = new AbortController()
		const timer = setTimeout(() => controller.abort(), args.timeoutMs)
		try {
			const result = await debug.runPostLoadSOPerfTest(
				debug.root,
				args.opCount,
				controller.signal,
			)
			return JSON.parse(JSON.stringify({
				...result,
				skipped: false,
				timeoutMs: args.timeoutMs,
			}))
		} finally {
			clearTimeout(timer)
		}
	}`, map[string]any{
		"opCount":   quickstartPostLoadSOOperationCount,
		"timeoutMs": quickstartPostLoadSOWorkloadTimeoutMS,
	})
	if err != nil {
		t.Fatalf("run post-load SharedObject workload: %v", err)
	}

	// Verify the SharedObject workload completed the expected operation count.
	workload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected post-load SharedObject workload result %T", raw)
	}
	if got, _ := workload["opCount"].(int); got != quickstartPostLoadSOOperationCount {
		if gotFloat, _ := workload["opCount"].(float64); int(gotFloat) != quickstartPostLoadSOOperationCount {
			t.Fatalf("post-load SharedObject workload accepted %v operations, want %d", workload["opCount"], quickstartPostLoadSOOperationCount)
		}
	}
	t.Logf("post-load SharedObject workload: %#v", workload)
	return workload
}

func collectForegroundResumeEvidence(t *testing.T, page playwright.Page) map[string]any {
	// Read foreground resume readiness before backgrounding the document.
	t.Helper()
	before, err := webDocumentResumeReadySnapshot(page)
	if err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "read initial WebDocument resume readiness: " + err.Error(),
		}
	}

	// Open a temporary page to background the quickstart document.
	beforeSequence := resumeReadySequence(before)
	backgroundPage, err := page.Context().NewPage()
	if err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "open backgrounding page: " + err.Error(),
			"before":        before,
		}
	}
	defer func() {
		if err := backgroundPage.Close(); err != nil {
			t.Logf("close foreground-resume background page: %v", err)
		}
	}()

	// Foreground the temporary blank page.
	if _, err := backgroundPage.Goto("about:blank"); err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "navigate backgrounding page: " + err.Error(),
			"before":        before,
		}
	}
	if err := backgroundPage.BringToFront(); err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "bring backgrounding page to front: " + err.Error(),
			"before":        before,
		}
	}

	// Verify the quickstart document became hidden.
	hiddenObserved, hiddenAtMs, hiddenState := waitForDocumentHiddenState(t, page, true, 5*time.Second)
	if !hiddenObserved {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "browser did not report the quickstart page as hidden",
			"before":        before,
			"hidden":        hiddenState,
		}
	}

	// Foreground the quickstart document and record its resume start.
	if err := page.BringToFront(); err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "bring quickstart page to front: " + err.Error(),
			"before":        before,
			"hidden":        hiddenState,
			"hiddenAtMs":    hiddenAtMs,
		}
	}
	foregroundStartMs := browserNowMs(t, page)

	// Measure the next foreground resume readiness transition.
	raw, err := page.Evaluate(`async (args) => {
		const roundMs = (value) =>
			typeof value === 'number' && Number.isFinite(value) ?
				Math.round(value * 1000) / 1000
			: null
		const readResumeState = () => {
			const state = globalThis.__swWebDocumentResumeReady ?? null
			return state ?
				{
					ready: state.ready === true,
					documentId: state.documentId ?? null,
					runtimeId: state.runtimeId ?? null,
					hidden: state.hidden === true,
					sequence:
						typeof state.sequence === 'number' ? state.sequence : null,
					focused:
						typeof state.focused === 'boolean' ? state.focused : null,
					visibilityState: state.visibilityState ?? null,
					timestampMs: roundMs(state.timestampMs),
				}
			: null
		}
		const deadline = performance.now() + args.timeoutMs
		let state = readResumeState()
		while (
			document.hidden ||
			!state?.ready ||
			typeof state.sequence !== 'number' ||
			state.sequence <= args.beforeSequence ||
			typeof state.timestampMs !== 'number' ||
			state.timestampMs < args.foregroundStartMs
		) {
			if (performance.now() > deadline) {
				return {
					scenario: 'quickstart-drive-foreground-resume',
					skipped: false,
					timedOut: true,
					timeoutMs: args.timeoutMs,
					beforeSequence: args.beforeSequence,
					foregroundStartMs: roundMs(args.foregroundStartMs),
					browserNowMs: roundMs(performance.now()),
					state,
					page: {
						visibilityState: document.visibilityState,
						hidden: document.hidden,
						focused: document.hasFocus(),
					},
				}
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
			state = readResumeState()
		}
		return {
			scenario: 'quickstart-drive-foreground-resume',
			skipped: false,
			timedOut: false,
			timeoutMs: args.timeoutMs,
			beforeSequence: args.beforeSequence,
			foregroundStartMs: roundMs(args.foregroundStartMs),
			resumeReadyMs: state.timestampMs,
			elapsedMs: roundMs(state.timestampMs - args.foregroundStartMs),
			state,
			page: {
				visibilityState: document.visibilityState,
				hidden: document.hidden,
				focused: document.hasFocus(),
			},
			evidence: ['document.visibilityState', 'web-document.resume-ready'],
		}
	}`, map[string]any{
		"beforeSequence":    beforeSequence,
		"foregroundStartMs": foregroundStartMs,
		"timeoutMs":         foregroundResumeReadyRecordMS,
	})
	if err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "wait for foreground resume readiness: " + err.Error(),
			"before":        before,
			"hidden":        hiddenState,
			"hiddenAtMs":    hiddenAtMs,
		}
	}

	// Decode the foreground resume evidence payload.
	evidence, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "unexpected foreground resume evidence payload",
			"before":        before,
			"hidden":        hiddenState,
			"hiddenAtMs":    hiddenAtMs,
		}
	}

	// Attach the initial and hidden states to the resume evidence.
	evidence["before"] = before
	evidence["hidden"] = hiddenState
	evidence["hiddenAtMs"] = hiddenAtMs
	t.Logf("foreground resume evidence: %#v", evidence)
	return evidence
}

func webDocumentResumeReadySnapshot(page playwright.Page) (map[string]any, error) {
	// Read the WebDocument's resume readiness and visibility snapshot.
	raw, err := page.Evaluate(`() => {
		const state = globalThis.__swWebDocumentResumeReady ?? null
		return {
			page: {
				visibilityState: document.visibilityState,
				hidden: document.hidden,
				focused: document.hasFocus(),
			},
			state: state ?
				{
					ready: state.ready === true,
					documentId: state.documentId ?? null,
					runtimeId: state.runtimeId ?? null,
					hidden: state.hidden === true,
					sequence:
						typeof state.sequence === 'number' ? state.sequence : null,
					focused:
						typeof state.focused === 'boolean' ? state.focused : null,
					visibilityState: state.visibilityState ?? null,
					timestampMs:
						typeof state.timestampMs === 'number' ?
							Math.round(state.timestampMs * 1000) / 1000
						: null,
				}
			: null,
		}
	}`)
	if err != nil {
		return nil, err
	}

	// Require an object payload for the resume readiness snapshot.
	snapshot, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.Errorf("unexpected WebDocument resume readiness snapshot %T", raw)
	}
	return snapshot, nil
}

func resumeReadySequence(snapshot map[string]any) int {
	state, _ := snapshot["state"].(map[string]any)
	raw, _ := state["sequence"].(float64)
	return int(raw)
}

// waitForDocumentHiddenState waits in the page for document.hidden to equal
// hidden, watching visibilitychange until the timeout. It returns whether the
// state was reached, the browser time it was observed, and the last snapshot.
func waitForDocumentHiddenState(t *testing.T, page playwright.Page, hidden bool, timeout time.Duration) (bool, int, map[string]any) {
	// Wait in the page for the visibility change, then read its snapshot.
	t.Helper()
	raw, err := page.Evaluate(`async ({ hidden, timeoutMs }) => {
		if (document.hidden !== hidden) {
			await new Promise((resolve) => {
				const finish = () => {
					clearTimeout(timer)
					document.removeEventListener('visibilitychange', onChange)
					resolve()
				}
				const onChange = () => {
					if (document.hidden === hidden) finish()
				}
				const timer = setTimeout(finish, timeoutMs)
				document.addEventListener('visibilitychange', onChange)
			})
		}
		return {
			visibilityState: document.visibilityState,
			hidden: document.hidden,
			focused: document.hasFocus(),
			browserNowMs: Math.round(performance.now()),
		}
	}`, map[string]any{"hidden": hidden, "timeoutMs": timeout.Milliseconds()})
	if err != nil {
		return false, 0, nil
	}

	// Report whether the snapshot reached the requested visibility.
	snapshot, _ := raw.(map[string]any)
	if got, _ := snapshot["hidden"].(bool); got != hidden {
		return false, 0, snapshot
	}
	return true, browserNowFromSnapshot(snapshot), snapshot
}

func browserNowFromSnapshot(snapshot map[string]any) int {
	if val, ok := snapshot["browserNowMs"].(int); ok {
		return val
	}
	if val, ok := snapshot["browserNowMs"].(float64); ok {
		return int(val)
	}
	return 0
}

func browserNowMs(t *testing.T, page playwright.Page) int {
	// Read the browser's current performance timestamp.
	t.Helper()
	raw, err := page.Evaluate(`() => Math.round(performance.now())`)
	if err != nil {
		t.Fatalf("read browser performance.now: %v", err)
	}

	// Decode an integer browser timestamp when available.
	val, ok := raw.(int)
	if ok {
		return val
	}

	// Decode the browser timestamp returned as a floating-point number.
	fval, ok := raw.(float64)
	if !ok {
		t.Fatalf("unexpected performance.now value %T", raw)
	}
	return int(fval)
}

func collectQuickstartSmokeArtifact(
	page playwright.Page,
	desc *browserReleaseDescriptor,
	source map[string]any,
	driveFrameReadyMs int,
	driveContentReadyMs *int,
	driveContentReadyError string,
	driveGoldenPathReadyMs *int,
	driveGoldenPathError string,
	runtimeTrace map[string]any,
	postLoadSOWorkload map[string]any,
	foregroundResume map[string]any,
) ([]byte, error) {

	// Prepare the optional Drive content readiness timestamp.
	var driveContentReadyArg any
	if driveContentReadyMs != nil {
		driveContentReadyArg = *driveContentReadyMs
	}

	// Prepare the optional Drive invitation readiness timestamp.
	var driveGoldenPathReadyArg any
	if driveGoldenPathReadyMs != nil {
		driveGoldenPathReadyArg = *driveGoldenPathReadyMs
	}

	// Collect the quickstart smoke artifact from browser state and timing.
	raw, err := page.Evaluate(`async (args) => {
		const startupPrefix = 'spacewave.startup.'
		const roundMs = (value) =>
			typeof value === 'number' && Number.isFinite(value) ?
				Math.round(value * 1000) / 1000
			: null
		const stableAliases = new Map()
		const stableAlias = (kind, value) => {
			if (typeof value !== 'string' || value === '') return value ?? null
			const existingAliases = stableAliases.get(kind)
			const aliases = existingAliases ?? new Map()
			if (!existingAliases) {
				stableAliases.set(kind, aliases)
			}
			const existingAlias = aliases.get(value)
			if (existingAlias) {
				return existingAlias
			}
			const alias = kind + '-' + (aliases.size + 1)
			aliases.set(value, alias)
			return alias
		}
		const normalizeDetailValue = (key, value) => {
			if (key === 'documentId') return stableAlias('document', value)
			if (key === 'from') return stableAlias('sender', value)
			if (key === 'path') return stableAlias('asset-path', value)
			if (key === 'workerId') return stableAlias('worker', value)
			if (Array.isArray(value)) {
				return value.map((item) => normalizeDetailValue(key, item))
			}
			if (value && typeof value === 'object') {
				return Object.fromEntries(
					Object.keys(value)
						.sort()
						.map((childKey) => [
							childKey,
							normalizeDetailValue(childKey, value[childKey]),
						]),
				)
			}
			return value ?? null
		}
		const normalizeDetail = (detail) => {
			if (!detail || typeof detail !== 'object') return null
			return Object.fromEntries(
				Object.keys(detail)
					.sort()
					.map((key) => [key, normalizeDetailValue(key, detail[key])]),
			)
		}
		const detectBrowserFamily = () => {
			const ua = navigator.userAgent
			if (ua.includes('Firefox/')) return 'firefox'
			if (ua.includes('Edg/')) return 'edge'
			if (ua.includes('Chrome/') || ua.includes('Chromium/')) return 'chromium'
			if (ua.includes('Safari/')) return 'webkit'
			return 'unknown'
		}
		const detectWorkerComms = async () => {
			const caps = {
				crossOriginIsolated: !!globalThis.crossOriginIsolated,
				sabAvailable: false,
				opfsAvailable: false,
				webLocksAvailable: !!navigator.locks,
				broadcastChannelAvailable: typeof BroadcastChannel === 'function',
			}
			try {
				const buf = new SharedArrayBuffer(8)
				caps.sabAvailable = buf.byteLength === 8
			} catch {}
			try {
				if (navigator.storage?.getDirectory) {
					await navigator.storage.getDirectory()
					caps.opfsAvailable = true
				}
			} catch {}
			const config =
				!caps.crossOriginIsolated || !caps.sabAvailable ? 'A'
				: caps.opfsAvailable && caps.webLocksAvailable ? 'C'
				: 'B'
			return { config, caps }
		}
		const storageEstimate =
			navigator.storage?.estimate ?
				await navigator.storage.estimate().catch(() => null)
			: null
		const persisted =
			navigator.storage?.persisted ?
				await navigator.storage.persisted().catch(() => null)
			: null
		const startupMarks = performance
			.getEntriesByType('mark')
			.filter((entry) => entry.name.startsWith(startupPrefix))
			.map((entry, index) => {
				const detail = normalizeDetail(entry.detail)
				return {
					name: entry.name,
					label: entry.name.slice(startupPrefix.length),
					startTimeMs: roundMs(entry.startTime),
					collectionOrdinal: index + 1,
					sequence:
						typeof detail?.sequence === 'number' ? detail.sequence : null,
					detail,
					_sortStartTimeMs: entry.startTime,
				}
			})
			.sort(
				(a, b) =>
					a._sortStartTimeMs - b._sortStartTimeMs ||
					(a.sequence ?? Number.MAX_SAFE_INTEGER) -
						(b.sequence ?? Number.MAX_SAFE_INTEGER) ||
					a.collectionOrdinal - b.collectionOrdinal ||
					a.name.localeCompare(b.name),
			)
			.map((mark, index) => {
				const { _sortStartTimeMs: _, ...stableMark } = mark
				return {
					...stableMark,
					timelineOrdinal: index + 1,
				}
			})
		const labels = new Set(startupMarks.map((mark) => mark.label))
		const expectedStartupMarks = [
			'shell.entrypoint-loaded',
			'web-document.construct-start',
			'worker-comms.detected',
			'storage.mode-detected',
			'runtime.mode-selected',
			'runtime.worker-created',
			'service-worker.register-ready',
			'runtime.connected',
			'web-document.resume-ready',
			'worker.first-ready',
			'plugin.running',
		]
		const markMatches = (mark, label, predicate) =>
			mark.label === label && (!predicate || predicate(mark))
		const firstMark = (label, predicate) =>
			startupMarks.find((mark) => markMatches(mark, label, predicate)) ?? null
		const lastMark = (label, predicate) => {
			for (let i = startupMarks.length - 1; i >= 0; i--) {
				const mark = startupMarks[i]
				if (markMatches(mark, label, predicate)) return mark
			}
			return null
		}
		const isPluginMark = (mark) =>
			mark.detail?.plugin === true ||
			(typeof mark.detail?.workerId === 'string' &&
				mark.detail.workerId.startsWith('plugin/'))
		const lastPluginReady =
			lastMark('worker.ready', isPluginMark) ??
			lastMark('worker.first-ready', isPluginMark)
		const firstWorkerReady =
			firstMark('worker.first-ready') ??
			firstMark('worker.ready')
		const pluginRunning =
			firstMark('plugin.running', isPluginMark) ??
			lastPluginReady
		const firstPluginStart =
			firstMark('worker.first-create-start', isPluginMark) ??
			firstMark('worker.construct-start', isPluginMark)
		const shellEntrypoint = firstMark('shell.entrypoint-loaded')
		const shellBootRequested = firstMark('shell.boot-requested')
		const runtimeConnected = firstMark('runtime.connected')
		const nav = performance.getEntriesByType('navigation')[0]
		const navigation =
			nav ?
				{
					type: nav.type,
					startTimeMs: roundMs(nav.startTime),
					responseEndMs: roundMs(nav.responseEnd),
					domInteractiveMs: roundMs(nav.domInteractive),
					domContentLoadedEventEndMs: roundMs(nav.domContentLoadedEventEnd),
					loadEventEndMs: roundMs(nav.loadEventEnd),
				}
			: null
		const paint = performance.getEntriesByType('paint').map((entry) => ({
			name: entry.name,
			startTimeMs: roundMs(entry.startTime),
		}))
		const brands =
			navigator.userAgentData?.brands?.map((brand) => ({
				brand: brand.brand,
				version: brand.version,
			})) ?? []
		const quickstartTiming =
			globalThis.__s4waveQuickstartTiming ??
			globalThis.__s4wave_debug?.quickstartTiming ??
			null
		const makeSegment = (name, startMs, endMs, attribution, evidence) => ({
			name,
			startMs: startMs ?? null,
			endMs: endMs ?? null,
			elapsedMs:
				typeof startMs === 'number' && typeof endMs === 'number' ?
					Math.max(0, endMs - startMs)
				: null,
			attribution,
			evidence,
		})
		const makeReadinessMark = (name, timestampMs, evidence, sourceMark) => ({
			name,
			timestampMs: timestampMs ?? null,
			evidence,
			sourceMark: sourceMark ?? null,
		})
		const readinessTimeline = [
			makeReadinessMark(
				'worker-ready',
				firstWorkerReady?.startTimeMs,
				[firstWorkerReady?.label ?? 'worker.ready'],
				firstWorkerReady,
			),
			makeReadinessMark(
				'plugin-running',
				pluginRunning?.startTimeMs,
				[pluginRunning?.label ?? 'plugin.running'],
				pluginRunning,
			),
			makeReadinessMark(
				'progress-ready',
				quickstartTiming?.progressReadyMs,
				['quickstart.progressReadyMs'],
				null,
			),
			makeReadinessMark(
				'frame-ready',
				args.driveFrameReadyMs,
				['driveFrameReadyMs', "[data-testid='unixfs-browser']"],
				null,
			),
			makeReadinessMark(
				'content-ready',
				args.driveContentReadyMs,
				['driveContentReadyMs', 'getting-started.md'],
				null,
			),
			makeReadinessMark(
				'golden-path-ready',
				args.driveGoldenPathReadyMs,
				['driveGoldenPathReadyMs', "[data-testid='drive-welcome']", "[data-testid='drive-invite-cta']", 'Add User dialog'],
				null,
			),
		]
		const missingReadinessMarks = readinessTimeline
			.filter((mark) => typeof mark.timestampMs !== 'number')
			.map((mark) => mark.name)
		const startupAttributionSegments = [
			makeSegment(
				'navigation-to-entrypoint',
				navigation?.startTimeMs,
				shellEntrypoint?.startTimeMs,
				'HTML, static assets, hydration entrypoint load',
				['navigation.startTimeMs', 'shell.entrypoint-loaded'],
			),
			makeSegment(
				'entrypoint-to-runtime-connected',
				shellEntrypoint?.startTimeMs,
				runtimeConnected?.startTimeMs,
				'Bldr web document and runtime connection',
				['shell.entrypoint-loaded', 'runtime.connected'],
			),
			makeSegment(
				'boot-request-to-first-plugin-worker',
				shellBootRequested?.startTimeMs,
				firstPluginStart?.startTimeMs,
				'Live app boot before the first plugin worker is constructed',
				['shell.boot-requested', firstPluginStart?.label ?? 'worker.construct-start'],
			),
			makeSegment(
				'plugin-worker-startup',
				firstPluginStart?.startTimeMs,
				lastPluginReady?.startTimeMs,
				'Plugin worker construction and readiness',
				[firstPluginStart?.label ?? 'worker.construct-start', lastPluginReady?.label ?? 'worker.ready'],
			),
			makeSegment(
				'plugin-ready-to-quickstart-start',
				lastPluginReady?.startTimeMs,
				quickstartTiming?.startedMs,
				'App shell, root resource access, routing, and quickstart resource scheduling before createQuickstartSetup begins',
				[lastPluginReady?.label ?? 'worker.ready', 'quickstart.startedMs'],
			),
			makeSegment(
				'quickstart-progress-setup',
				quickstartTiming?.startedMs,
				quickstartTiming?.progressReadyMs,
				'createQuickstartSetup RPC flow through session, space, world, and Drive frame prerequisites',
				['quickstart.startedMs', 'quickstart.progressReadyMs'],
			),
			makeSegment(
				'quickstart-content-seed',
				quickstartTiming?.progressReadyMs,
				quickstartTiming?.finishedMs,
				'Quickstart-specific seed content population before final navigation',
				['quickstart.progressReadyMs', 'quickstart.finishedMs'],
			),
			makeSegment(
				'quickstart-finished-to-frame-ready',
				quickstartTiming?.finishedMs,
				args.driveFrameReadyMs,
				'Post-setup redirect, session mount, space mount, and Drive frame render',
				['quickstart.finishedMs', 'driveFrameReadyMs'],
			),
			makeSegment(
				'frame-ready-to-content-ready',
				args.driveFrameReadyMs,
				args.driveContentReadyMs,
				'Drive content watch and file list render',
				['driveFrameReadyMs', 'driveContentReadyMs'],
			),
		]
		const measuredSegments = startupAttributionSegments.filter(
			(segment) => typeof segment.elapsedMs === 'number',
		)
		const longestSegment =
			measuredSegments.length ?
				measuredSegments.reduce((longest, segment) =>
					segment.elapsedMs > longest.elapsedMs ? segment : longest,
				)
			: null
		const artifact = {
			schemaVersion: 7,
			scenario: 'quickstart-drive-production-smoke',
			collectedAt: new Date().toISOString(),
			baseURL: args.baseURL,
			finalURL: window.location.href,
			release: args.release,
			source: args.source,
			browser: {
				family: detectBrowserFamily(),
				harnessName: args.browserName,
				userAgent: navigator.userAgent,
				brands,
			},
			page: {
				visibilityState: document.visibilityState,
				focused: document.hasFocus(),
			},
			workerComms: await detectWorkerComms(),
			storage: {
				mode:
					navigator.storage?.getDirectory ?
						'browser-opfs-indexeddb'
					: 'browser-indexeddb',
				persistSupported: !!navigator.storage?.persist,
				persistedSupported: !!navigator.storage?.persisted,
				persisted,
				estimate: storageEstimate,
			},
			timing: {
				browserNowMs: roundMs(performance.now()),
				driveFrameReadyMs: args.driveFrameReadyMs,
				driveContentReadyMs: args.driveContentReadyMs,
				driveContentReadyError: args.driveContentReadyError || null,
				driveGoldenPathReadyMs: args.driveGoldenPathReadyMs,
				driveGoldenPathError: args.driveGoldenPathError || null,
				quickstart: quickstartTiming,
				navigation,
				paint,
			},
			timeline: {
				base: 'navigation.startTimeMs',
				unit: 'ms',
				ordering: [
					'performance.mark.startTime',
					'detail.sequence',
					'collectionOrdinal',
					'name',
				],
				normalizedVolatileDetailFields: [
					'documentId',
					'from',
					'path',
					'workerId',
				],
			},
			readiness: {
				startupPerformanceGate: 'frame-ready',
				contentCorrectnessTiming: 'content-ready',
				foregroundResumeTiming: 'web-document.resume-ready',
				frameReadyMs: args.driveFrameReadyMs,
				quickstartState: quickstartTiming?.state ?? null,
				progressReadyMs: quickstartTiming?.progressReadyMs ?? null,
				quickstartContentReadyMs: quickstartTiming?.contentReadyMs ?? null,
				contentReadyMs: args.driveContentReadyMs,
				contentReadyError: args.driveContentReadyError || null,
				goldenPathReadyMs: args.driveGoldenPathReadyMs,
				goldenPathError: args.driveGoldenPathError || null,
				workerReadyMs: firstWorkerReady?.startTimeMs ?? null,
				pluginRunningMs: pluginRunning?.startTimeMs ?? null,
				missingReadinessMarks,
				timeline: readinessTimeline,
				coldStart: {
					startupPerformanceGate: 'frame-ready',
					contentCorrectnessTiming: 'content-ready',
					frameReadyMs: args.driveFrameReadyMs,
					contentReadyMs: args.driveContentReadyMs,
					contentReadyError: args.driveContentReadyError || null,
					goldenPathReadyMs: args.driveGoldenPathReadyMs,
					goldenPathError: args.driveGoldenPathError || null,
					timeline: readinessTimeline,
				},
				foregroundResume: args.foregroundResume,
			},
			runtimeTrace: args.runtimeTrace,
			postLoadSharedObjectWorkload: args.postLoadSOWorkload,
			foregroundResume: args.foregroundResume,
			startupMarks,
			missingStartupMarks: expectedStartupMarks.filter((label) => !labels.has(label)),
			startupAttribution: {
				range: makeSegment(
					'last-plugin-ready-to-frame-ready',
					lastPluginReady?.startTimeMs,
					args.driveFrameReadyMs,
					'Previously unattributed post-plugin startup tail through the startup performance gate',
					[lastPluginReady?.label ?? 'worker.ready', 'driveFrameReadyMs'],
				),
				longestSegment,
				segments: startupAttributionSegments,
			},
		}
		return JSON.stringify(artifact, null, 2)
	}`, map[string]any{
		"baseURL":                testHarness.getBaseURL(),
		"browserName":            testHarness.browserName,
		"driveFrameReadyMs":      driveFrameReadyMs,
		"driveContentReadyMs":    driveContentReadyArg,
		"driveContentReadyError": driveContentReadyError,
		"driveGoldenPathReadyMs": driveGoldenPathReadyArg,
		"driveGoldenPathError":   driveGoldenPathError,
		"runtimeTrace":           runtimeTrace,
		"postLoadSOWorkload":     postLoadSOWorkload,
		"foregroundResume":       foregroundResume,
		"release": map[string]any{
			"generationId": desc.GenerationID,
			"shellAssets": map[string]any{
				"entrypoint":    desc.ShellAssets.Entrypoint,
				"serviceWorker": desc.ShellAssets.ServiceWorker,
				"sharedWorker":  desc.ShellAssets.SharedWorker,
				"wasm":          desc.ShellAssets.Wasm,
				"css":           desc.ShellAssets.CSS,
			},
			"prerenderedRoutes":    desc.PrerenderedRoutes,
			"requiredStaticAssets": desc.RequiredStaticAssets,
		},
		"source": source,
	})
	if err != nil {
		return nil, err
	}

	// Decode the serialized quickstart smoke artifact.
	data, ok := raw.(string)
	if !ok {
		return nil, errors.Errorf("unexpected artifact payload %T", raw)
	}
	return []byte(data + "\n"), nil
}

func writeQuickstartSmokeArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func setBoolJSONField(arena *fastjson.Arena, obj *fastjson.Value, key string, value bool) {
	if value {
		obj.Set(key, arena.NewTrue())
	} else {
		obj.Set(key, arena.NewFalse())
	}
}

func setRawJSONValue(arena *fastjson.Arena, obj *fastjson.Value, key string, value *fastjson.Value) {
	// Serialize an absent raw JSON value as null.
	if value == nil {
		obj.Set(key, arena.NewNull())
		return
	}

	// Attach the existing raw JSON value to the output object.
	obj.Set(key, value)
}

func sourceRevision(t testing.TB) map[string]any {
	// Read the checkout's current source revision.
	t.Helper()
	headCmd := exec.Command("git", "rev-parse", "HEAD")
	headRaw, err := headCmd.Output()
	if err != nil {
		t.Fatalf("read git HEAD: %v", err)
	}

	// Read the checkout's working-tree status.
	statusCmd := exec.Command("git", "status", "--short")
	statusRaw, err := statusCmd.Output()
	if err != nil {
		t.Fatalf("read git status: %v", err)
	}

	// Split the checkout status into the source artifact's change inventory.
	status := strings.TrimSpace(string(statusRaw))
	statusLines := []string{}
	if status != "" {
		statusLines = strings.Split(status, "\n")
	}
	return map[string]any{
		"head":        strings.TrimSpace(string(headRaw)),
		"dirty":       len(statusLines) != 0,
		"statusShort": statusLines,
	}
}

// waitForQuickstartDriveFrame waits for each boot gate before accepting the Drive frame.
func waitForQuickstartDriveFrame(t *testing.T, page playwright.Page, allowLiveApp bool, failure string) {
	// Require either the cold prerender or the already live presentation.
	t.Helper()
	if allowLiveApp {
		waitForPrerenderRootOrLiveApp(t, page)
	} else {
		waitForPrerenderRoot(t, page)
	}

	// Preserve the boot, route, intro, and final frame checks in their original order.
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	waitForQuickstartAppRoute(t, page)
	completeQuickstartDriveIntroIfPresent(t, page)
	if err := waitForDriveFrameReady(page, browserWaitMS); err != nil {
		dumpPageState(t, page)
		t.Fatalf(failure, err)
	}
}
