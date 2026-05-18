//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/sirupsen/logrus"
)

var testHarness *harness

const browserWaitMS = 420000
const foregroundResumeReadyRecordMS = 10000
const quickstartContentReadyRecordMS = 60000
const quickstartPostLoadSOOperationCount = 25
const quickstartPostLoadSOWorkloadTimeoutMS = 120000

// TIER: nightly
func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	h, err := boot(ctx, le)
	if err != nil {
		le.WithError(err).Fatal("boot release wasm harness")
	}
	testHarness = h

	code := m.Run()
	h.release(le)
	os.Exit(code)
}

func TestBrowserReleaseDescriptorIncludesPrerenderedWasmShell(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if desc.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", desc.SchemaVersion)
	}
	if desc.GenerationID == "" {
		t.Fatal("expected generation id")
	}
	if desc.ShellAssets.Entrypoint == "" {
		t.Fatal("expected shellAssets.entrypoint")
	}
	if desc.ShellAssets.ServiceWorker == "" {
		t.Fatal("expected shellAssets.serviceWorker")
	}
	if desc.ShellAssets.SharedWorker == "" {
		t.Fatal("expected shellAssets.sharedWorker")
	}
	if desc.ShellAssets.Wasm == "" {
		t.Fatal("expected shellAssets.wasm")
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/") {
		t.Fatalf("expected / in prerendered routes: %v", desc.PrerenderedRoutes)
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/quickstart/drive") {
		t.Fatalf("expected /quickstart/drive in prerendered routes: %v", desc.PrerenderedRoutes)
	}
}

func TestRootPrerenderLoadsProductionWasmBundle(t *testing.T) {
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err := page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)
}

func TestProductionRuntimeMatchesReleaseDescriptor(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)

	raw, err := page.Evaluate(`async () => {
		const registration = await navigator.serviceWorker.ready
		return {
			generationId: globalThis.__swGenerationId || '',
			controllerURL: navigator.serviceWorker.controller?.scriptURL || '',
			activeURL: registration.active?.scriptURL || '',
		}
	}`)
	if err != nil {
		t.Fatalf("read production runtime state: %v", err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected production runtime state %T", raw)
	}
	generationID, _ := state["generationId"].(string)
	if generationID != desc.GenerationID {
		t.Fatalf("generation id=%q want %q", generationID, desc.GenerationID)
	}
	controllerURL, _ := state["controllerURL"].(string)
	if !strings.HasSuffix(controllerURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("controller service worker=%q want suffix %q", controllerURL, desc.ShellAssets.ServiceWorker)
	}
	activeURL, _ := state["activeURL"].(string)
	if !strings.HasSuffix(activeURL, "/"+desc.ShellAssets.ServiceWorker) {
		t.Fatalf("active service worker=%q want suffix %q", activeURL, desc.ShellAssets.ServiceWorker)
	}
}

func TestQuickstartPrerenderAutoBootsProductionWasmBundle(t *testing.T) {
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := testHarness.quickstartSmokeArtifactPath(t)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove previous quickstart smoke artifact: %v", err)
	}
	source := sourceRevision(t)
	page := testHarness.newPage(t)
	traceCapture := beginQuickstartRuntimeTrace(t, page)
	defer traceCapture.cleanup(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}
	enableQuickstartTimingLogs(t, page)

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	waitForCanonicalQuickstartURL(t, page)
	err = page.Locator("[data-testid='unixfs-browser']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart frame-ready: %v", err)
	}
	driveFrameReadyMs := browserNowMs(t, page)
	driveContentReadyMs, driveContentReadyError := waitForQuickstartDriveContentReady(t, page)
	if driveContentReadyError != "" {
		t.Logf("quickstart content-ready not reached: %s", driveContentReadyError)
	}
	postLoadSOWorkload := runQuickstartPostLoadSOWorkload(t, page, driveContentReadyMs != nil)
	foregroundResume := collectForegroundResumeEvidence(t, page)
	logQuickstartTiming(t, page)
	runtimeTrace := traceCapture.stop(t)

	data, err := collectQuickstartSmokeArtifact(page, desc, source, driveFrameReadyMs, driveContentReadyMs, driveContentReadyError, runtimeTrace, postLoadSOWorkload, foregroundResume)
	if err != nil {
		t.Fatalf("collect quickstart smoke artifact: %v", err)
	}
	if err := writeQuickstartSmokeArtifact(path, data); err != nil {
		t.Fatalf("write quickstart smoke artifact: %v", err)
	}
	t.Logf("quickstart smoke artifact written to %s (%d bytes)", path, len(data))
}

type quickstartRuntimeTraceCapture struct {
	started bool
	stopped bool
	info    map[string]any
}

func beginQuickstartRuntimeTrace(t *testing.T, page playwright.Page) *quickstartRuntimeTraceCapture {
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
	if testHarness.browserName != "chromium" {
		info["skippedReason"] = "Chromium tracing is only available for the chromium release WASM browser"
		return c
	}
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
	c.started = true
	info["captured"] = true
	info["startedBefore"] = "page.goto('/quickstart/drive')"
	return c
}

func (c *quickstartRuntimeTraceCapture) stop(t *testing.T) map[string]any {
	t.Helper()

	if !c.started || c.stopped {
		return c.info
	}
	data, err := testHarness.browser.StopTracing()
	c.stopped = true
	if err != nil {
		t.Fatalf("stop quickstart runtime trace: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty quickstart runtime trace")
	}
	c.info["bytes"] = len(data)
	c.info["stoppedAfter"] = "foreground resume probe"
	t.Logf("quickstart runtime trace written to %s (%d bytes)", c.info["path"], len(data))
	return c.info
}

func (c *quickstartRuntimeTraceCapture) cleanup(t *testing.T) {
	t.Helper()

	if !c.started || c.stopped {
		return
	}
	if _, err := testHarness.browser.StopTracing(); err != nil {
		t.Logf("stop abandoned quickstart runtime trace: %v", err)
	}
	c.stopped = true
}

func waitForCanonicalQuickstartURL(t *testing.T, page playwright.Page) {
	t.Helper()

	expectedURL := testHarness.getBaseURL() + "/#/quickstart/drive"
	if page.URL() == expectedURL {
		return
	}
	err := page.WaitForURL(expectedURL, playwright.PageWaitForURLOptions{
		Timeout:   playwright.Float(30000),
		WaitUntil: playwright.WaitUntilStateCommit,
	})
	if err != nil {
		t.Fatalf("wait for canonical quickstart URL: %v", err)
	}
}

func waitForLiveApp(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		await Promise.race([
			globalThis.__swReady,
			new Promise((_, reject) => setTimeout(() => reject(new Error('runtime did not become ready')), 30000)),
		])
		const deadline = performance.now() + 30000
		while (document.querySelector('#bldr-root')?.hasAttribute('data-prerendered')) {
			if (performance.now() > deadline) {
				throw new Error('prerender did not switch to live app')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return true
	}`)
	if err != nil {
		t.Fatalf("wait for live app: %v", err)
	}
}

func waitForPrerenderRoot(t *testing.T, page playwright.Page) {
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

func waitForBootFunction(t *testing.T, page playwright.Page) {
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

func dumpPageState(t *testing.T, page playwright.Page) {
	t.Helper()

	state, err := page.Evaluate(`() => {
		const startupPrefix = 'spacewave.startup.'
		const state = {
			href: window.location.href,
			hash: window.location.hash,
			pathname: window.location.pathname,
			title: document.title,
			text: document.body?.innerText?.slice(0, 4000) ?? '',
			rootHtml: document.querySelector('#bldr-root')?.outerHTML?.slice(0, 4000) ?? '',
			hasDebugRoot: !!globalThis.__s4wave_debug?.root,
			quickstartTiming:
				globalThis.__s4waveQuickstartTiming ??
				globalThis.__s4wave_debug?.quickstartTiming ??
				null,
			testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
				testid: el.getAttribute('data-testid'),
				text: el.textContent?.slice(0, 200) ?? '',
			})),
			startupMarks: performance
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
	t.Logf("page state: %s", state)
}

func enableQuickstartTimingLogs(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`() => {
		globalThis.__s4waveLogQuickstartTiming = true
	}`)
	if err != nil {
		t.Fatalf("enable quickstart timing logs: %v", err)
	}
}

func logQuickstartTiming(t *testing.T, page playwright.Page) {
	t.Helper()

	timing, err := page.Evaluate(`() => JSON.stringify(globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null)`)
	if err != nil {
		t.Logf("quickstart timing: %v", err)
		return
	}
	t.Logf("quickstart timing: %v", timing)
}

func waitForQuickstartDriveContentReady(t *testing.T, page playwright.Page) (*int, string) {
	t.Helper()

	err := page.Locator("text=getting-started.md").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(quickstartContentReadyRecordMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		return nil, err.Error()
	}
	driveContentReadyMs := browserNowMs(t, page)
	return &driveContentReadyMs, ""
}

func runQuickstartPostLoadSOWorkload(t *testing.T, page playwright.Page, contentReady bool) map[string]any {
	t.Helper()

	if !contentReady {
		return map[string]any{
			"scenario":      "quickstart-post-load-shared-object-throughput",
			"skipped":       true,
			"skippedReason": "drive content-ready was not reached",
		}
	}

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
	t.Helper()

	before, err := webDocumentResumeReadySnapshot(page)
	if err != nil {
		return map[string]any{
			"scenario":      "quickstart-drive-foreground-resume",
			"skipped":       true,
			"skippedReason": "read initial WebDocument resume readiness: " + err.Error(),
		}
	}
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
	evidence["before"] = before
	evidence["hidden"] = hiddenState
	evidence["hiddenAtMs"] = hiddenAtMs
	t.Logf("foreground resume evidence: %#v", evidence)
	return evidence
}

func webDocumentResumeReadySnapshot(page playwright.Page) (map[string]any, error) {
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

func waitForDocumentHiddenState(t *testing.T, page playwright.Page, hidden bool, timeout time.Duration) (bool, int, map[string]any) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var snapshot map[string]any
	for {
		raw, err := page.Evaluate(`() => ({
			visibilityState: document.visibilityState,
			hidden: document.hidden,
			focused: document.hasFocus(),
			browserNowMs: Math.round(performance.now()),
		})`)
		if err == nil {
			if next, ok := raw.(map[string]any); ok {
				snapshot = next
				if got, _ := next["hidden"].(bool); got == hidden {
					return true, browserNowFromSnapshot(next), next
				}
			}
		}
		if time.Now().After(deadline) {
			return false, 0, snapshot
		}
		time.Sleep(50 * time.Millisecond)
	}
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
	t.Helper()

	raw, err := page.Evaluate(`() => Math.round(performance.now())`)
	if err != nil {
		t.Fatalf("read browser performance.now: %v", err)
	}
	val, ok := raw.(int)
	if ok {
		return val
	}
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
	runtimeTrace map[string]any,
	postLoadSOWorkload map[string]any,
	foregroundResume map[string]any,
) ([]byte, error) {
	var driveContentReadyArg any
	if driveContentReadyMs != nil {
		driveContentReadyArg = *driveContentReadyMs
	}
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

func sourceRevision(t testing.TB) map[string]any {
	t.Helper()

	headCmd := exec.Command("git", "rev-parse", "HEAD")
	headRaw, err := headCmd.Output()
	if err != nil {
		t.Fatalf("read git HEAD: %v", err)
	}
	statusCmd := exec.Command("git", "status", "--short")
	statusRaw, err := statusCmd.Output()
	if err != nil {
		t.Fatalf("read git status: %v", err)
	}
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
