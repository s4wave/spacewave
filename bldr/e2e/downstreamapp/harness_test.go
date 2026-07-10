//go:build !js

package downstreamapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/gitroot"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/sirupsen/logrus"
)

func TestResolveBrowserCompiler(t *testing.T) {
	t.Setenv(legacyCompilerEnv, "")
	t.Setenv(CompilerEnv, "")
	got, err := ResolveBrowserCompiler()
	if err != nil {
		t.Fatalf("ResolveBrowserCompiler() error = %v", err)
	}
	if got != BrowserCompilerGoScript {
		t.Fatalf("ResolveBrowserCompiler() = %q, want %q", got, BrowserCompilerGoScript)
	}

	t.Setenv(CompilerEnv, "go")
	got, err = ResolveBrowserCompiler()
	if err != nil {
		t.Fatalf("ResolveBrowserCompiler(go) error = %v", err)
	}
	if got != BrowserCompilerGo {
		t.Fatalf("ResolveBrowserCompiler(go) = %q, want %q", got, BrowserCompilerGo)
	}

	t.Setenv(CompilerEnv, "wat")
	if _, err := ResolveBrowserCompiler(); err == nil {
		t.Fatal("ResolveBrowserCompiler(wat) error = nil, want unsupported compiler error")
	}
}

func TestResolveBrowserCompilerRejectsLegacySelector(t *testing.T) {
	t.Setenv(legacyCompilerEnv, "goscript")
	t.Setenv(CompilerEnv, "")
	_, err := ResolveBrowserCompiler()
	if err == nil {
		t.Fatal("ResolveBrowserCompiler() error = nil, want legacy selector error")
	}
	if !strings.Contains(err.Error(), legacyCompilerEnv) {
		t.Fatalf("ResolveBrowserCompiler() error = %v, want mention of %s", err, legacyCompilerEnv)
	}
}

func TestResolveWorkerMode(t *testing.T) {
	t.Setenv(WorkerModeEnv, "")
	got, err := ResolveWorkerMode()
	if err != nil {
		t.Fatalf("ResolveWorkerMode() error = %v", err)
	}
	if got != WorkerModeDedicated {
		t.Fatalf("ResolveWorkerMode() = %q, want %q", got, WorkerModeDedicated)
	}

	t.Setenv(WorkerModeEnv, "shared-worker")
	got, err = ResolveWorkerMode()
	if err != nil {
		t.Fatalf("ResolveWorkerMode(shared-worker) error = %v", err)
	}
	if got != WorkerModeShared {
		t.Fatalf("ResolveWorkerMode(shared-worker) = %q, want %q", got, WorkerModeShared)
	}

	t.Setenv(WorkerModeEnv, "workerpool")
	if _, err := ResolveWorkerMode(); err == nil {
		t.Fatal("ResolveWorkerMode(workerpool) error = nil, want unsupported worker mode")
	}
}

func TestRecordBrowserConsoleMessage(t *testing.T) {
	tests := []struct {
		name    string
		msgType string
		text    string
		want    bool
	}{
		{
			name:    "error",
			msgType: "error",
			text:    "failed to load module",
			want:    true,
		},
		{
			name:    "js plugin import",
			msgType: "log",
			text:    "shared-worker: starting plugin: /b/pd/web/plugin.mjs",
			want:    true,
		},
		{
			name:    "web plugin debug",
			msgType: "debug",
			text:    "web plugin status running=true",
			want:    true,
		},
		{
			name:    "runtime startup",
			msgType: "info",
			text:    "runtime-wasm: starting plugin host scheduler",
			want:    true,
		},
		{
			name:    "controller retry",
			msgType: "log",
			text:    "failed to execute devtool controller: plugin scheduler controller failed",
			want:    true,
		},
		{
			name:    "ordinary log",
			msgType: "log",
			text:    "service worker warmed cache",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recordBrowserConsoleMessage(tt.msgType, tt.text); got != tt.want {
				t.Fatalf("recordBrowserConsoleMessage(%q, %q) = %v, want %v", tt.msgType, tt.text, got, tt.want)
			}
		})
	}
}

func TestFixtureProjectConfig(t *testing.T) {
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	conf, err := loadFixtureProjectConfig(repoRoot)
	if err != nil {
		t.Fatalf("load fixture project config: %v", err)
	}
	if err := conf.Validate(); err != nil {
		t.Fatalf("fixture project config did not validate: %v", err)
	}

	for _, pluginID := range []string{"web", "downstream-core", "downstream-web"} {
		if conf.GetManifests()[pluginID] == nil {
			t.Fatalf("fixture manifest %q missing", pluginID)
		}
	}
	for _, forbidden := range []string{"spacewave-core", "spacewave-web", "spacewave-app"} {
		if conf.GetManifests()[forbidden] != nil {
			t.Fatalf("fixture unexpectedly includes product manifest %q", forbidden)
		}
	}
}

func TestEnableBrowserReleaseAutoStartPreservesGeneratedDescriptor(t *testing.T) {
	entryDir := t.TempDir()
	descriptorPath := filepath.Join(entryDir, "browser-release.json")
	const generated = `{
  "schemaVersion": 1,
  "generationId": "/sw-generated.mjs",
  "shellAssets": {
    "entrypoint": "/entrypoint/entrypoint.mjs",
    "serviceWorker": "/sw-generated.mjs",
    "sharedWorker": "/shw-generated.mjs",
    "wasm": "/entrypoint/runtime.wasm",
    "css": ["/entrypoint/app.css"]
  },
  "prerenderedRoutes": ["/"],
  "requiredStaticAssets": []
}`
	if err := os.WriteFile(descriptorPath, []byte(generated), 0o644); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}

	if err := enableBrowserReleaseAutoStart(entryDir); err != nil {
		t.Fatalf("enable auto-start: %v", err)
	}

	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatalf("read descriptor: %v", err)
	}
	var parser fastjson.Parser
	got, err := parser.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse descriptor: %v", err)
	}
	if !got.GetBool("autoStart") {
		t.Fatal("autoStart = false, want true")
	}
	if generationID := string(got.GetStringBytes("generationId")); generationID != "/sw-generated.mjs" {
		t.Fatalf("generationId = %q, want generated service worker", generationID)
	}
	if serviceWorker := string(got.GetStringBytes("shellAssets", "serviceWorker")); serviceWorker != "/sw-generated.mjs" {
		t.Fatalf("serviceWorker = %q, want generated service worker", serviceWorker)
	}
	if sharedWorker := string(got.GetStringBytes("shellAssets", "sharedWorker")); sharedWorker != "/shw-generated.mjs" {
		t.Fatalf("sharedWorker = %q, want generated shared worker", sharedWorker)
	}
	css := got.GetArray("shellAssets", "css")
	if len(css) != 1 || string(css[0].GetStringBytes()) != "/entrypoint/app.css" {
		t.Fatalf("css assets changed: %s", got.Get("shellAssets", "css"))
	}
}

func TestGoScriptDownstreamAppLoadsSonner(t *testing.T) {
	if os.Getenv(RunEnv) != "1" {
		t.Skipf("set %s=1 to run downstream browser e2e", RunEnv)
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	le := logrus.New().WithField("package", "bldr/e2e/downstreamapp")
	h, err := Boot(ctx, le)
	if err != nil {
		t.Fatalf("boot downstream harness: %v", err)
	}
	defer h.Release()

	if err := h.LaunchBrowser(); err != nil {
		t.Fatalf("launch chromium: %v", err)
	}

	browserCtx, page, err := h.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	defer browserCtx.Close()

	diag := &browserDiagnostics{}
	wireDiagnostics(browserCtx, page, diag)

	scenarioStart := time.Now()
	if _, err := page.Goto(h.BaseURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		t.Fatalf("load downstream app: %v\n%s", err, diag.String())
	}
	if err := page.GetByText("Downstream Sonner loaded through Bldr").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(180000),
	}); err != nil {
		t.Fatalf("wait for sonner toast text: %v\n%s\n%s", err, describePage(page), diag.String())
	}
	if err := page.GetByText("Can't open this object yet").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(180000),
	}); err != nil {
		t.Fatalf("wait for SDK fallback text: %v\n%s\n%s", err, describePage(page), diag.String())
	}

	probe, err := collectSonnerProbe(page, diag)
	if err != nil {
		t.Fatalf("collect sonner probe: %v\n%s", err, diag.String())
	}
	if probe.Status != 200 {
		t.Fatalf("sonner response status = %d, want 200\n%s", probe.Status, diag.String())
	}
	if probe.BodyLength == 0 {
		t.Fatalf("sonner response body was empty: %+v\n%s", probe, diag.String())
	}
	if !probe.BrowserObserved {
		t.Fatalf("browser resource timing did not observe sonner module\n%s", diag.String())
	}
	sdkProbe, err := collectSDKAppProbe(page)
	if err != nil {
		t.Fatalf("collect SDK app probe: %v\n%s\n%s", err, describePage(page), diag.String())
	}
	if !sdkProbe.SDKAppImport {
		t.Fatalf("SDK app import probe was false: %+v\n%s", sdkProbe, diag.String())
	}
	if sdkProbe.ProductViewerImplicit {
		t.Fatalf("downstream catalog unexpectedly included product viewer: %+v\n%s", sdkProbe, diag.String())
	}
	if sdkProbe.NonIndexRootMarker != "non-index-root-package" {
		t.Fatalf("non-index-root package bare import marker = %q, want fixture marker\n%s", sdkProbe.NonIndexRootMarker, diag.String())
	}
	if !slices.Contains(sdkProbe.CatalogComponentIDs, "spacewave.object-layout.viewer") ||
		!slices.Contains(sdkProbe.CatalogComponentIDs, "spacewave.debug.viewer") ||
		!slices.Contains(sdkProbe.CatalogComponentIDs, "mercury.note.viewer") {
		t.Fatalf("downstream catalog missing expected viewers: %+v\n%s", sdkProbe, diag.String())
	}
	for _, label := range []string{"Configured", "Loading", "Loaded", "Failed", "Retrying", "Removed", "Upgraded"} {
		if !slices.Contains(sdkProbe.LifecycleLabels, label) {
			t.Fatalf("downstream lifecycle labels missing %q: %+v\n%s", label, sdkProbe, diag.String())
		}
	}
	if !sdkProbe.FallbackRendered {
		t.Fatalf("missing-viewer fallback did not render: %+v\n%s", sdkProbe, diag.String())
	}
	diag.LogHTTP(t)
	diag.AssertNoHTTPFailures(t)

	t.Logf(
		"downstream harness timings: package_wall=%s boot_ready=%s scenario_ready=%s sonner_bytes=%d sonner_url=%s",
		time.Since(started).Round(time.Millisecond),
		h.BootTime().Round(time.Millisecond),
		time.Since(scenarioStart).Round(time.Millisecond),
		probe.BodyLength,
		probe.URL,
	)
}

func TestReleaseShapedEntrypointInteractiveBeforeIrrelevantManifestCompletes(t *testing.T) {
	if os.Getenv(RunEnv) != "1" {
		t.Skipf("set %s=1 to run downstream browser e2e", RunEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	const irrelevantPluginID = "downstream-e2e-irrelevant-manifest"
	blockedManifest := newBlockingManifestPreflight(irrelevantPluginID, []string{"web/js/wasm"})
	le := logrus.New().WithField("package", "bldr/e2e/downstreamapp")
	h, err := boot(ctx, le, bootConfig{
		minifyEntrypoint:         true,
		blockedManifestPreflight: blockedManifest,
	})
	if err != nil {
		t.Fatalf("boot release-shaped downstream harness: %v", err)
	}
	t.Cleanup(h.Release)
	if slices.Contains(h.projConfig.GetStart().GetPlugins(), irrelevantPluginID) {
		t.Fatalf("synthetic manifest %q unexpectedly appears in startPlugins", irrelevantPluginID)
	}
	for _, req := range h.startupManifestRequests() {
		if req.pluginID == irrelevantPluginID {
			t.Fatalf("synthetic manifest %q unexpectedly appears in required settle requests", irrelevantPluginID)
		}
	}
	t.Log("boot_shape=cold_release_shaped execute_web_wasm=true minify_entrypoint=true root_entry_build_type=RELEASE browser_release_auto_start=true dev_mode=true synthetic_not_in_start_plugins=true synthetic_not_in_required_settle=true")

	startedCtx, cancelStarted := context.WithTimeout(ctx, 30*time.Second)
	select {
	case <-blockedManifest.started:
		t.Logf("event_order=1 irrelevant_manifest_started plugin=%s", irrelevantPluginID)
	case <-startedCtx.Done():
		cancelStarted()
		t.Fatalf("wait for irrelevant manifest resolver start: %v", startedCtx.Err())
	}
	cancelStarted()
	select {
	case err := <-blockedManifest.completed:
		t.Fatalf("irrelevant Manifest completed before browser interaction: %v", err)
	default:
	}

	if err := h.LaunchBrowser(); err != nil {
		t.Fatalf("launch chromium: %v", err)
	}
	browserCtx, page, err := h.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	t.Cleanup(func() {
		_ = browserCtx.Close()
	})

	diag := &browserDiagnostics{}
	wireDiagnostics(browserCtx, page, diag)
	if _, err := page.Goto(h.BaseURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		t.Fatalf("load downstream app: %v\n%s", err, diag.String())
	}
	interaction := page.GetByTestId("downstream-startup-interaction").First()
	if err := interaction.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(180000),
	}); err != nil {
		t.Fatalf("wait for downstream interaction control: %v\n%s\n%s", err, describePage(page), diag.String())
	}
	if err := interaction.Click(); err != nil {
		t.Fatalf("click downstream interaction control: %v\n%s\n%s", err, describePage(page), diag.String())
	}
	if err := page.GetByText("Startup interactions: 1").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Fatalf("wait for interaction state change: %v\n%s\n%s", err, describePage(page), diag.String())
	}
	select {
	case err := <-blockedManifest.completed:
		t.Fatalf("irrelevant Manifest completed before interaction state changed: %v", err)
	default:
		t.Log("event_order=2 interactive_click_state_changed state=\"Startup interactions: 1\" irrelevant_manifest_completed=false")
	}

	blockedManifest.Release()
	t.Logf("event_order=3 irrelevant_manifest_released plugin=%s", irrelevantPluginID)
	completedCtx, cancelCompleted := context.WithTimeout(ctx, 30*time.Second)
	select {
	case err := <-blockedManifest.completed:
		if err != nil {
			t.Fatalf("irrelevant Manifest completed with error: %v", err)
		}
		t.Logf("event_order=4 irrelevant_manifest_completed plugin=%s", irrelevantPluginID)
	case <-completedCtx.Done():
		cancelCompleted()
		t.Fatalf("wait for irrelevant manifest resolver completion: %v", completedCtx.Err())
	}
	cancelCompleted()
}

type sdkAppProbe struct {
	SDKAppImport          bool
	CatalogComponentIDs   []string
	ProductViewerImplicit bool
	NonIndexRootMarker    string
	LifecycleLabels       []string
	FallbackRendered      bool
}

func collectSDKAppProbe(page playwright.Page) (sdkAppProbe, error) {
	for _, frame := range page.Frames() {
		value, err := frame.Evaluate(`() => {
  const state = window.__downstreamE2E
  return state ? JSON.stringify(state) : ""
}`)
		if err != nil || value == "" {
			continue
		}
		probe, err := parseSDKAppProbe([]byte(value.(string)))
		if err != nil {
			return sdkAppProbe{}, err
		}
		return probe, nil
	}
	return sdkAppProbe{}, errors.New("downstream SDK app probe was not published")
}

func parseSDKAppProbe(data []byte) (sdkAppProbe, error) {
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		return sdkAppProbe{}, err
	}
	return sdkAppProbe{
		SDKAppImport:          v.GetBool("sdkAppImport"),
		CatalogComponentIDs:   fastjsonStringSlice(v.GetArray("catalogComponentIDs")),
		ProductViewerImplicit: v.GetBool("productViewerImplicit"),
		NonIndexRootMarker:    string(v.GetStringBytes("nonIndexRootMarker")),
		LifecycleLabels:       fastjsonStringSlice(v.GetArray("lifecycleLabels")),
		FallbackRendered:      v.GetBool("fallbackRendered"),
	}, nil
}

func fastjsonStringSlice(values []*fastjson.Value) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value.GetStringBytes()))
	}
	return out
}

type browserDiagnostics struct {
	mu       sync.Mutex
	console  []string
	errors   []string
	requests []string
	failed   []string
	sonner   []playwright.Response
}

func wireDiagnostics(browserCtx playwright.BrowserContext, page playwright.Page, diag *browserDiagnostics) {
	recordConsole := func(source string, msg playwright.ConsoleMessage) {
		if !recordBrowserConsoleMessage(msg.Type(), msg.Text()) {
			return
		}
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.console = append(diag.console, source+" "+msg.Type()+": "+msg.Text())
	}
	browserCtx.OnConsole(func(msg playwright.ConsoleMessage) {
		source := "context"
		if worker, err := msg.Worker(); err == nil && worker != nil {
			source = "context worker " + worker.URL()
		}
		recordConsole(source, msg)
	})
	page.OnWorker(func(worker playwright.Worker) {
		diag.mu.Lock()
		diag.console = append(diag.console, "worker: "+worker.URL())
		diag.mu.Unlock()
		worker.OnConsole(func(msg playwright.ConsoleMessage) {
			recordConsole("worker", msg)
		})
	})
	page.On("pageerror", func(err error) {
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.errors = append(diag.errors, err.Error())
	})
	page.OnRequest(func(req playwright.Request) {
		url := req.URL()
		if !isHTTPURL(url) {
			return
		}
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.requests = append(diag.requests, "request "+req.Method()+" "+url)
	})
	page.OnRequestFailed(func(req playwright.Request) {
		url := req.URL()
		if !isHTTPURL(url) {
			return
		}
		failure := req.Failure().Error()
		msg := "request failed " + req.Method() + " " + url + ": " + failure
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.requests = append(diag.requests, msg)
		if !isBrowserAbortedRequest(failure) {
			diag.failed = append(diag.failed, msg)
		}
	})
	page.On("response", func(resp playwright.Response) {
		url := resp.URL()
		if strings.Contains(url, "sonner") {
			diag.mu.Lock()
			diag.sonner = append(diag.sonner, resp)
			diag.mu.Unlock()
		}
		if !isHTTPURL(url) {
			return
		}
		line := "response " + strconv.Itoa(resp.Status()) + " " + resp.StatusText() + " " + url
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.requests = append(diag.requests, line)
		if resp.Status() >= 400 {
			diag.failed = append(diag.failed, line)
		}
	})
}

func isHTTPURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func isBrowserAbortedRequest(failure string) bool {
	return strings.Contains(failure, "net::ERR_ABORTED")
}

func recordBrowserConsoleMessage(msgType, text string) bool {
	if msgType == "error" || msgType == "warning" {
		return true
	}
	for _, marker := range []string{
		"shared-worker: starting plugin:",
		"Starting Bldr JS plugin entrypoint",
		"runtime-goscript:",
		"runtime-wasm:",
		"starting plugin host scheduler",
		"plugin scheduler controller failed",
		"web plugin host is running",
		"web js plugin host is running",
		"loading startup plugin",
		"devtool controller attempt failed",
		"failed to execute devtool controller",
		"routine exited",
		"start websocket controller",
		"start fetch manifest via rpc controller",
		"Loading web plugin",
		"web plugin status",
		"Web plugin is not ready yet",
		"Processing ",
		"frontend entrypoint",
		"web view handlers",
		"web pkgs",
		"WebPluginBrowser:",
		"Starting web plugin for browser",
		"Web plugin for browser started",
		"WebDocument: registered web view",
		"WebView: set render mode",
		"WebView: set html links",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

type sonnerProbe struct {
	URL             string
	Status          int
	BodyLength      int
	Tail            string
	BrowserObserved bool
}

func collectSonnerProbe(page playwright.Page, diag *browserDiagnostics) (sonnerProbe, error) {
	observed := false
	for _, frame := range page.Frames() {
		value, err := frame.Evaluate(`() => {
  const state = window.__downstreamE2E || {}
  const entries = state.resources || performance.getEntriesByType('resource').map((entry) => entry.name)
  return state.ready === true && entries.some((name) => String(name).includes('sonner'))
}`)
		if err != nil {
			continue
		}
		if value == true {
			observed = true
			break
		}
	}

	diag.mu.Lock()
	responses := append([]playwright.Response(nil), diag.sonner...)
	diag.mu.Unlock()
	if len(responses) == 0 {
		return sonnerProbe{}, errors.New("no sonner response observed")
	}
	resp := responses[len(responses)-1]
	body, err := resp.Body()
	if err != nil {
		return sonnerProbe{}, err
	}
	tail := string(body)
	if len(tail) > 256 {
		tail = tail[len(tail)-256:]
	}
	return sonnerProbe{
		URL:             resp.URL(),
		Status:          resp.Status(),
		BodyLength:      len(body),
		Tail:            tail,
		BrowserObserved: observed,
	}, nil
}

func (d *browserDiagnostics) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.Join([]string{
		"console:\n" + strings.Join(d.console, "\n"),
		"errors:\n" + strings.Join(d.errors, "\n"),
		"http:\n" + strings.Join(d.requests, "\n"),
		"failed http:\n" + strings.Join(d.failed, "\n"),
	}, "\n\n")
}

func (d *browserDiagnostics) AssertNoHTTPFailures(t *testing.T) {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.failed) != 0 {
		t.Fatalf("browser HTTP failures:\n%s", strings.Join(d.failed, "\n"))
	}
}

func (d *browserDiagnostics) LogHTTP(t *testing.T) {
	t.Helper()

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, line := range d.requests {
		t.Log("browser http: " + line)
	}
}

func describePage(page playwright.Page) string {
	var parts []string
	if snapshot := describeBootSnapshot(page); snapshot != "" {
		parts = append(parts, "boot:\n"+snapshot)
	}
	if text, err := page.Locator("body").InnerText(playwright.LocatorInnerTextOptions{
		Timeout: playwright.Float(1000),
	}); err == nil {
		parts = append(parts, "body:\n"+text)
	}
	for i, frame := range page.Frames() {
		text, _ := frame.Evaluate(`() => document.body?.innerText || ""`)
		parts = append(parts, "frame["+strconv.Itoa(i)+"] "+frame.URL()+":\n"+text.(string))
	}
	return strings.Join(parts, "\n\n")
}

func describeBootSnapshot(page playwright.Page) string {
	value, err := page.Evaluate(`() => {
  const local = window.localStorage
  const session = window.sessionStorage
  const resourceEntries = performance.getEntriesByType('resource')
    .map((entry) => entry.name)
    .filter((name) => String(name).includes('/bldr-dev/') || String(name).includes('/b/'))
    .slice(-20)
  return JSON.stringify({
    href: window.location.href,
    localBootVersion: local.getItem('spacewave-browser-app-state-version'),
    sessionBootVersion: session.getItem('spacewave-browser-tab-state-version'),
    resetAttempt: session.getItem('spacewave-browser-app-state-reset-attempted'),
    hasSession: local.getItem('spacewave-has-session'),
    bootStatus: window.__swBootStatus || null,
    bootRecoveryStatus: window.__swBootRecoveryStatus || null,
    bootMarks: window.__swStartupMarks || null,
    downstreamState: window.__downstreamE2E || null,
    resourceEntries,
  }, null, 2)
}`)
	if err != nil {
		return "snapshot error: " + err.Error()
	}
	body, ok := value.(string)
	if !ok {
		return "snapshot marshal error: browser returned non-string snapshot"
	}
	return body
}
