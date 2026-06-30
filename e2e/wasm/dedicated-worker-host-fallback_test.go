//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

const (
	dedicatedFallbackFolder = "dedicated-worker-fallback-proof"
	dedicatedMultiTabLeft   = "dedicated-worker-multitab-left"
	dedicatedMultiTabRight  = "dedicated-worker-multitab-right"

	dedicatedHostRoleAttached = "attached"
	dedicatedHostRoleHost     = "host"
)

// TestDedicatedWorkerHostFallback proves the temporary Chromium 528332884
// fallback keeps a one-tab Drive session on DedicatedWorker hosting while the
// browser still exposes SharedWorker, direct OPFS, and persisted Drive state.
func TestDedicatedWorkerHostFallback(t *testing.T) {
	h := harness(t)
	skipDedicatedWorkerFallbackIfUnsupported(t, h)

	sess := h.NewCleanSession(t)
	stopConsole := watchFallbackConsole(t, sess, "one-tab DedicatedWorker fallback")
	defer stopConsole()

	scenario := CreateDriveScenario(t, h, sess)
	page := scenario.GetSession().Page()
	ready := WaitForDriveReady(t, h, page)
	AssertQuickstartContentAfterProgress(t, ready)
	AssertBrowserStartupDone(t, h, page)
	assertDedicatedWorkerHostTopology(t, page, dedicatedHostRoleHost)
	assertDirectOpfsMarkers(t, page)

	createDriveFolder(t, page, dedicatedFallbackFolder)
	targetHash, err := currentHash(page.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}

	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current context: %v", err)
	}
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/"+targetHash); err != nil {
		t.Fatalf("reload drive route in same browser context: %v", err)
	}
	page = sess.Page()
	WaitForApp(t, page)
	WaitForDriveReady(t, h, page)
	AssertBrowserStartupDone(t, h, page)
	assertDedicatedWorkerHostTopology(t, page, dedicatedHostRoleHost)
	assertDirectOpfsMarkers(t, page)
	waitForDriveEntry(t, page, dedicatedFallbackFolder)
	assertDriveRoute(t, page, scenario.GetSessionIndex(), scenario.GetSpaceID())
}

// TestDedicatedWorkerHostMultiTab proves two documents in one browser profile
// use one elected DedicatedWorker host generation, then fail over to a survivor
// after the elected host closes without corrupting OPFS-backed Drive state.
func TestDedicatedWorkerHostMultiTab(t *testing.T) {
	h := harness(t)
	skipDedicatedWorkerFallbackIfUnsupported(t, h)

	sess := h.NewCleanSession(t)
	stopConsole := watchFallbackConsole(t, sess, "multi-tab DedicatedWorker fallback")
	defer stopConsole()

	scenario := CreateDriveScenario(t, h, sess)
	leftPage := scenario.GetSession().Page()
	WaitForDriveReady(t, h, leftPage)
	AssertBrowserStartupDone(t, h, leftPage)
	assertDedicatedWorkerHostTopology(t, leftPage, dedicatedHostRoleHost)
	createDriveFolder(t, leftPage, dedicatedMultiTabLeft)
	targetHash, err := currentHash(leftPage.URL())
	if err != nil {
		t.Fatalf("current drive hash: %v", err)
	}

	rightPage, err := h.newBrowserPage(sess)
	if err != nil {
		t.Fatalf("open second app document: %v", err)
	}
	h.registerPageSession(rightPage, sess)
	defer func() {
		h.unregisterPageSession(rightPage)
		if err := rightPage.Close(); err != nil {
			t.Errorf("close second app document: %v", err)
		}
	}()

	loadPageURL(t, rightPage, h.BaseURL()+"/"+targetHash)
	WaitForApp(t, rightPage)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	assertDedicatedWorkerHostTopology(t, rightPage, dedicatedHostRoleAttached)
	waitForDriveEntry(t, rightPage, dedicatedMultiTabLeft)
	if err := leftPage.Close(); err != nil {
		t.Fatalf("close elected DedicatedWorker host document: %v", err)
	}
	waitForDedicatedWorkerHostRole(t, rightPage, dedicatedHostRoleHost)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	assertDedicatedWorkerHostTopology(t, rightPage, dedicatedHostRoleHost)
	assertDirectOpfsMarkers(t, rightPage)
	waitForDriveEntry(t, rightPage, dedicatedMultiTabLeft)
	createDriveFolder(t, rightPage, dedicatedMultiTabRight)
	waitForDriveEntry(t, rightPage, dedicatedMultiTabRight)

	reloadPage(t, rightPage)
	WaitForApp(t, rightPage)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	waitForDriveEntry(t, rightPage, dedicatedMultiTabLeft)
	waitForDriveEntry(t, rightPage, dedicatedMultiTabRight)
	assertDedicatedWorkerHostTopology(t, rightPage, dedicatedHostRoleHost)
	assertDriveRoute(t, rightPage, scenario.GetSessionIndex(), scenario.GetSpaceID())
}

func skipDedicatedWorkerFallbackIfUnsupported(t testing.TB, h *Harness) {
	t.Helper()
	if h.BrowserName() != "chromium" {
		t.Skipf("DedicatedWorker fallback proof is Chromium-only; browser=%s", h.BrowserName())
	}
}

func watchFallbackConsole(t testing.TB, sess *TestSession, label string) func() {
	t.Helper()
	console, stopConsole := sess.WatchConsole()
	return func() {
		stopConsole()
		report := DrainCrashReport(console)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during %s: %+v", label, report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during %s: %+v", label, report)
		}
	}
}

func assertDedicatedWorkerHostTopology(
	t testing.TB,
	page playwright.Page,
	expectedRole string,
) map[string]any {
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		let runtimeMode = null
		let runtimeDocumentId = null
		let dedicatedHostRole = null
		let dedicatedHostGeneration = null
		let dedicatedHostAttachReady = false
		for (const mark of marks) {
			if (mark.label === 'runtime.mode-selected') {
				runtimeMode = mark.detail?.mode ?? null
				runtimeDocumentId = mark.detail?.documentId ?? null
			}
			if (mark.label === 'dedicated-host.lease-acquired') {
				dedicatedHostRole = 'host'
				dedicatedHostGeneration = mark.detail?.generation ?? null
			}
			if (mark.label === 'dedicated-host.attach-selected') {
				dedicatedHostRole = 'attached'
			}
			if (mark.label === 'dedicated-host.attach-open-ready') {
				dedicatedHostAttachReady = true
			}
		}
		const pluginDispatches = marks
			.filter((mark) => mark.label === 'worker.create-dispatch-start' && mark.detail?.plugin)
			.map((mark) => ({
				workerId: mark.detail?.workerId ?? null,
				shared: mark.detail?.shared ?? null,
				detectConfig: mark.detail?.detectConfig ?? null,
				workerMode: mark.detail?.workerMode ?? null,
				path: mark.detail?.path ?? null,
			}))
		const opfsBridgeMarks = marks
			.filter((mark) => mark.label === 'runtime.opfs-bridge-ready')
			.map((mark) => ({
				workerId: mark.detail?.workerId ?? null,
				documentId: mark.detail?.documentId ?? null,
				runtimeId: mark.detail?.runtimeId ?? null,
				enabled: mark.detail?.enabled ?? null,
			}))
		return {
			crossOriginIsolated: !!globalThis.crossOriginIsolated,
			sharedWorkerType: typeof SharedWorker,
			opfsGetDirectoryType: typeof navigator.storage?.getDirectory,
			runtimeMode,
			runtimeDocumentId,
			dedicatedHostRole,
			dedicatedHostGeneration,
			dedicatedHostAttachReady,
			pluginDispatches,
			opfsBridgeEnabled: opfsBridgeMarks.some((mark) => mark.enabled === true),
			opfsBridgeMarks,
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read DedicatedWorker fallback topology: %v", err)
	}
	proof, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected DedicatedWorker topology %T: %#v", raw, raw)
	}
	if got := stringField(proof, "sharedWorkerType"); got != "function" {
		t.Fatalf("SharedWorker type=%q want function; topology=%#v", got, proof)
	}
	if !boolField(proof, "crossOriginIsolated") {
		t.Fatalf("page is not cross-origin isolated; topology=%#v", proof)
	}
	if got := stringField(proof, "opfsGetDirectoryType"); got != "function" {
		t.Fatalf("navigator.storage.getDirectory type=%q want function; topology=%#v", got, proof)
	}
	if got := stringField(proof, "runtimeMode"); got != "dedicated-worker" {
		t.Fatalf("runtime mode=%q want dedicated-worker; topology=%#v", got, proof)
	}
	if got := stringField(proof, "dedicatedHostRole"); got != expectedRole {
		t.Fatalf("dedicated host role=%q want %q; topology=%#v", got, expectedRole, proof)
	}
	if boolField(proof, "opfsBridgeEnabled") {
		t.Fatalf("DedicatedWorker fallback unexpectedly enabled the SharedWorker OPFS bridge; topology=%#v", proof)
	}
	dispatches, ok := proof["pluginDispatches"].([]any)
	if !ok {
		t.Fatalf("unexpected plugin dispatches %T: %#v", proof["pluginDispatches"], proof)
	}
	switch expectedRole {
	case dedicatedHostRoleHost:
		if stringField(proof, "dedicatedHostGeneration") == "" {
			t.Fatalf("host page did not record a DedicatedWorker generation; topology=%#v", proof)
		}
		if len(dispatches) == 0 {
			t.Fatalf("host page recorded no plugin worker dispatch marks; topology=%#v", proof)
		}
	case dedicatedHostRoleAttached:
		if !boolField(proof, "dedicatedHostAttachReady") {
			t.Fatalf("attached page did not open through the elected host; topology=%#v", proof)
		}
		if len(dispatches) != 0 {
			t.Fatalf("attached page created duplicate plugin workers; topology=%#v", proof)
		}
	default:
		t.Fatalf("unknown DedicatedWorker host role assertion: %q", expectedRole)
	}
	for _, item := range dispatches {
		dispatch, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("unexpected plugin dispatch %T: %#v", item, item)
		}
		if boolField(dispatch, "shared") {
			t.Fatalf("plugin worker dispatch used SharedWorker under fallback; dispatch=%#v topology=%#v", dispatch, proof)
		}
	}
	return proof
}

func waitForDedicatedWorkerHostRole(
	t testing.TB,
	page playwright.Page,
	expectedRole string,
) {
	t.Helper()
	_, err := page.WaitForFunction(`(arg) => {
		const expectedRole = Array.isArray(arg) ? arg[0] : arg
		const marks = globalThis.__swStartupMarks ?? []
		let role = null
		let attachReady = false
		let hostGeneration = null
		let hostDispatches = 0
		for (const mark of marks) {
			if (mark.label === 'dedicated-host.lease-acquired') {
				role = 'host'
				hostGeneration = mark.detail?.generation ?? null
			}
			if (mark.label === 'dedicated-host.attach-selected') {
				role = 'attached'
			}
			if (mark.label === 'dedicated-host.attach-open-ready') {
				attachReady = true
			}
			if (mark.label === 'worker.create-dispatch-start' && mark.detail?.plugin) {
				hostDispatches++
			}
		}
		if (expectedRole === 'attached') {
			return role === 'attached' && attachReady
		}
		return role === 'host' && hostGeneration && hostDispatches > 0
	}`, []any{expectedRole}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(120000),
	})
	if err != nil {
		t.Fatalf("wait for DedicatedWorker host role %q: %v", expectedRole, err)
	}
}

func assertDirectOpfsMarkers(t testing.TB, page playwright.Page) []string {
	t.Helper()
	markers := listOpfsFormatMarkers(t, page)
	if len(markers) == 0 {
		t.Fatal("expected a direct page OPFS format marker after Drive readiness")
	}
	return markers
}

func loadPageURL(t testing.TB, page playwright.Page, targetURL string) {
	t.Helper()
	waitUntil := playwright.WaitUntilStateDomcontentloaded
	timeout := float64(120000)
	resp, err := page.Goto(targetURL, playwright.PageGotoOptions{
		WaitUntil: waitUntil,
		Timeout:   &timeout,
	})
	if err != nil {
		t.Fatalf("load app URL %s: %v", targetURL, err)
	}
	if resp != nil && resp.Status() >= 400 {
		t.Fatalf("app URL %s returned HTTP %d", targetURL, resp.Status())
	}
}

func reloadPage(t testing.TB, page playwright.Page) {
	t.Helper()
	waitUntil := playwright.WaitUntilStateDomcontentloaded
	timeout := float64(120000)
	if _, err := page.Reload(playwright.PageReloadOptions{
		WaitUntil: waitUntil,
		Timeout:   &timeout,
	}); err != nil {
		t.Fatalf("reload app page: %v", err)
	}
}
