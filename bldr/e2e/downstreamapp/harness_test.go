//go:build !js

package downstreamapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	playwright "github.com/playwright-community/playwright-go"
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
	var got struct {
		GenerationID string `json:"generationId"`
		AutoStart    bool   `json:"autoStart"`
		ShellAssets  struct {
			ServiceWorker string   `json:"serviceWorker"`
			SharedWorker  string   `json:"sharedWorker"`
			CSS           []string `json:"css"`
		} `json:"shellAssets"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal descriptor: %v", err)
	}
	if !got.AutoStart {
		t.Fatal("autoStart = false, want true")
	}
	if got.GenerationID != "/sw-generated.mjs" {
		t.Fatalf("generationId = %q, want generated service worker", got.GenerationID)
	}
	if got.ShellAssets.ServiceWorker != "/sw-generated.mjs" || got.ShellAssets.SharedWorker != "/shw-generated.mjs" {
		t.Fatalf("shell assets changed: %+v", got.ShellAssets)
	}
	if len(got.ShellAssets.CSS) != 1 || got.ShellAssets.CSS[0] != "/entrypoint/app.css" {
		t.Fatalf("css assets changed: %+v", got.ShellAssets.CSS)
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

	diag := newBrowserDiagnostics()
	wireDiagnostics(page, diag)

	scenarioStart := time.Now()
	if _, err := page.Goto(h.BaseURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		t.Fatalf("load downstream app: %v\n%s", err, diag.String())
	}
	if err := page.GetByText("Downstream Sonner loaded through Bldr").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(180000),
	}); err != nil {
		t.Fatalf("wait for sonner toast text: %v\n%s\n%s", err, describePage(page), diag.String())
	}

	probe, err := collectSonnerProbe(page, diag)
	if err != nil {
		t.Fatalf("collect sonner probe: %v\n%s", err, diag.String())
	}
	if probe.Status != 200 {
		t.Fatalf("sonner response status = %d, want 200\n%s", probe.Status, diag.String())
	}
	if probe.BodyLength == 0 || !strings.Contains(probe.Tail, "sourceMappingURL") && !strings.Contains(probe.Tail, "sonner") {
		t.Fatalf("sonner response body did not look complete: %+v\n%s", probe, diag.String())
	}
	if !probe.BrowserObserved {
		t.Fatalf("browser resource timing did not observe sonner module\n%s", diag.String())
	}

	t.Logf(
		"downstream harness timings: package_wall=%s boot_ready=%s scenario_ready=%s sonner_bytes=%d sonner_url=%s",
		time.Since(started).Round(time.Millisecond),
		h.BootTime().Round(time.Millisecond),
		time.Since(scenarioStart).Round(time.Millisecond),
		probe.BodyLength,
		probe.URL,
	)
}

type browserDiagnostics struct {
	mu       sync.Mutex
	console  []string
	errors   []string
	requests []string
	sonner   []playwright.Response
}

func newBrowserDiagnostics() *browserDiagnostics { return &browserDiagnostics{} }

func wireDiagnostics(page playwright.Page, diag *browserDiagnostics) {
	page.On("console", func(msg playwright.ConsoleMessage) {
		if msg.Type() != "error" && msg.Type() != "warning" {
			return
		}
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.console = append(diag.console, msg.Type()+": "+msg.Text())
	})
	page.On("pageerror", func(err error) {
		diag.mu.Lock()
		defer diag.mu.Unlock()
		diag.errors = append(diag.errors, err.Error())
	})
	page.On("response", func(resp playwright.Response) {
		url := resp.URL()
		if strings.Contains(url, "sonner") {
			diag.mu.Lock()
			diag.sonner = append(diag.sonner, resp)
			diag.mu.Unlock()
		}
		if resp.Status() >= 400 || strings.Contains(url, "sonner") {
			diag.mu.Lock()
			defer diag.mu.Unlock()
			diag.requests = append(diag.requests, resp.StatusText()+" "+url)
		}
	})
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
	for i := len(responses) - 1; i >= 0; i-- {
		resp := responses[i]
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
	return sonnerProbe{}, errors.New("no sonner response observed")
}

func (d *browserDiagnostics) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.Join([]string{
		"console:\n" + strings.Join(d.console, "\n"),
		"errors:\n" + strings.Join(d.errors, "\n"),
		"requests:\n" + strings.Join(d.requests, "\n"),
	}, "\n\n")
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
  return {
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
  }
}`)
	if err != nil {
		return "snapshot error: " + err.Error()
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "snapshot marshal error: " + err.Error()
	}
	return string(body)
}
