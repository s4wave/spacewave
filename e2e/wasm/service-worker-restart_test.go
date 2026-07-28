//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

const (
	serviceWorkerRestartPreFolder     = "service-worker-restart-before"
	serviceWorkerRestartPostFolder    = "service-worker-restart-after"
	serviceWorkerRestartReloadCounter = "__serviceWorkerRestartLoadCount"
	serviceWorkerScriptPath           = "/sw.mjs"
)

// TestServiceWorkerRestartRecoversBldrWebHarness proves the ServiceWorker can
// stop mid-session and recover the runtime fetch relay without reloading tabs.
func TestServiceWorkerRestartRecoversBldrWebHarness(t *testing.T) {
	h := harness(t)
	if h.BrowserName() != "chromium" {
		t.Skipf("ServiceWorker.stopWorker is a Chromium CDP control; browser=%s", h.BrowserName())
	}

	sess := h.NewCleanBlankSession(t)
	installServiceWorkerRestartReloadCounter(t, sess)
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, sess.Page())
	ctx, cancel := context.WithCancel(h.ctx)
	t.Cleanup(cancel)
	if err := sess.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources: %v", err)
	}

	console, stopConsole := sess.WatchConsole()
	consoleCollector := startBackgroundThrottleConsoleCollector(console)

	scenario := CreateDriveScenario(t, h, sess)
	leftPage := scenario.GetSession().Page()
	WaitForDriveReady(t, h, leftPage)
	AssertBrowserStartupDone(t, h, leftPage)
	leftLoadCount := serviceWorkerRestartLoadCount(t, leftPage)
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
	assertServiceWorkerRestartCrossTabConfig(t, leftPage)
	assertServiceWorkerRestartCrossTabConfig(t, rightPage)
	AssertBrowserStartupDone(t, h, rightPage)
	rightLoadCount := serviceWorkerRestartLoadCount(t, rightPage)

	createDriveFolder(t, leftPage, serviceWorkerRestartPreFolder)
	waitForDriveEntry(t, rightPage, serviceWorkerRestartPreFolder)
	pluginAssetURL := serviceWorkerRestartPluginAssetURL(t, leftPage)
	assertPluginAssetFetchSucceeds(t, leftPage, pluginAssetURL, "before ServiceWorker restart")

	control := newServiceWorkerRestartCDPControl(t, rightPage)
	defer control.Close()
	active := control.WaitForRunning(t)
	stopped := control.Stop(t, active)
	control.Drain()
	assertPluginAssetFetchSucceeds(t, rightPage, pluginAssetURL, "after ServiceWorker restart")
	restarted := control.WaitForRunning(t)
	if restarted.VersionID != stopped.VersionID {
		t.Fatalf("ServiceWorker restarted unexpected version: stopped=%#v running=%#v", stopped, restarted)
	}
	assertServiceWorkerRestartLoadCount(t, leftPage, leftLoadCount)
	assertServiceWorkerRestartLoadCount(t, rightPage, rightLoadCount)
	createDriveFolder(t, rightPage, serviceWorkerRestartPostFolder)
	waitForDriveEntry(t, leftPage, serviceWorkerRestartPostFolder)
	waitForDriveEntry(t, rightPage, serviceWorkerRestartPreFolder)
	waitForDriveEntry(t, rightPage, serviceWorkerRestartPostFolder)

	stopConsole()
	consoleCollector.Wait()
	messages := consoleCollector.Messages()
	assertNoBackgroundThrottleConsoleFailures(t, messages)
	assertNoRemoteDocumentDeletedLog(t, messages)
}

type serviceWorkerRestartCDPControl struct {
	session playwright.CDPSession
	updates chan []serviceWorkerRestartVersion
}

type serviceWorkerRestartVersion struct {
	VersionID     string
	ScriptURL     string
	Status        string
	RunningStatus string
}

func newServiceWorkerRestartCDPControl(t testing.TB, page playwright.Page) *serviceWorkerRestartCDPControl {
	t.Helper()

	session, err := page.Context().NewCDPSession(page)
	if err != nil {
		t.Fatalf("new ServiceWorker CDP session: %v", err)
	}
	control := &serviceWorkerRestartCDPControl{
		session: session,
		updates: make(chan []serviceWorkerRestartVersion, 16),
	}
	session.On("ServiceWorker.workerVersionUpdated", func(raw any) {
		versions := parseServiceWorkerRestartVersions(raw)
		if len(versions) == 0 {
			return
		}
		select {
		case control.updates <- versions:
		default:
		}
	})
	if _, err := session.Send("ServiceWorker.enable", nil); err != nil {
		session.Detach()
		t.Fatalf("enable ServiceWorker CDP domain: %v", err)
	}
	return control
}

func (c *serviceWorkerRestartCDPControl) Close() {
	_ = c.session.Detach()
}

func (c *serviceWorkerRestartCDPControl) Drain() {
	for {
		select {
		case <-c.updates:
		default:
			return
		}
	}
}

func (c *serviceWorkerRestartCDPControl) WaitForRunning(t testing.TB) serviceWorkerRestartVersion {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	return c.waitForVersion(ctx, t, func(version serviceWorkerRestartVersion) bool {
		return version.isHarnessServiceWorker() && version.RunningStatus == "running"
	}, "running ServiceWorker version")
}

func (c *serviceWorkerRestartCDPControl) Stop(
	t testing.TB,
	version serviceWorkerRestartVersion,
) serviceWorkerRestartVersion {
	t.Helper()

	if _, err := c.session.Send("ServiceWorker.stopWorker", map[string]any{
		"versionId": version.VersionID,
	}); err != nil {
		t.Fatalf("stop ServiceWorker version %s: %v", version.VersionID, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	return c.waitForVersion(ctx, t, func(candidate serviceWorkerRestartVersion) bool {
		return candidate.VersionID == version.VersionID && candidate.RunningStatus == "stopped"
	}, "stopped ServiceWorker version")
}

func (c *serviceWorkerRestartCDPControl) waitForVersion(
	ctx context.Context,
	t testing.TB,
	match func(serviceWorkerRestartVersion) bool,
	description string,
) serviceWorkerRestartVersion {
	t.Helper()

	for {
		select {
		case versions := <-c.updates:
			for _, version := range versions {
				if match(version) {
					return version
				}
			}
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", description, ctx.Err())
		}
	}
}

func (v serviceWorkerRestartVersion) isHarnessServiceWorker() bool {
	return v.VersionID != "" &&
		v.Status == "activated" &&
		strings.HasSuffix(v.ScriptURL, serviceWorkerScriptPath)
}

func parseServiceWorkerRestartVersions(raw any) []serviceWorkerRestartVersion {
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	rawVersions, ok := payload["versions"].([]any)
	if !ok {
		return nil
	}
	versions := make([]serviceWorkerRestartVersion, 0, len(rawVersions))
	for _, rawVersion := range rawVersions {
		versionMap, ok := rawVersion.(map[string]any)
		if !ok {
			continue
		}
		versions = append(versions, serviceWorkerRestartVersion{
			VersionID:     stringField(versionMap, "versionId"),
			ScriptURL:     stringField(versionMap, "scriptURL"),
			Status:        stringField(versionMap, "status"),
			RunningStatus: stringField(versionMap, "runningStatus"),
		})
	}
	return versions
}

func installServiceWorkerRestartReloadCounter(t testing.TB, sess *TestSession) {
	t.Helper()

	script := `(() => {
		const key = '` + serviceWorkerRestartReloadCounter + `'
		const count = Number(sessionStorage.getItem(key) || '0') + 1
		sessionStorage.setItem(key, String(count))
		globalThis.__serviceWorkerRestartLoadCount = count
	})()`
	if err := sess.BrowserContext().AddInitScript(playwright.Script{Content: &script}); err != nil {
		t.Fatalf("install ServiceWorker restart reload counter: %v", err)
	}
}

func serviceWorkerRestartLoadCount(t testing.TB, page playwright.Page) int {
	t.Helper()

	raw, err := page.Evaluate(`() => globalThis.__serviceWorkerRestartLoadCount ?? 0`, nil)
	if err != nil {
		t.Fatalf("read ServiceWorker restart load count: %v", err)
	}
	return intFromBrowserNumber(raw)
}

func assertServiceWorkerRestartLoadCount(t testing.TB, page playwright.Page, expected int) {
	t.Helper()

	if got := serviceWorkerRestartLoadCount(t, page); got != expected {
		t.Fatalf("page load count changed from %d to %d", expected, got)
	}
}

func assertServiceWorkerRestartCrossTabConfig(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`() => {
		const mark = (globalThis.__swStartupMarks ?? []).find((item) =>
			item.label === 'worker-comms.detected'
		)
		return mark?.detail ?? null
	}`, nil)
	if err != nil {
		t.Fatalf("read worker comms config: %v", err)
	}
	detail, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("worker comms config mark missing: %#v", raw)
	}
	config := stringField(detail, "config")
	if config != "B" && config != "C" {
		t.Fatalf("worker comms config %q does not use cross-tab MessagePorts; detail=%#v", config, detail)
	}
}

func serviceWorkerRestartPluginAssetURL(t testing.TB, page playwright.Page) string {
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

// assertPluginAssetFetchSucceeds fetches a plugin asset through the runtime
// relay and asserts it returns HTTP 200. Every test that disturbs the relay,
// by restarting the ServiceWorker or by failing the DedicatedWorker host over
// to a survivor document, uses this to prove the relay serves again afterwards.
func assertPluginAssetFetchSucceeds(
	t testing.TB,
	page playwright.Page,
	url string,
	label string,
) {
	t.Helper()

	raw, err := page.Evaluate(`async (arg) => {
		const [url, label] = Array.isArray(arg) ? arg : [arg.url, arg.label]
		const startedAt = performance.now()
		try {
			const resp = await fetch(url, { cache: 'no-store' })
			return {
				label,
				url,
				ok: resp.ok,
				status: resp.status,
				body: (await resp.text()).slice(0, 120),
				startedAt,
				settledAt: performance.now(),
			}
		} catch (err) {
			return {
				label,
				url,
				error: err instanceof Error ? err.message : String(err),
				startedAt,
				settledAt: performance.now(),
			}
		}
	}`, []any{url, label})
	if err != nil {
		t.Fatalf("fetch plugin asset %s: %v", label, err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected plugin asset fetch proof %T: %#v", raw, raw)
	}
	if gotURL := stringField(result, "url"); gotURL != url {
		t.Fatalf("plugin asset fetch URL=%q want %q; result=%#v", gotURL, url, result)
	}
	if errMsg := stringField(result, "error"); errMsg != "" {
		t.Fatalf("plugin asset fetch failed %s: %s; result=%#v", label, errMsg, result)
	}
	if !boolField(result, "ok") || intField(result, "status") != 200 {
		t.Fatalf("plugin asset fetch did not return HTTP 200 %s; result=%#v", label, result)
	}
}

func intFromBrowserNumber(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
