//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

const (
	dedicatedFallbackFolder        = "dedicated-worker-fallback-proof"
	dedicatedMultiTabLeft          = "dedicated-worker-multitab-left"
	dedicatedMultiTabRight         = "dedicated-worker-multitab-right"
	dedicatedFailoverReloadCounter = "__dedicatedHostFailoverLoadCount"

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

	installDedicatedWorkerFailoverReloadCounter(t, sess)
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
	pluginAssetURL := dedicatedWorkerPluginAssetURL(t, leftPage)
	startDedicatedWorkerPluginAssetFetch(t, rightPage, pluginAssetURL)
	markDedicatedWorkerFailoverCloseStart(t, rightPage)
	if err := leftPage.Close(); err != nil {
		t.Fatalf("close elected DedicatedWorker host document: %v", err)
	}
	waitForDedicatedWorkerHostRole(t, rightPage, dedicatedHostRoleHost)
	WaitForDriveReady(t, h, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	assertDedicatedWorkerHostTopology(t, rightPage, dedicatedHostRoleHost)
	assertNoDedicatedWorkerFailoverReload(t, rightPage)
	assertDedicatedWorkerPluginAssetFetch(t, rightPage, pluginAssetURL)
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

// TestWebDocumentLivenessLockReleaseNoDeletion proves a liveness Web Lock grant
// in the runtime is suspect evidence while the document port stays open. The
// unit test pins the runtime-side suspect transition; this browser path releases
// and re-acquires the page-held lock, then checks that Go did not observe a
// remote-document teardown signal.
func TestWebDocumentLivenessLockReleaseNoDeletion(t *testing.T) {
	h := harness(t)
	skipDedicatedWorkerFallbackIfUnsupported(t, h)

	sess := h.NewCleanBlankSession(t)
	installWebDocumentLivenessLockReleaseControl(t, sess)
	console, stopConsole := sess.WatchConsole()
	var consoleMessages []string
	defer func() {
		stopConsole()
		consoleMessages = append(consoleMessages, drainConsoleMessages(console)...)
		report := crashReportFromMessages(consoleMessages)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during liveness release: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during liveness release: %+v", report)
		}
		assertNoRemoteDocumentDeletedLog(t, consoleMessages)
	}()

	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, sess.Page())
	ctx, cancel := context.WithCancel(h.ctx)
	t.Cleanup(cancel)
	if err := sess.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources: %v", err)
	}

	scenario := CreateDriveScenario(t, h, sess)
	page := scenario.GetSession().Page()
	WaitForDriveReady(t, h, page)
	AssertBrowserStartupDone(t, h, page)
	assertDedicatedWorkerHostTopology(t, page, dedicatedHostRoleHost)
	assertWebDocumentLivenessLockControlReady(t, page)
	released := releaseWebDocumentLivenessLock(t, page)
	t.Logf("released WebDocument liveness lock %q", released)
	assertWebDocumentLivenessLockRecovered(t, page)
	assertDriveRoute(t, page, scenario.GetSessionIndex(), scenario.GetSpaceID())
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
		messages := drainConsoleMessages(console)
		report := crashReportFromMessages(messages)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during %s: %+v", label, report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during %s: %+v", label, report)
		}
		for _, message := range messages {
			if strings.Contains(message, "dedicated runtime host role is attached") {
				t.Errorf("attached document started a private runtime client during %s: %s", label, message)
				break
			}
		}
	}
}

func installDedicatedWorkerFailoverReloadCounter(t testing.TB, sess *TestSession) {
	t.Helper()
	script := `(() => {
		const key = '` + dedicatedFailoverReloadCounter + `'
		const count = Number(sessionStorage.getItem(key) || '0') + 1
		sessionStorage.setItem(key, String(count))
		globalThis.__dedicatedHostFailoverLoadCount = count
	})()`
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install DedicatedWorker failover reload counter: %v", err)
	}
}

func installWebDocumentLivenessLockReleaseControl(t testing.TB, sess *TestSession) {
	t.Helper()
	script := `(() => {
		const control = {
			ready: false,
			released: false,
			recovered: false,
			controlledName: '',
			error: '',
			release: null,
		}
		globalThis.__bldrLivenessLockControl = control
		const locks = navigator.locks
		if (!locks || typeof locks.request !== 'function') {
			control.error = 'navigator.locks.request unavailable'
			return
		}
		const originalRequest = locks.request.bind(locks)
		locks.request = (name, options, callback) => {
			const lockName = String(name)
			if (lockName.startsWith('bldr-doc-') && !control.ready) {
				return originalRequest(name, options, (...args) => {
					control.ready = true
					control.controlledName = lockName
					return new Promise((resolve, reject) => {
						control.release = () => {
							if (control.released) return
							control.released = true
							originalRequest(lockName, {}, () => {
								control.recovered = true
								return new Promise(() => {})
							}).catch((err) => {
								control.error = String(err)
							})
							resolve(undefined)
						}
						try {
							Promise.resolve(callback(...args)).catch(reject)
						} catch (err) {
							reject(err)
						}
					})
				})
			}
			return originalRequest(name, options, callback)
		}
	})()`
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install WebDocument liveness release control: %v", err)
	}
}

func assertWebDocumentLivenessLockControlReady(t testing.TB, page playwright.Page) {
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const control = globalThis.__bldrLivenessLockControl
		return {
			present: !!control,
			ready: !!control?.ready,
			released: !!control?.released,
			hasRelease: typeof control?.release === 'function',
			controlledName: control?.controlledName ?? '',
			error: control?.error ?? '',
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read WebDocument liveness release control: %v", err)
	}
	proof, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected liveness release control proof %T: %#v", raw, raw)
	}
	if errMsg := stringField(proof, "error"); errMsg != "" {
		t.Fatalf("WebDocument liveness release control failed: %s; proof=%#v", errMsg, proof)
	}
	if !boolField(proof, "present") || !boolField(proof, "ready") || !boolField(proof, "hasRelease") {
		t.Fatalf("WebDocument liveness release control is not ready; proof=%#v", proof)
	}
	if name := stringField(proof, "controlledName"); !strings.HasPrefix(name, "bldr-doc-") {
		t.Fatalf("WebDocument liveness release control captured unexpected lock %q; proof=%#v", name, proof)
	}
}

func releaseWebDocumentLivenessLock(t testing.TB, page playwright.Page) string {
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const control = globalThis.__bldrLivenessLockControl
		if (!control) throw new Error('liveness release control missing')
		if (control.error) throw new Error(control.error)
		if (typeof control.release !== 'function') {
			throw new Error('liveness release function missing')
		}
		const name = control.controlledName || ''
		control.release()
		return name
	}`, nil)
	if err != nil {
		t.Fatalf("release WebDocument liveness lock: %v", err)
	}
	name, ok := raw.(string)
	if !ok || name == "" {
		t.Fatalf("unexpected released liveness lock name %T: %#v", raw, raw)
	}
	return name
}

func assertWebDocumentLivenessLockRecovered(t testing.TB, page playwright.Page) {
	t.Helper()
	_, err := page.WaitForFunction(`() => {
		const control = globalThis.__bldrLivenessLockControl
		if (control?.error) throw new Error(control.error)
		return !!control?.recovered
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(120000),
	})
	if err != nil {
		t.Fatalf("WebDocument liveness lock did not recover: %v", err)
	}
}

func drainConsoleMessages(messages <-chan string) []string {
	var out []string
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return out
			}
			out = append(out, msg)
		default:
			return out
		}
	}
}

func crashReportFromMessages(messages []string) CrashReport {
	var report CrashReport
	for _, msg := range messages {
		report.AddMessage(msg)
	}
	return report
}

func assertNoRemoteDocumentDeletedLog(t testing.TB, messages []string) {
	t.Helper()
	for _, msg := range messages {
		if strings.Contains(msg, "removed remote web document") {
			t.Fatalf("runtime deleted the remote web document after suspect lock release: %s", msg)
		}
	}
}

func dedicatedWorkerPluginAssetURL(t testing.TB, page playwright.Page) string {
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		const dispatch = marks.find((mark) =>
			mark.label === 'worker.create-dispatch-start' &&
			mark.detail?.plugin &&
			typeof mark.detail?.path === 'string' &&
			mark.detail.path
		)
		if (!dispatch) {
			throw new Error('plugin worker dispatch path not found')
		}
		return new URL(dispatch.detail.path, window.location.href).toString()
	}`, nil)
	if err != nil {
		t.Fatalf("read plugin asset URL: %v", err)
	}
	url, ok := raw.(string)
	if !ok || url == "" {
		t.Fatalf("unexpected plugin asset URL %T: %#v", raw, raw)
	}
	return url
}

func startDedicatedWorkerPluginAssetFetch(
	t testing.TB,
	page playwright.Page,
	url string,
) {
	t.Helper()
	if _, err := page.Evaluate(`(arg) => {
		const url = Array.isArray(arg) ? arg[0] : arg
		globalThis.__dedicatedHostFailoverPluginFetchSettledAt = null
		globalThis.__dedicatedHostFailoverPluginFetch = fetch(url, { cache: 'no-store' })
			.then(async (resp) => {
				const result = {
					url,
					ok: resp.ok,
					status: resp.status,
					body: (await resp.text()).slice(0, 120),
				}
				globalThis.__dedicatedHostFailoverPluginFetchSettledAt = performance.now()
				return result
			})
			.catch((err) => {
				globalThis.__dedicatedHostFailoverPluginFetchSettledAt = performance.now()
				return {
					url,
					error: err instanceof Error ? err.message : String(err),
				}
			})
		return true
	}`, []any{url}); err != nil {
		t.Fatalf("start in-flight plugin asset fetch: %v", err)
	}
}

func markDedicatedWorkerFailoverCloseStart(t testing.TB, page playwright.Page) {
	t.Helper()
	if _, err := page.Evaluate(`() => {
		globalThis.__dedicatedHostFailoverCloseStartedAt = performance.now()
		return true
	}`, nil); err != nil {
		t.Fatalf("mark DedicatedWorker failover close start: %v", err)
	}
}

func assertNoDedicatedWorkerFailoverReload(t testing.TB, page playwright.Page) {
	t.Helper()
	raw, err := page.Evaluate(`(arg) => {
		const key = Array.isArray(arg) ? arg[0] : arg
		return {
			count: Number(sessionStorage.getItem(key) || globalThis.__dedicatedHostFailoverLoadCount || 0),
			navigationTypes: performance.getEntriesByType('navigation').map((entry) => entry.type),
		}
	}`, []any{dedicatedFailoverReloadCounter})
	if err != nil {
		t.Fatalf("read DedicatedWorker failover reload counter: %v", err)
	}
	proof, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected reload counter proof %T: %#v", raw, raw)
	}
	if count := intField(proof, "count"); count != 1 {
		t.Fatalf("survivor document reloaded during DedicatedWorker failover; proof=%#v", proof)
	}
}

func assertDedicatedWorkerPluginAssetFetch(t testing.TB, page playwright.Page, url string) {
	t.Helper()
	raw, err := page.Evaluate(`async (arg) => {
		const wantURL = Array.isArray(arg) ? arg[0] : arg
		const promise = globalThis.__dedicatedHostFailoverPluginFetch
		if (!promise || typeof promise.then !== 'function') {
			throw new Error('plugin asset fetch promise not found')
		}
		const result = await promise
		return {
			...result,
			wantURL,
			closeStartedAt: globalThis.__dedicatedHostFailoverCloseStartedAt ?? null,
			settledAt: globalThis.__dedicatedHostFailoverPluginFetchSettledAt ?? null,
		}
	}`, []any{url})
	if err != nil {
		t.Fatalf("await in-flight plugin asset fetch: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected plugin asset fetch proof %T: %#v", raw, raw)
	}
	if intField(result, "settledAt") < intField(result, "closeStartedAt") {
		t.Fatalf("plugin asset fetch settled before host close; result=%#v", result)
	}
	if gotURL := stringField(result, "url"); gotURL != url {
		t.Fatalf("plugin asset fetch URL=%q want %q; result=%#v", gotURL, url, result)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("plugin asset fetch failed after DedicatedWorker failover: %s; result=%#v", errMsg, result)
	}
	if !boolField(result, "ok") || intField(result, "status") != 200 {
		t.Fatalf("plugin asset fetch did not return HTTP 200 after DedicatedWorker failover; result=%#v", result)
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
			if (mark.label === 'dedicated-host.lease-acquired' || mark.label === 'dedicated-host.promoted') {
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
			if (mark.label === 'dedicated-host.lease-acquired' || mark.label === 'dedicated-host.promoted') {
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
