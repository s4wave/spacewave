//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	"github.com/sirupsen/logrus"
)

type releaseWorldConfigValues struct {
	spaceID string
	cdnBase string
}

const cliTerminalTextExpression = `(() => {
	const terminal = document.querySelector('.xterm')
	if (!terminal) return ''
	const parts = []
	const pushText = (node) => {
		const text = node?.textContent ?? ''
		if (text) parts.push(text)
	}
	pushText(terminal.querySelector('.xterm-accessibility-tree'))
	pushText(terminal.querySelector('.live-region'))
	pushText(terminal.querySelector('.xterm-rows'))
	return parts.join('\n').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim()
})()`

var testHarness *harness

const (
	browserWaitMS                         = 420000
	foregroundResumeReadyRecordMS         = 10000
	quickstartContentReadyRecordMS        = 60000
	quickstartPostLoadSOOperationCount    = 25
	quickstartPostLoadSOWorkloadTimeoutMS = 120000
)

// TIER: nightly
func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	if err := applyReleaseStartupTraceEnv(); err != nil {
		le.WithError(err).Fatal("apply release wasm startup trace env")
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

func TestLaunchPostPrerenderAssets(t *testing.T) {
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/blog/2026/04/launch"); err != nil {
		t.Fatalf("goto launch post: %v", err)
	}

	raw, err := page.Evaluate(`async () => {
		const heading = document.querySelector('.blog-prose h2')
		const list = document.querySelector('.blog-prose ul')
		const paragraph = document.querySelector('.blog-prose p')
		const avatar = document.querySelector('img[alt="Christian Stewart"]')
		const signoff = [...document.querySelectorAll('.blog-prose p')].find(
			(element) => element.textContent?.includes('Thanks for checking out Spacewave!'),
		)
		if (!heading || !list || !paragraph || !avatar || !signoff) {
			throw new Error('launch post contract elements are missing')
		}
		if (!avatar.complete) {
			await new Promise((resolve) => {
				avatar.addEventListener('load', resolve, { once: true })
				avatar.addEventListener('error', resolve, { once: true })
			})
		}

		const headingStyle = getComputedStyle(heading)
		const listStyle = getComputedStyle(list)
		const paragraphStyle = getComputedStyle(paragraph)
		return {
			linkedHydrateCss: [...document.querySelectorAll('link[rel="stylesheet"]')].some(
				(link) => new URL(link.href).pathname.startsWith('/static/assets/hydrate-'),
			),
			headingFontSize: headingStyle.fontSize,
			headingFontWeight: headingStyle.fontWeight,
			headingMarginTop: headingStyle.marginTop,
			headingMarginBottom: headingStyle.marginBottom,
			listStyleType: listStyle.listStyleType,
			listPaddingLeft: listStyle.paddingLeft,
			paragraphLineHeight: paragraphStyle.lineHeight,
			paragraphMarginBottom: paragraphStyle.marginBottom,
			avatarLoaded: avatar.complete && avatar.naturalWidth > 0 && avatar.naturalHeight > 0,
			avatarSameOrigin: new URL(avatar.currentSrc || avatar.src).origin === location.origin,
			signoffHasHardBreaks: signoff.querySelectorAll('br').length === 2,
		}
	}`)
	if err != nil {
		t.Fatalf("inspect launch post: %v", err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected launch post state %T: %#v", raw, raw)
	}

	for _, key := range []string{"linkedHydrateCss", "avatarLoaded", "avatarSameOrigin", "signoffHasHardBreaks"} {
		if !releaseBoolField(state, key) {
			t.Errorf("expected %s: %#v", key, state)
		}
	}
	for key, want := range map[string]string{
		"headingFontSize":       "24px",
		"headingFontWeight":     "600",
		"headingMarginTop":      "32px",
		"headingMarginBottom":   "12px",
		"listStyleType":         "disc",
		"listPaddingLeft":       "24px",
		"paragraphLineHeight":   "28px",
		"paragraphMarginBottom": "20px",
	} {
		if got := releaseStringField(state, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
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

func TestGoScriptDedicatedWorkerLocalBundleSmoke(t *testing.T) {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript dedicated-worker release smoke", E2EReleaseWasmGoScriptEnv)
	}

	page := testHarness.newDedicatedWorkerPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}
	waitForLiveApp(t, page)
	assertRuntimeWorkerMode(t, page, "dedicated-worker")
}

func TestGoScriptServiceWorkerPluginDistModuleIntegrity(t *testing.T) {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript ServiceWorker plugin dist module probe", E2EReleaseWasmGoScriptEnv)
	}

	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-core",
		"plugin/spacewave-launcher",
	})
	t.Log("drive gate: wait for quickstart route")
	waitForQuickstartAppRoute(t, page)
	t.Log("drive gate: complete intro if present")
	completeQuickstartDriveIntroIfPresent(t, page)
	t.Log("drive gate: wait for unixfs browser frame")
	err = page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	)
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart drive frame: %v", err)
	}
	t.Log("drive gate: wait for content ready")
	if _, quickstartErr := waitForQuickstartDriveContentReady(t, page); quickstartErr != "" {
		t.Fatalf("wait for quickstart drive content ready: %s", quickstartErr)
	}
	t.Log("drive gate: exercise golden path")
	if _, quickstartErr := exerciseQuickstartDriveGoldenPath(t, page); quickstartErr != "" {
		t.Fatalf("exercise quickstart drive golden path: %s", quickstartErr)
	}

	raw, err := page.Evaluate(`async (args) => {
		await navigator.serviceWorker.ready
		const controllerURL = navigator.serviceWorker.controller?.scriptURL || ''
		if (!controllerURL) {
			throw new Error('page is not controlled by the release ServiceWorker')
		}

		const hasDefaultExport = (text) =>
			/\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}\s*;?\s*$/.test(text) ||
			/\bexport\s+default\b/.test(text)
		const failures = []
		const results = []
		const recordFailure = (result, reason) => {
			failures.push({ ...result, reason })
		}

		for (let round = 0; round < args.rounds; round++) {
			for (const path of args.paths) {
				const requestURL =
					path +
					(path.includes('?') ? '&' : '?') +
					'sw_module_integrity=' +
					round +
					'-' +
					Date.now()
				const response = await fetch(requestURL, { cache: 'reload' })
				const text = await response.text()
				const result = {
					path,
					requestURL,
					round,
					status: response.status,
					ok: response.ok,
					contentType: response.headers.get('content-type') ?? '',
					contentLength: response.headers.get('content-length') ?? '',
					bodyLength: text.length,
					hasDefaultExport: hasDefaultExport(text),
					head: text.slice(0, 120),
					tail: text.slice(-180),
				}
				results.push(result)
				if (!response.ok) {
					recordFailure(result, 'non-OK response')
					continue
				}
				if (text.length < args.minBodyLength) {
					recordFailure(result, 'body shorter than expected minimum')
				}
				if (/^\s*</.test(text)) {
					recordFailure(result, 'response looks like HTML instead of JavaScript')
				}
				if (!result.hasDefaultExport) {
					recordFailure(result, 'module body has no default export shape')
				}
			}
		}

		if (failures.length) {
			throw new Error(
				'ServiceWorker plugin dist module integrity probe failed: ' +
					JSON.stringify(
						{
							controllerURL,
							failures,
							resultCount: results.length,
						},
						null,
						2,
					),
			)
		}
		return { controllerURL, results }
	}`, map[string]any{
		"paths": []string{
			"/b/pd/spacewave-core/spacewave-core.mjs",
			"/b/pd/spacewave-launcher/spacewave-launcher.mjs",
		},
		"rounds":        3,
		"minBodyLength": 1024 * 1024,
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("probe ServiceWorker plugin dist module integrity: %v", err)
	}
	t.Logf("ServiceWorker plugin dist module integrity probe: %#v", raw)
}

func TestBrowserReleaseLazyPluginRemoteSupplyAndDurableRestart(t *testing.T) {
	if os.Getenv("E2E_RELEASE_WASM_LAZY_PLUGIN_FIXTURE") != "1" {
		t.Skip("set E2E_RELEASE_WASM_LAZY_PLUGIN_FIXTURE=1 to run the release-world lazy-plugin fixture")
	}

	releaseWorld, err := releaseWorldFixtureConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	releasePackPrefix := strings.TrimRight(releaseWorld.cdnBase, "/") + "/" + releaseWorld.spaceID + "/packs/"
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range desc.RequiredStaticAssets {
		if strings.Contains(asset, "spacewave-cli-plugin") {
			t.Fatalf("lazy fixture descriptor embeds CLI plugin asset %q", asset)
		}
	}

	page, mutePageDiagnostics := testHarness.newPageWithDiagnosticsControl(t)
	ctx := page.Context()
	type releaseWorldRequest struct {
		url         string
		rangeHeader string
	}
	var requestsMu sync.Mutex
	var releaseWorldRequests []releaseWorldRequest
	var routeAbortErrors []error
	ctx.OnRequest(func(req playwright.Request) {
		if !strings.HasPrefix(req.URL(), releasePackPrefix) {
			return
		}
		rangeHeader, _ := req.HeaderValue("Range")
		requestsMu.Lock()
		releaseWorldRequests = append(releaseWorldRequests, releaseWorldRequest{url: req.URL(), rangeHeader: rangeHeader})
		requestsMu.Unlock()
	})
	abortPackRoute := func(route playwright.Route) {
		if err := route.Abort(); err != nil {
			requestsMu.Lock()
			routeAbortErrors = append(routeAbortErrors, errors.Wrapf(err, "abort Release World pack request %s", route.Request().URL()))
			requestsMu.Unlock()
		}
	}

	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto lazy-plugin fixture root: %v", err)
	}
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	if _, err := page.Evaluate(`() => globalThis.__swBoot('#/')`); err != nil {
		t.Fatalf("boot lazy-plugin fixture: %v", err)
	}
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-cli-plugin",
	})
	waitForCliTerminalPrompt(t, page)
	waitForPluginManifestCopyDone(t, page, "spacewave-cli-plugin")

	requestsMu.Lock()
	firstRequests := slices.Clone(releaseWorldRequests)
	requestsMu.Unlock()
	firstRangeCount := 0
	for _, request := range firstRequests {
		if request.rangeHeader != "" {
			firstRangeCount++
		}
	}
	if firstRangeCount == 0 {
		t.Fatal("lazy plugin became ready without a Release World CDN Range request")
	}

	mutePageDiagnostics()
	if err := page.Close(); err != nil {
		t.Fatalf("close first lazy-plugin fixture page: %v", err)
	}

	restartPage, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("create lazy-plugin restart page: %v", err)
	}
	muteRestartPageDiagnostics := testHarness.attachPageDiagnostics(t, restartPage)
	t.Cleanup(func() {
		muteRestartPageDiagnostics()
		_ = restartPage.Close()
	})
	cdp, err := ctx.NewCDPSession(restartPage)
	if err != nil {
		t.Fatalf("create restart CDP session: %v", err)
	}
	if _, err := cdp.Send("Network.clearBrowserCache", nil); err != nil {
		t.Fatalf("clear restart browser HTTP cache: %v", err)
	}
	if err := ctx.Route(releasePackPrefix+"**/*.kvf", abortPackRoute); err != nil {
		t.Fatalf("abort Release World pack requests on restart: %v", err)
	}
	if _, err := restartPage.Goto(testHarness.getBaseURL() + "/"); err != nil {
		dumpPageState(t, restartPage)
		t.Fatalf("durable local restart failed with Release World pack requests aborted: %v", err)
	}
	waitForPrerenderRoot(t, restartPage)
	waitForBootFunction(t, restartPage)
	if _, err := restartPage.Evaluate(`() => globalThis.__swBoot('#/')`); err != nil {
		t.Fatalf("boot lazy-plugin fixture after Release World pack route: %v", err)
	}
	waitForLiveApp(t, restartPage)
	waitForPluginWorkersRunning(t, restartPage, []string{
		"plugin/spacewave-cli-plugin",
	})
	waitForCliTerminalPrompt(t, restartPage)

	requestsMu.Lock()
	restartRequests := slices.Clone(releaseWorldRequests)
	routeErrors := slices.Clone(routeAbortErrors)
	requestsMu.Unlock()
	if len(routeErrors) != 0 {
		t.Fatalf("abort Release World pack request route failed: %v", routeErrors)
	}
	if len(restartRequests) != len(firstRequests) {
		t.Fatalf(
			"lazy plugin restart attempted %d additional exact Release World CDN requests; local durable cache proof failed",
			len(restartRequests)-len(firstRequests),
		)
	}
	muteRestartPageDiagnostics()
	if err := restartPage.Close(); err != nil {
		t.Fatalf("close restart lazy-plugin fixture page: %v", err)
	}
	freshContext, err := testHarness.browser.NewContext(testHarness.newContextOptions(t))
	if err != nil {
		t.Fatalf("create fresh quickstart browser context: %v", err)
	}
	t.Cleanup(func() {
		if err := freshContext.Close(); err != nil {
			t.Logf("close fresh quickstart browser context: %v", err)
		}
	})
	freshPage, err := freshContext.NewPage()
	if err != nil {
		t.Fatalf("create fresh quickstart page: %v", err)
	}
	muteFreshPageDiagnostics := testHarness.attachPageDiagnostics(t, freshPage)
	if _, err := freshPage.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		dumpPageState(t, freshPage)
		t.Fatalf("goto fresh quickstart drive: %v", err)
	}
	waitForPrerenderRoot(t, freshPage)
	waitForBootFunction(t, freshPage)
	waitForLiveApp(t, freshPage)
	waitForQuickstartAppRoute(t, freshPage)
	completeQuickstartDriveIntroIfPresent(t, freshPage)
	if err := freshPage.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, freshPage)
		t.Fatalf("wait for fresh quickstart frame-ready: %v", err)
	}
	if _, err := waitForQuickstartDriveContentReady(t, freshPage); err != "" {
		dumpPageState(t, freshPage)
		t.Fatalf("fresh quickstart Drive content-ready failed: %s", err)
	}
	muteFreshPageDiagnostics()
	if err := freshPage.Close(); err != nil {
		t.Fatalf("close fresh quickstart page: %v", err)
	}
}

func releaseWorldFixtureConfig(t *testing.T) (releaseWorldConfigValues, error) {
	t.Helper()
	result, err := bldr_project_starlark.Evaluate(filepath.Join(testHarness.repoRoot, "bldr.star"))
	if err != nil {
		return releaseWorldConfigValues{}, err
	}
	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		return releaseWorldConfigValues{}, errors.New("missing release-web-lazy-plugin-fixture build")
	}
	launcherOverride := build.GetManifestOverrides()["spacewave-launcher"]
	if launcherOverride == nil {
		return releaseWorldConfigValues{}, errors.New("missing lazy fixture launcher override")
	}
	var launcherConf bldr_plugin_compiler_go.Config
	if err := launcherConf.UnmarshalJSON(launcherOverride.GetConfig()); err != nil {
		return releaseWorldConfigValues{}, errors.Wrap(err, "decode lazy fixture launcher config")
	}
	hostConfig := launcherConf.GetHostConfigSet()["release-world"]
	if hostConfig == nil {
		return releaseWorldConfigValues{}, errors.New("missing lazy fixture Release World host config")
	}
	var worldConf cdn_world_controller.Config
	if err := worldConf.UnmarshalJSON(hostConfig.GetConfig()); err != nil {
		return releaseWorldConfigValues{}, errors.Wrap(err, "decode lazy fixture Release World config")
	}
	return releaseWorldConfigValues{
		spaceID: worldConf.GetSpaceId(),
		cdnBase: worldConf.GetCdnBaseUrl(),
	}, nil
}

func waitForCliTerminalPrompt(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/quickstart/local"); err != nil {
		t.Fatalf("open local quickstart for CLI terminal proof: %v", err)
	}
	if _, err := page.WaitForFunction(`() => /^#\/u\/\d+\/?$/.test(window.location.hash)`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("local quickstart did not create a session for CLI RPC proof: %v", err)
	}
	hash, err := page.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read local session route for CLI RPC proof: %v", err)
	}
	hashString, ok := hash.(string)
	if !ok {
		t.Fatalf("local session route has unexpected type %T", hash)
	}
	sessionIndex := strings.TrimSuffix(strings.TrimPrefix(hashString, "#/u/"), "/")
	if sessionIndex == "" {
		t.Fatalf("local session route %q has no session index for CLI RPC proof", hashString)
	}
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/u/" + sessionIndex + "/settings/cli"); err != nil {
		t.Fatalf("open CLI settings for session %s: %v", sessionIndex, err)
	}
	openCLIButton := page.Locator("button:has-text('Open CLI terminal')").First()
	if err := openCLIButton.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("CLI settings did not expose Open CLI terminal: %v", err)
	}
	if err := openCLIButton.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("open CLI terminal for RPC proof: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.location.hash.includes('/settings/cli/terminal')`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("CLI terminal route did not open: %v", err)
	}
	terminalScreen := page.Locator(".xterm:visible .xterm-screen").First()
	if err := terminalScreen.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("CLI terminal screen did not mount: %v", err)
	}
	if _, err := page.WaitForFunction(`() => (`+cliTerminalTextExpression+`).includes('spacewave>')`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("CLI RunCli stream did not reach spacewave prompt: %v", err)
	}
}

func waitForPluginManifestCopyDone(t *testing.T, page playwright.Page, pluginID string) {
	t.Helper()
	raw, err := page.WaitForFunction(`(pluginId) => {
		const marks = globalThis.__swStartupMarks ?? []
		if (marks.some((mark) =>
			mark.label === 'manifest-copy.failed' &&
			mark.detail?.pluginId === pluginId
		)) {
			return 'failed'
		}
		if (marks.some((mark) =>
			mark.label === 'manifest-copy.done' &&
			mark.detail?.pluginId === pluginId
		)) {
			return 'done'
		}
		return false
	}`, pluginID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("manifest copy completion wait failed for %s: %v", pluginID, err)
	}
	stateValue, err := raw.JSONValue()
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("read manifest copy completion state for %s: %v", pluginID, err)
	}
	state, ok := stateValue.(string)
	if !ok || state != "done" {
		dumpPageState(t, page)
		t.Fatalf("manifest copy failed for %s: state=%v", pluginID, stateValue)
	}
}

func TestGoScriptQuickstartDriveLoadsAppModule(t *testing.T) {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript quickstart Drive app module probe", E2EReleaseWasmGoScriptEnv)
	}

	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-core",
		"plugin/spacewave-launcher",
	})
	waitForQuickstartAppRoute(t, page)
	waitForQuickstartDriveAppModule(t, page)
}

const (
	sonnerModulePath     = "/b/pkg/sonner/dist/index.mjs"
	webPkgArtifactRelDir = ".bldr-dist/build/js/spacewave-web/assets/bldr-web-pkgs"
)

func waitForPluginWorkersRunning(t *testing.T, page playwright.Page, workerIDs []string) {
	t.Helper()

	_, err := page.WaitForFunction(`(workerIds) => {
		const marks = globalThis.__swStartupMarks ?? []
		return workerIds.every((workerId) =>
			marks.some((mark) =>
				mark.label === 'plugin.running' &&
				mark.detail?.workerId === workerId,
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

func waitForQuickstartDriveAppModule(t *testing.T, page playwright.Page) {
	t.Helper()

	raw, err := page.WaitForFunction(`() => {
		const text = document.body?.innerText || ''
		const failed = text.match(/Failed to load module\s+(\S+)/)
		if (failed) {
			return {
				state: 'failed',
				modulePath: failed[1],
				text,
			}
		}
		if (
			document.querySelector("[data-testid='unixfs-browser']") ||
			text.includes('Create a Drive') ||
			text.includes('Drive Quickstart')
		) {
			return {
				state: 'loaded',
				href: location.href,
				text,
			}
		}
		return false
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart Drive app module: %v", err)
	}
	value, err := raw.JSONValue()
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("read quickstart Drive app module probe payload: %v", err)
	}
	state, ok := value.(map[string]any)
	if !ok {
		dumpPageState(t, page)
		t.Fatalf("unexpected quickstart Drive app module probe payload %T", value)
	}
	if state["state"] == "failed" {
		modulePath, _ := state["modulePath"].(string)
		if modulePath != "" {
			collectQuickstartModuleLoadDifferential(t, page, modulePath)
		}
		dumpPageState(t, page)
		t.Fatalf("quickstart Drive app module failed to load: %v", state["modulePath"])
	}
	t.Logf("quickstart Drive app module loaded: %#v", state)
}

func collectQuickstartModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) {
	t.Helper()

	browserProbe := collectBrowserModuleLoadDifferential(t, page, modulePath)
	rootDirect := directServerModuleProbe(t, modulePath)
	sonnerDirect := directServerModuleProbe(t, sonnerModulePath)
	sonnerArtifact := releaseWebPkgArtifactProbe(t, sonnerModulePath)
	report := moduleLoadDifferentialReport{
		ModulePath:   modulePath,
		BrowserProbe: browserProbe,
		DirectServer: moduleLoadDifferentialDirectServer{
			Root:   rootDirect,
			Sonner: sonnerDirect,
		},
		ReleaseArtifact: moduleLoadDifferentialArtifacts{
			Sonner: sonnerArtifact,
		},
	}
	var arena fastjson.Arena
	reportJSON := report.appendJSON(&arena).MarshalTo(nil)
	t.Logf("quickstart module load differential: %s", string(reportJSON))
	writeModuleLoadDifferentialArtifact(t, string(reportJSON))

	assertBrowserModuleFetchComplete(t, "root App module", browserProbe.RootFetch)
	assertBrowserModuleFetchMatchesArtifact(t, "Sonner module", browserProbe.SonnerFetch, sonnerArtifact)
}

func collectBrowserModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) browserModuleLoadDifferential {
	t.Helper()

	raw, err := page.Evaluate(`async (args) => {
		const textEncoder = new TextEncoder()
		const textDecoder = new TextDecoder()
		const toHex = (bytes) =>
			Array.from(new Uint8Array(bytes))
				.map((byte) => byte.toString(16).padStart(2, '0'))
				.join('')
		const sha256Bytes = async (bytes) =>
			toHex(await crypto.subtle.digest('SHA-256', bytes))
		const summarizeBody = async (bytes) => {
			const text = textDecoder.decode(bytes)
			return {
				bodyComplete: true,
				bodyLength: text.length,
				bodyByteLength: bytes.byteLength,
				sha256: await sha256Bytes(bytes),
				head: text.slice(0, 160),
				tail: text.slice(-240),
			}
		}
		const collectChunks = (chunks, byteLength) => {
			const body = new Uint8Array(byteLength)
			chunks.reduce((offset, chunk) => {
				body.set(chunk, offset)
				return offset + chunk.byteLength
			}, 0)
			return body
		}
		const chunkByteLength = (chunks) =>
			chunks.reduce((total, chunk) => total + chunk.byteLength, 0)
		const readBody = async (response) => {
			const stream = response.body
			if (!stream) {
				const text = await response.text()
				const body = textEncoder.encode(text)
				return {
					...(await summarizeBody(body)),
					bodyReader: 'text',
					bodyChunks: body.byteLength > 0 ? 1 : 0,
				}
			}
			const reader = stream.getReader()
			const chunks = []
			try {
				for (;;) {
					const read = await reader.read()
					if (read.done) {
						break
					}
					if (read.value) {
						const chunk = read.value
						chunks.push(chunk)
					}
				}
			} catch (error) {
				const bodyByteLength = chunkByteLength(chunks)
				const partialBody = collectChunks(chunks, bodyByteLength)
				const partialText = textDecoder.decode(partialBody)
				return {
					ok: false,
					bodyComplete: false,
					bodyReader: 'stream',
					bodyChunks: chunks.length,
					bodyByteLength,
					partialSha256: await sha256Bytes(partialBody),
					partialHead: partialText.slice(0, 160),
					partialTail: partialText.slice(-240),
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
			const bodyByteLength = chunkByteLength(chunks)
			return {
				...(await summarizeBody(collectChunks(chunks, bodyByteLength))),
				bodyReader: 'stream',
				bodyChunks: chunks.length,
			}
		}
		const headerObject = (headers) => {
			const out = {}
			for (const [key, value] of headers.entries()) {
				if (
					key === 'content-length' ||
					key === 'content-type' ||
					key.startsWith('x-bldr-')
				) {
					out[key] = value
				}
			}
			return out
		}
		const cacheBust = (path, label) =>
			path + (path.includes('?') ? '&' : '?') + label + '=' + Date.now()
		const fetchProbe = async (path, label) => {
			const requestURL = cacheBust(path, label)
			try {
				const response = await fetch(requestURL, { cache: 'reload' })
				try {
					const body = await readBody(response)
					return {
						path,
						requestURL,
						status: response.status,
						ok: response.ok && body.bodyComplete,
						headers: headerObject(response.headers),
						...body,
					}
				} catch (error) {
					return {
						path,
						requestURL,
						ok: false,
						phase: 'body',
						status: response.status,
						headers: headerObject(response.headers),
						name: error?.name ?? '',
						message: error?.message ?? String(error),
						stack: error?.stack ?? '',
					}
				}
			} catch (error) {
				return {
					path,
					requestURL,
					ok: false,
					phase: 'fetch',
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
		}
		const importProbe = async (path, label) => {
			const requestURL = cacheBust(path, label)
			try {
				const mod = await import(/* @vite-ignore */ requestURL)
				return {
					path,
					requestURL,
					ok: true,
					exportKeys: Object.keys(mod).sort(),
					hasDefault: Object.prototype.hasOwnProperty.call(mod, 'default'),
				}
			} catch (error) {
				return {
					path,
					requestURL,
					ok: false,
					name: error?.name ?? '',
					message: error?.message ?? String(error),
					stack: error?.stack ?? '',
				}
			}
		}
		const performanceEntries = performance
			.getEntriesByType('resource')
			.map((entry) => ({
				name: entry.name,
				initiatorType: entry.initiatorType,
				transferSize: entry.transferSize,
				encodedBodySize: entry.encodedBodySize,
				decodedBodySize: entry.decodedBodySize,
			}))
			.filter((entry) =>
				entry.name.includes('/b/pa/') ||
				entry.name.includes('/b/pkg/sonner'),
			)
		return JSON.stringify({
			location: location.href,
			controllerURL: navigator.serviceWorker.controller?.scriptURL ?? '',
			rootAssetStatus: globalThis.__bldrWebViewRootAssetStatus ?? null,
			moduleImportError: globalThis.__bldrWebViewModuleImportError ?? null,
			rootFetch: await fetchProbe(args.modulePath, 'root_module_probe'),
			sonnerFetch: await fetchProbe(args.sonnerPath, 'sonner_module_probe'),
			rootImport: await importProbe(args.modulePath, 'root_import_probe'),
			sonnerImport: await importProbe(args.sonnerPath, 'sonner_import_probe'),
			performanceEntries,
		})
	}`, map[string]any{
		"modulePath": modulePath,
		"sonnerPath": sonnerModulePath,
	})
	if err != nil {
		t.Fatalf("collect browser module load differential: %v", err)
	}
	encoded, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected browser module load differential payload %T", raw)
	}
	probe, err := parseBrowserModuleLoadDifferential(encoded)
	if err != nil {
		t.Fatalf("parse browser module load differential payload: %v", err)
	}
	return probe
}

func directServerModuleProbe(t *testing.T, modulePath string) moduleBodyProbe {
	t.Helper()

	if !strings.HasPrefix(modulePath, "/") {
		t.Fatalf("module path must be absolute: %q", modulePath)
	}
	req, err := http.NewRequest(http.MethodGet, testHarness.getBaseURL()+modulePath, nil)
	if err != nil {
		t.Fatalf("build direct module request %q: %v", modulePath, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("direct module request %q: %v", modulePath, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read direct module response %q: %v", modulePath, err)
	}
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.Status = resp.StatusCode
	probe.ContentType = resp.Header.Get("Content-Type")
	probe.ContentLength = resp.Header.Get("Content-Length")
	return probe
}

func releaseWebPkgArtifactProbe(t *testing.T, modulePath string) moduleBodyProbe {
	t.Helper()

	artifactRelPath := releaseWebPkgArtifactRelPath(t, modulePath)
	body, err := os.ReadFile(filepath.Join(testHarness.repoRoot, artifactRelPath))
	if err != nil {
		t.Fatalf("read release web package artifact %s for %s: %v", artifactRelPath, modulePath, err)
	}
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.ArtifactRelPath = artifactRelPath
	probe.OK = true
	return probe
}

func releaseWebPkgArtifactRelPath(t *testing.T, modulePath string) string {
	t.Helper()

	const prefix = "/b/pkg/"
	if !strings.HasPrefix(modulePath, prefix) {
		t.Fatalf("release web package artifact path must begin with %s: %q", prefix, modulePath)
	}
	pkgPath := strings.TrimPrefix(modulePath, prefix)
	cleanPkgPath := path.Clean(pkgPath)
	if cleanPkgPath == "." || cleanPkgPath == ".." || strings.HasPrefix(cleanPkgPath, "../") || path.IsAbs(cleanPkgPath) {
		t.Fatalf("release web package artifact path escapes package root: %q", modulePath)
	}
	return filepath.ToSlash(filepath.Join(webPkgArtifactRelDir, filepath.FromSlash(cleanPkgPath)))
}

func summarizeModuleBody(body []byte) moduleBodyProbe {
	sum := sha256.Sum256(body)
	bodyText := string(body)
	head := bodyText
	if len(head) > 160 {
		head = head[:160]
	}
	tail := bodyText
	if len(tail) > 240 {
		tail = tail[len(tail)-240:]
	}
	return moduleBodyProbe{
		BodyByteLength: len(body),
		SHA256:         hex.EncodeToString(sum[:]),
		Head:           head,
		Tail:           tail,
	}
}

func assertBrowserModuleFetchComplete(t *testing.T, label string, browser moduleFetchProbe) {
	t.Helper()

	if browser.Status != http.StatusOK {
		t.Fatalf("%s browser probe did not return 200: %#v", label, browser)
	}
	if !browser.OK {
		t.Fatalf("%s browser body failed after headers: %#v", label, browser)
	}
}

func assertBrowserModuleFetchMatchesArtifact(t *testing.T, label string, browser moduleFetchProbe, artifact moduleBodyProbe) {
	t.Helper()

	assertBrowserModuleFetchComplete(t, label, browser)
	if !artifact.OK {
		t.Fatalf("%s release artifact probe failed: %#v", label, artifact)
	}
	if browser.BodyByteLength != artifact.BodyByteLength {
		t.Fatalf("%s browser body length %d != release artifact length %d: browser=%#v artifact=%#v", label, browser.BodyByteLength, artifact.BodyByteLength, browser, artifact)
	}
	if browser.SHA256 != artifact.SHA256 {
		t.Fatalf("%s browser body hash %v != release artifact hash %v: browser=%#v artifact=%#v", label, browser.SHA256, artifact.SHA256, browser, artifact)
	}
}

func writeModuleLoadDifferentialArtifact(t *testing.T, state string) {
	t.Helper()

	if testHarness == nil || testHarness.artifactDir == "" {
		return
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	path := filepath.Join(
		testHarness.artifactDir,
		replacer.Replace(strings.ToLower(t.Name()))+"-module-load-differential.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("write module load differential artifact mkdir %s: %v", path, err)
		return
	}
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		t.Logf("write module load differential artifact %s: %v", path, err)
		return
	}
	t.Logf("module load differential artifact: %s", path)
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
	waitForQuickstartAppRoute(t, page)
	completeQuickstartDriveIntroIfPresent(t, page)
	err = page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
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
	driveGoldenPathReadyMs, driveGoldenPathError := exerciseQuickstartDriveGoldenPath(t, page)
	if driveGoldenPathError != "" {
		t.Logf("quickstart golden path not reached: %s", driveGoldenPathError)
	}
	postLoadSOWorkload := runQuickstartPostLoadSOWorkload(t, page, driveContentReadyMs != nil)
	foregroundResume := collectForegroundResumeEvidence(t, page)
	logQuickstartTiming(t, page)
	runtimeTrace := traceCapture.stop(t)

	data, err := collectQuickstartSmokeArtifact(page, desc, source, driveFrameReadyMs, driveContentReadyMs, driveContentReadyError, driveGoldenPathReadyMs, driveGoldenPathError, runtimeTrace, postLoadSOWorkload, foregroundResume)
	if err != nil {
		t.Fatalf("collect quickstart smoke artifact: %v", err)
	}
	if err := writeQuickstartSmokeArtifact(path, data); err != nil {
		t.Fatalf("write quickstart smoke artifact: %v", err)
	}
	t.Logf("quickstart smoke artifact written to %s (%d bytes)", path, len(data))
}

func TestQuickstartSecondTabReusesRuntimeAndCloseKeepsFirstTab(t *testing.T) {
	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto first quickstart drive: %v", err)
	}
	waitForPrerenderRoot(t, pageA)
	waitForBootFunction(t, pageA)
	waitForLiveApp(t, pageA)
	waitForQuickstartAppRoute(t, pageA)
	completeQuickstartDriveIntroIfPresent(t, pageA)
	if err := pageA.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, pageA)
		t.Fatalf("wait for first quickstart frame-ready: %v", err)
	}
	firstURL := pageA.URL()
	if _, err := pageA.Evaluate(`() => {
		const navEvents = []
		globalThis.__swCrossTabNavEvents = navEvents
		const record = (type, detail = {}) => {
			navEvents.push({
				type,
				href: location.href,
				hash: location.hash,
				time: performance.now(),
				...detail,
			})
		}
		window.addEventListener('hashchange', () => record('hashchange'))
		window.addEventListener('storage', (ev) => record('storage', {
			key: ev.key,
			newValue: ev.newValue,
		}))
		globalThis.__swCrossTabReloadProbe = {
			token: crypto.randomUUID(),
			href: location.href,
			markedAt: performance.now(),
		}
	}`); err != nil {
		t.Fatalf("install first tab reload probe: %v", err)
	}

	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(quickstartURL); err != nil {
		t.Fatalf("goto second quickstart drive: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, pageB)
	waitForBootFunction(t, pageB)
	waitForLiveApp(t, pageB)
	waitForQuickstartAppRoute(t, pageB)
	completeQuickstartDriveIntroIfPresent(t, pageB)
	if err := pageB.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, pageB)
		t.Fatalf("wait for second quickstart frame-ready: %v", err)
	}

	if err := pageB.Close(); err != nil {
		t.Fatalf("close second quickstart tab: %v", err)
	}
	if err := pageA.BringToFront(); err != nil {
		t.Fatalf("bring first quickstart tab to front: %v", err)
	}
	raw, err := pageA.Evaluate(`async () => {
		await new Promise((resolve) => requestAnimationFrame(() => {
			requestAnimationFrame(resolve)
		}))
		const marker = globalThis.__swCrossTabReloadProbe ?? null
		const driveReady = !!document.querySelector("[data-testid='unixfs-browser']")
		return {
			href: location.href,
			markerPresent: !!marker,
			markerHref: marker?.href ?? '',
			driveReady,
			bootStatus: globalThis.__swBootStatus ?? null,
			resumeReady: globalThis.__swWebDocumentResumeReady ?? null,
			navEvents: globalThis.__swCrossTabNavEvents ?? [],
			localTabs: localStorage.getItem('shell-tabs-state'),
			sessionTabs: sessionStorage.getItem('shell-tabs-state'),
		}
	}`)
	if err != nil {
		t.Fatalf("read first tab after closing second: %v", err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected first tab close state %T", raw)
	}
	if state["href"] != firstURL {
		t.Fatalf("first tab URL changed after closing second tab: got %v want %s state=%#v", state["href"], firstURL, state)
	}
	if state["markerPresent"] != true {
		dumpPageState(t, pageA)
		t.Fatalf("first tab reloaded after closing second tab: %#v", state)
	}
	if state["driveReady"] != true {
		dumpPageState(t, pageA)
		t.Fatalf("first tab lost drive readiness after closing second tab: %#v", state)
	}
}

func TestQuickstartShellTabsComposedBrowserProof(t *testing.T) {
	pageA := testHarness.newDedicatedWorkerPage(t)
	ctx := pageA.Context()
	legacyShellState := `() => {
		sessionStorage.setItem('shell-tabs-state', JSON.stringify({
			tabs: [{ id: 'legacy-tab', path: '/legacy', name: 'Legacy' }],
			activeTabId: 'legacy-tab',
		}))
	}`
	if err := pageA.AddInitScript(playwright.Script{Content: &legacyShellState}); err != nil {
		t.Fatalf("seed explicit old Shell snapshot reset case: %v", err)
	}

	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, pageA, quickstartURL)
	assertRuntimeWorkerMode(t, pageA, "dedicated-worker")
	hostGeneration, hostDocumentID := assertDedicatedWorkerHost(t, pageA)
	assertWarmPresentation(t, pageA, hostGeneration, hostDocumentID, false)
	waitForShellRecordCount(t, pageA, 1)
	initialSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(initialSnapshot.Records) != 1 {
		t.Fatalf("old Shell state was not cleanly initialized: %#v", initialSnapshot)
	}
	if initialSnapshot.Records[0].ID == "legacy-tab" || initialSnapshot.Records[0].Path == "/legacy" {
		t.Fatalf("legacy Shell record was imported instead of reset: %#v", initialSnapshot.Records[0])
	}
	legacyAfterInit, err := pageA.Evaluate(`() => sessionStorage.getItem('shell-tabs-state')`)
	if err != nil {
		t.Fatalf("read obsolete Shell snapshot after clean initialization: %v", err)
	}
	if legacyAfterInit != nil {
		t.Fatalf("obsolete shell-tabs-state survived clean initialization: %#v", legacyAfterInit)
	}
	firstURL := pageA.URL()

	pageB := testHarness.newPageInContext(t, ctx)
	openQuickstartReleasePage(t, pageB, quickstartURL)
	assertRuntimeWorkerMode(t, pageB, "dedicated-worker")
	assertWarmPresentation(t, pageB, hostGeneration, hostDocumentID, true)
	waitForShellRecordCount(t, pageA, 2)
	waitForShellRecordCount(t, pageB, 2)
	logWarmAttachCorrectnessMetrics(t, pageB, hostGeneration)
	afterB := readBrowserShellTabsSnapshot(t, pageB)
	if len(afterB.Records) != 2 {
		t.Fatalf("fresh second document did not create exactly one shared record: %#v", afterB)
	}
	secondRecordID := findNewBrowserShellRecord(initialSnapshot, afterB)
	if secondRecordID == "" {
		t.Fatalf("fresh second document did not add one new record: before=%#v after=%#v", initialSnapshot, afterB)
	}
	if pageA.URL() != firstURL {
		dumpPageState(t, pageA)
		dumpPageState(t, pageB)
		t.Fatalf("second document stole first document URL: got %s want %s", pageA.URL(), firstURL)
	}
	assertNoRuntimeWorkerCreated(t, pageB)

	logShellDiagnostic(t, pageA, "before_docs_hash_page_a")
	logShellDiagnostic(t, pageB, "before_docs_hash_page_b")
	setShellHash(t, pageB, "#/docs")
	logShellDiagnostic(t, pageB, "after_docs_hash_page_b")
	waitForBrowserShellRecordPath(t, pageB, secondRecordID, "/docs")
	if pageA.URL() != firstURL {
		t.Fatalf("inactive shared path update changed first document hash: got %s want %s", pageA.URL(), firstURL)
	}
	renameActiveShellTab(t, pageB, "Shared Docs")
	waitForShellLabel(t, pageA, "Shared Docs")
	sharedSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	sharedRecord := findBrowserShellRecord(sharedSnapshot, secondRecordID)
	if sharedRecord == nil || sharedRecord.Path != "/docs" || sharedRecord.CustomName != "Shared Docs" {
		t.Fatalf("shared path/name/customName did not converge: %#v", sharedSnapshot)
	}

	beforeConcurrentA := readComposedShellProjection(t, pageA)
	beforeConcurrentB := readComposedShellProjection(t, pageB)
	concurrentCreateShellTabs(t, pageA, pageB)
	waitForShellRecordCount(t, pageA, 4)
	waitForShellRecordCount(t, pageB, 4)
	concurrentSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(concurrentSnapshot.Records) != 4 {
		t.Fatalf("concurrent Shell creation lost a record: %#v", concurrentSnapshot)
	}
	if !sameBrowserShellRecordIDs(t, concurrentSnapshot, readBrowserShellTabsSnapshot(t, pageB)) {
		t.Fatalf("A/B shared record inventories diverged after concurrent creation")
	}
	waitForBrowserShellActiveRecordChange(t, pageA, beforeConcurrentA.ActiveTabID)
	waitForBrowserShellActiveRecordChange(t, pageB, beforeConcurrentB.ActiveTabID)
	setShellHash(t, pageA, "#/a-only")
	setShellHash(t, pageB, "#/b-only")
	waitForBrowserShellActivePath(t, pageA, "/a-only")
	waitForBrowserShellActivePath(t, pageB, "/b-only")
	if pageA.URL() == pageB.URL() {
		t.Fatalf("A/B active selection and hash are not independent: A=%s B=%s", pageA.URL(), pageB.URL())
	}
	assertDifferentShellProjectionOrder(t, pageA, pageB)
	waitForShellLabel(t, pageA, "Shared Docs")
	waitForShellLabel(t, pageB, "Shared Docs")
	independentProjectionA := readComposedShellProjection(t, pageA)

	selectShellTabByText(t, pageB, "Shared Docs")
	retainedURL := ""
	popup, err := ctx.ExpectPage(func() error {
		return pageB.Locator("button[title='Open in new tab']").First().Click()
	})
	if err != nil {
		t.Fatalf("open retained-ID Shell popout: %v", err)
	}
	if _, err := popup.WaitForFunction(`() => location.href !== 'about:blank'`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("wait for retained-ID popup navigation: %v", err)
	}
	openQuickstartReleasePage(t, popup, popup.URL())
	retainedURL = popup.URL()
	if !strings.Contains(retainedURL, "shellTabId="+secondRecordID) {
		t.Fatalf("popout URL lost retained Shell Tab ID: %s", retainedURL)
	}
	waitForShellRecordCount(t, popup, 4)
	waitForShellLabel(t, popup, "Shared Docs")

	copied := testHarness.newPageInContext(t, ctx)
	if _, err := copied.Goto(retainedURL); err != nil {
		t.Fatalf("goto copied retained-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, copied, retainedURL)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")
	if _, err := copied.Reload(); err != nil {
		t.Fatalf("reload copied retained-ID URL: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, copied)
	waitForLiveApp(t, copied)
	waitForShellTabButtons(t, copied)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")

	beforeCloseA := readComposedShellProjection(t, pageA)
	assertSameComposedShellProjection(
		t,
		beforeCloseA,
		independentProjectionA,
		"retained-ID transitions changed page A",
	)
	closeShellTabByText(t, pageB, "Shared Docs")
	waitForShellRecordCount(t, pageA, 3)
	waitForShellRecordCount(t, pageB, 3)
	afterCloseA := readComposedShellProjection(t, pageA)
	assertInactiveClosePreservedProjection(t, beforeCloseA, afterCloseA, "Shared Docs")
	assertShellLabelAbsent(t, pageA, "Shared Docs")
	assertShellLabelAbsent(t, pageB, "Shared Docs")

	removedIDURL := retainedURL
	removed := testHarness.newPageInContext(t, ctx)
	if _, err := removed.Goto(removedIDURL); err != nil {
		t.Fatalf("goto removed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, removed, removedIDURL)
	waitForShellRecordCount(t, removed, 4)
	removedSnapshot := readBrowserShellTabsSnapshot(t, removed)
	if findBrowserShellRecord(removedSnapshot, secondRecordID) != nil {
		t.Fatalf("removed-ID handoff resurrected the closed record: %#v", removedSnapshot)
	}
	if removedSnapshot.Records[len(removedSnapshot.Records)-1].Path != "/docs" {
		t.Fatalf("removed-ID fallback did not create a fresh /docs record: %#v", removedSnapshot)
	}
	invalidURL := strings.Replace(retainedURL, "shellTabId="+secondRecordID, "shellTabId=!malformed", 1)
	invalid := testHarness.newPageInContext(t, ctx)
	if _, err := invalid.Goto(invalidURL); err != nil {
		t.Fatalf("goto malformed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, invalid, invalidURL)
	waitForShellRecordCount(t, invalid, 5)
	invalidHash := invalid.URL()
	if _, err := invalid.Reload(); err != nil {
		t.Fatalf("reload malformed-ID fallback: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, invalid)
	waitForLiveApp(t, invalid)
	waitForShellTabButtons(t, invalid)
	waitForShellRecordCount(t, invalid, 5)
	if invalid.URL() != invalidHash {
		t.Fatalf("malformed-ID fallback reload changed stable URL: got %s want %s", invalid.URL(), invalidHash)
	}

	for _, page := range []playwright.Page{popup, copied, removed, invalid} {
		if err := page.Close(); err != nil {
			t.Fatalf("close completed retained-ID proof page: %v", err)
		}
	}
	pageFail := testHarness.newPageInContext(t, ctx)
	if _, err := pageFail.Goto(retainedURL); err != nil {
		t.Fatalf("goto retained URL for relay-failure proof: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, pageFail)
	waitForBootFunction(t, pageFail)
	waitForLiveApp(t, pageFail)
	waitForStartupMark(t, pageFail, "dedicated-host.attach-open-start")
	if err := pageA.Close(); err != nil {
		t.Fatalf("close elected host document: %v", err)
	}
	waitForStartupMark(t, pageFail, "dedicated-host.attach-open-failed")
	assertRelayFailureStayedCold(t, pageFail)
	promotionGeneration := assertWarmPromotion(t, pageB, hostGeneration)
	assertWarmPresentation(t, pageB, promotionGeneration, "", false)
	waitForShellRecordCount(t, pageFail, 6)
	expectedInventory := browserShellRecordIDs(readBrowserShellTabsSnapshot(t, pageFail))
	if !sameStringSet(browserShellRecordIDs(readBrowserShellTabsSnapshot(t, pageB)), expectedInventory) {
		t.Fatalf("Shell inventory did not survive host loss/promotion")
	}
	if err := pageB.BringToFront(); err != nil {
		t.Fatalf("bring survivor document to front: %v", err)
	}
	if err := pageB.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		t.Fatalf("wait for promoted survivor usability: %v", err)
	}
	preservedSnapshot := readBrowserShellTabsSnapshot(t, pageB)
	for _, page := range ctx.Pages() {
		if page.IsClosed() {
			continue
		}
		if err := page.Close(); err != nil {
			t.Fatalf("close document before zero-document retention check: %v", err)
		}
	}
	reopen := testHarness.newPageInContext(t, ctx)
	openQuickstartReleasePage(t, reopen, quickstartURL)
	waitForShellRecordCount(t, reopen, len(preservedSnapshot.Records)+1)
	reopenedSnapshot := readBrowserShellTabsSnapshot(t, reopen)
	if findNewBrowserShellRecord(preservedSnapshot, reopenedSnapshot) == "" {
		t.Fatalf("fresh zero-document reopen did not add exactly one route record: before=%#v after=%#v", preservedSnapshot, reopenedSnapshot)
	}
	if !sameBrowserShellRecordValues(preservedSnapshot, reopenedSnapshot) {
		t.Fatalf("zero-document reopen changed existing Shell record fields: before=%#v after=%#v", preservedSnapshot, reopenedSnapshot)
	}
	beforeReset := reopenedSnapshot
	resetShellTabsVisibly(t, reopen)
	waitForShellRecordCount(t, reopen, 1)
	afterReset := readBrowserShellTabsSnapshot(t, reopen)
	if afterReset.Epoch <= beforeReset.Epoch || len(afterReset.Records) != 1 {
		t.Fatalf("visible Shell reset did not advance epoch and replace inventory: before=%#v after=%#v", beforeReset, afterReset)
	}
	waitForShellTabButtons(t, reopen)
}

func TestQuickstartShellTabsPersistentProfileRestart(t *testing.T) {
	if testHarness.browserName != "chromium" {
		t.Skip("persistent-profile release proof requires Chromium")
	}

	userDataDir := t.TempDir()
	firstContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	firstContextClosed := false
	t.Cleanup(func() {
		if firstContextClosed {
			return
		}
		if err := firstContext.Close(); err != nil {
			t.Logf("close first persistent browser context during cleanup: %v", err)
		}
	})
	firstPage := testHarness.newPageInContext(t, firstContext)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, firstPage, quickstartURL)
	waitForShellRecordCount(t, firstPage, 1)
	if err := firstPage.Locator("button[title='New tab']").First().Click(); err != nil {
		t.Fatalf("create persistent-profile Shell record: %v", err)
	}
	waitForShellRecordCount(t, firstPage, 2)
	beforeRestart := readBrowserShellTabsSnapshot(t, firstPage)
	if len(beforeRestart.Records) != 2 {
		t.Fatalf("persistent-profile setup did not create two records: %#v", beforeRestart)
	}
	if err := firstContext.Close(); err != nil {
		t.Fatalf("close persistent browser context for restart: %v", err)
	}
	firstContextClosed = true

	secondContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	t.Cleanup(func() {
		if err := secondContext.Close(); err != nil {
			t.Logf("close relaunched persistent browser context: %v", err)
		}
	})
	secondPage := testHarness.newPageInContext(t, secondContext)
	openQuickstartReleasePage(t, secondPage, quickstartURL)
	waitForShellRecordCount(t, secondPage, len(beforeRestart.Records)+1)
	afterRestart := readBrowserShellTabsSnapshot(t, secondPage)
	if findNewBrowserShellRecord(beforeRestart, afterRestart) == "" ||
		!sameBrowserShellRecordValues(beforeRestart, afterRestart) {
		t.Fatalf("persistent-profile browser restart did not preserve records while creating fresh entry: before=%#v after=%#v", beforeRestart, afterRestart)
	}
	waitForShellLabel(t, secondPage, beforeRestart.Records[0].Name)

	beforeReset := afterRestart
	resetShellTabsVisibly(t, secondPage)
	waitForShellRecordCount(t, secondPage, 1)
	afterReset := readBrowserShellTabsSnapshot(t, secondPage)
	if afterReset.Epoch <= beforeReset.Epoch || len(afterReset.Records) != 1 {
		t.Fatalf("persistent-profile visible reset did not advance epoch: before=%#v after=%#v", beforeReset, afterReset)
	}
	waitForShellTabButtons(t, secondPage)
}

type composedShellRecord struct {
	ID               string
	Path             string
	Name             string
	CustomName       string
	CreationSequence float64
}

type composedShellSnapshot struct {
	SchemaVersion float64
	Epoch         float64
	Revision      float64
	Records       []composedShellRecord
}

type composedShellProjection struct {
	URL            string
	Hash           string
	ActiveTabID    string
	VisiblePanelID string
	Labels         []string
}

type shellDiagnosticStorage struct {
	Present bool
	Raw     string
}

type shellDiagnosticRecord struct {
	LocationHref         string
	LocationHash         string
	SessionDocumentState shellDiagnosticStorage
	BrowserShellTabs     shellDiagnosticStorage
	VisibleActivePanelID string
}

func logShellDiagnostic(t *testing.T, page playwright.Page, label string) shellDiagnosticRecord {
	t.Helper()

	raw, err := page.Evaluate(`() => {
		const readStorage = (storage, key) => {
			const raw = storage.getItem(key)
			return { present: raw !== null, raw }
		}
		const visiblePanel = [...document.querySelectorAll('[data-tab-id]')].find((element) => {
			const style = getComputedStyle(element)
			const rect = element.getBoundingClientRect()
			return style.display !== 'none' &&
				style.visibility !== 'hidden' &&
				rect.width > 0 &&
				rect.height > 0
		})
		return {
			locationHref: location.href,
			locationHash: location.hash,
			sessionDocumentState: readStorage(sessionStorage, 'shell-document-state'),
			browserShellTabs: readStorage(localStorage, 'browser-shell-tabs'),
			visibleActivePanelID: visiblePanel?.getAttribute('data-tab-id') ?? '',
		}
	}`)
	if err != nil {
		t.Fatalf("shell_diagnostic label=%s collection_error=%v", label, err)
	}
	record, err := decodeShellDiagnosticRecord(raw)
	if err != nil {
		t.Fatalf("shell_diagnostic label=%s decode_error=%v payload_type=%T", label, err, raw)
	}
	t.Logf(
		"shell_diagnostic label=%s location_href=%q location_hash=%q "+
			"session_storage_shell_document_state_present=%t "+
			"session_storage_shell_document_state_raw=%q "+
			"local_storage_browser_shell_tabs_present=%t "+
			"local_storage_browser_shell_tabs_raw=%q "+
			"visible_active_panel_id=%q",
		label,
		record.LocationHref,
		record.LocationHash,
		record.SessionDocumentState.Present,
		record.SessionDocumentState.Raw,
		record.BrowserShellTabs.Present,
		record.BrowserShellTabs.Raw,
		record.VisibleActivePanelID,
	)
	return record
}

func decodeShellDiagnosticRecord(raw any) (shellDiagnosticRecord, error) {
	value, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticRecord{}, errors.Errorf("expected object, got %T", raw)
	}
	locationHref, err := requiredShellDiagnosticString(value, "locationHref", true)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	locationHash, err := requiredShellDiagnosticString(value, "locationHash", false)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	sessionDocumentState, err := decodeShellDiagnosticStorage(value, "sessionDocumentState")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	browserShellTabs, err := decodeShellDiagnosticStorage(value, "browserShellTabs")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	visibleActivePanelID, err := requiredShellDiagnosticString(value, "visibleActivePanelID", false)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	return shellDiagnosticRecord{
		LocationHref:         locationHref,
		LocationHash:         locationHash,
		SessionDocumentState: sessionDocumentState,
		BrowserShellTabs:     browserShellTabs,
		VisibleActivePanelID: visibleActivePanelID,
	}, nil
}

func requiredShellDiagnosticString(value map[string]any, field string, nonEmpty bool) (string, error) {
	raw, ok := value[field]
	if !ok {
		return "", errors.Errorf("missing %s", field)
	}
	text, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("%s has type %T, want string", field, raw)
	}
	if nonEmpty && text == "" {
		return "", errors.Errorf("%s is empty", field)
	}
	return text, nil
}

func decodeShellDiagnosticStorage(value map[string]any, field string) (shellDiagnosticStorage, error) {
	raw, ok := value[field]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s", field)
	}
	storage, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s has type %T, want object", field, raw)
	}
	present, ok := storage["present"].(bool)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s.present has type %T, want bool", field, storage["present"])
	}
	rawValue, ok := storage["raw"]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s.raw", field)
	}
	if rawValue == nil {
		if present {
			return shellDiagnosticStorage{}, errors.Errorf("%s.raw is null while present", field)
		}
		return shellDiagnosticStorage{}, nil
	}
	text, ok := rawValue.(string)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s.raw has type %T, want string or null", field, rawValue)
	}
	if !present {
		return shellDiagnosticStorage{}, errors.Errorf("%s.raw is non-null while absent", field)
	}
	return shellDiagnosticStorage{Present: true, Raw: text}, nil
}

type composedStartupMark struct {
	Label    string
	Sequence float64
	Detail   map[string]any
}

func openQuickstartReleasePage(t *testing.T, page playwright.Page, targetURL string) {
	t.Helper()

	if _, err := page.Goto(targetURL); err != nil {
		t.Fatalf("goto composed Shell page %s: %v", targetURL, err)
	}
	waitForPrerenderRootOrLiveApp(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	if strings.Contains(targetURL, "/quickstart/drive") ||
		strings.Contains(targetURL, "#/quickstart/drive") {
		waitForQuickstartAppRoute(t, page)
		completeQuickstartDriveIntroIfPresent(t, page)
		if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
			playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
		); err != nil {
			dumpPageState(t, page)
			t.Fatalf("wait for composed quickstart frame: %v", err)
		}
		return
	}
	waitForShellTabButtons(t, page)
}

func readBrowserShellTabsSnapshot(t *testing.T, page playwright.Page) composedShellSnapshot {
	t.Helper()

	raw, err := page.Evaluate(`() => {
		const value = localStorage.getItem('browser-shell-tabs')
		return value ? JSON.parse(value) : null
	}`)
	if err != nil {
		t.Fatalf("read browser Shell Tabs snapshot: %v", err)
	}
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("browser Shell Tabs snapshot missing or invalid: %#v", raw)
	}
	snapshot := composedShellSnapshot{
		SchemaVersion: composedNumber(value["schemaVersion"]),
		Epoch:         composedNumber(value["epoch"]),
		Revision:      composedNumber(value["revision"]),
	}
	rawRecords, ok := value["records"].([]any)
	if !ok {
		t.Fatalf("browser Shell Tabs records missing or invalid: %#v", value)
	}
	snapshot.Records = make([]composedShellRecord, 0, len(rawRecords))
	for _, rawRecord := range rawRecords {
		record, ok := rawRecord.(map[string]any)
		if !ok {
			t.Fatalf("browser Shell Tabs record invalid: %#v", rawRecord)
		}
		snapshot.Records = append(snapshot.Records, composedShellRecord{
			ID:               composedString(record["id"]),
			Path:             composedString(record["path"]),
			Name:             composedString(record["name"]),
			CustomName:       composedString(record["customName"]),
			CreationSequence: composedNumber(record["creationSequence"]),
		})
	}
	if snapshot.SchemaVersion != 1 {
		t.Fatalf("browser Shell Tabs schema version=%v want 1: %#v", snapshot.SchemaVersion, snapshot)
	}
	return snapshot
}

func composedNumber(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func composedString(value any) string {
	text, _ := value.(string)
	return text
}

func findNewBrowserShellRecord(before, after composedShellSnapshot) string {
	beforeIDs := make(map[string]bool, len(before.Records))
	for _, record := range before.Records {
		beforeIDs[record.ID] = true
	}
	var added string
	for _, record := range after.Records {
		if !beforeIDs[record.ID] {
			if added != "" {
				return ""
			}
			added = record.ID
		}
	}
	if len(after.Records) != len(before.Records)+1 {
		return ""
	}
	return added
}

func findBrowserShellRecord(snapshot composedShellSnapshot, id string) *composedShellRecord {
	for index := range snapshot.Records {
		if snapshot.Records[index].ID == id {
			return &snapshot.Records[index]
		}
	}
	return nil
}

func browserShellRecordIDs(snapshot composedShellSnapshot) []string {
	ids := make([]string, 0, len(snapshot.Records))
	for _, record := range snapshot.Records {
		ids = append(ids, record.ID)
	}
	return ids
}

func sameBrowserShellRecordIDs(t *testing.T, left, right composedShellSnapshot) bool {
	t.Helper()

	leftIDs := browserShellRecordIDs(left)
	rightIDs := browserShellRecordIDs(right)
	return sameStringSet(leftIDs, rightIDs)
}

func sameBrowserShellRecordValues(before, after composedShellSnapshot) bool {
	afterByID := make(map[string]composedShellRecord, len(after.Records))
	for _, record := range after.Records {
		afterByID[record.ID] = record
	}
	for _, record := range before.Records {
		if afterRecord, ok := afterByID[record.ID]; !ok || afterRecord != record {
			return false
		}
	}
	return true
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]bool, len(left))
	for _, value := range left {
		values[value] = true
	}
	for _, value := range right {
		if !values[value] {
			return false
		}
	}
	return true
}

func waitForShellRecordCount(t *testing.T, page playwright.Page, count int) {
	t.Helper()

	_, err := page.WaitForFunction(`(want) => {
		try {
			const value = localStorage.getItem('browser-shell-tabs')
			return value && JSON.parse(value).records?.length === want
		} catch {
			return false
		}
	}`, count, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for %d shared Shell records: %v", count, err)
	}
}

func waitForBrowserShellRecordPath(t *testing.T, page playwright.Page, id, path string) {
	t.Helper()

	t.Logf("shell_record_path_wait phase=before target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_before")

	_, err := page.WaitForFunction(`(args) => {
		try {
			const value = localStorage.getItem('browser-shell-tabs')
			const records = value ? JSON.parse(value).records : []
			return records.some((record) => record.id === args.id && record.path === args.path)
		} catch {
			return false
		}
	}`, map[string]any{"id": id, "path": path}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		t.Logf("shell_record_path_wait phase=failure target_id=%q target_path=%q error=%v", id, path, err)
		logShellDiagnostic(t, page, "browser_shell_record_path_wait_failure")
		t.Fatalf("wait for shared Shell record %s path %s: %v", id, path, err)
	}
	t.Logf("shell_record_path_wait phase=success target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_success")
}

func waitForBrowserShellActiveRecordChange(t *testing.T, page playwright.Page, previousID string) {
	t.Helper()

	_, err := page.WaitForFunction(`(previousID) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			const snapshot = JSON.parse(localStorage.getItem('browser-shell-tabs') ?? 'null')
			const active = snapshot?.records?.find((record) => record.id === documentState?.activeTabId)
			return active &&
				active.id !== previousID &&
				location.hash === '#' + active.path
		} catch {
			return false
		}
	}`, previousID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for created Shell record selection after %s: %v", previousID, err)
	}
}

func waitForBrowserShellActivePath(t *testing.T, page playwright.Page, path string) {
	t.Helper()

	_, err := page.WaitForFunction(`(want) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			const snapshot = JSON.parse(localStorage.getItem('browser-shell-tabs') ?? 'null')
			const active = snapshot?.records?.find((record) => record.id === documentState?.activeTabId)
			return active?.path === want && location.hash.startsWith('#' + want)
		} catch {
			return false
		}
	}`, path, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for active Shell path %s: %v", path, err)
	}
}

func waitForShellTabButtons(t *testing.T, page playwright.Page) {
	t.Helper()

	tabs := page.Locator(".flexlayout__tab_button:visible").First()
	if err := tabs.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for visible Shell tab buttons: %v", err)
	}
	enabled, err := tabs.IsEnabled()
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("check visible Shell tab button interactivity: %v", err)
	}
	if !enabled {
		dumpPageState(t, page)
		t.Fatalf("visible Shell tab button is disabled")
	}
}

func setShellHash(t *testing.T, page playwright.Page, hash string) {
	t.Helper()

	if _, err := page.Evaluate(`(next) => {
		window.location.hash = next
		return window.location.hash
	}`, hash); err != nil {
		t.Fatalf("navigate visible Shell hash %s: %v", hash, err)
	}
}

func readComposedShellProjection(t *testing.T, page playwright.Page) composedShellProjection {
	t.Helper()

	raw, err := page.Evaluate(`() => {
		const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
		const visiblePanel = [...document.querySelectorAll('[data-tab-id]')].find((element) => {
			const style = getComputedStyle(element)
			const rect = element.getBoundingClientRect()
			return style.display !== 'none' &&
				style.visibility !== 'hidden' &&
				rect.width > 0 &&
				rect.height > 0
		})
		return {
			url: location.href,
			hash: location.hash,
			activeTabId: documentState?.activeTabId ?? '',
			visiblePanelId: visiblePanel?.getAttribute('data-tab-id') ?? '',
			labels: [...document.querySelectorAll('.flexlayout__tab_button')].map((button) => button.textContent?.trim() || ''),
		}
	}`)
	if err != nil {
		t.Fatalf("read visible Shell projection: %v", err)
	}
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("visible Shell projection invalid: %#v", raw)
	}
	rawLabels, ok := value["labels"].([]any)
	if !ok {
		t.Fatalf("visible Shell labels invalid: %#v", value)
	}
	labels := make([]string, 0, len(rawLabels))
	for _, rawLabel := range rawLabels {
		labels = append(labels, composedString(rawLabel))
	}
	return composedShellProjection{
		URL:            composedString(value["url"]),
		Hash:           composedString(value["hash"]),
		ActiveTabID:    composedString(value["activeTabId"]),
		VisiblePanelID: composedString(value["visiblePanelId"]),
		Labels:         labels,
	}
}

func assertSameComposedShellProjection(
	t *testing.T,
	got, want composedShellProjection,
	message string,
) {
	t.Helper()

	if got.URL != want.URL ||
		got.Hash != want.Hash ||
		got.ActiveTabID != want.ActiveTabID ||
		got.VisiblePanelID != want.VisiblePanelID ||
		!slices.Equal(got.Labels, want.Labels) {
		t.Fatalf("%s: got=%#v want=%#v", message, got, want)
	}
}

func assertInactiveClosePreservedProjection(
	t *testing.T,
	before, after composedShellProjection,
	removedLabel string,
) {
	t.Helper()

	expectedLabels := make([]string, 0, len(before.Labels))
	removed := false
	for _, label := range before.Labels {
		if label == removedLabel && !removed {
			removed = true
			continue
		}
		expectedLabels = append(expectedLabels, label)
	}
	if !removed {
		t.Fatalf("inactive close precondition lacks label %q: %#v", removedLabel, before)
	}
	assertSameComposedShellProjection(
		t,
		after,
		composedShellProjection{
			URL:            before.URL,
			Hash:           before.Hash,
			ActiveTabID:    before.ActiveTabID,
			VisiblePanelID: before.VisiblePanelID,
			Labels:         expectedLabels,
		},
		"closing inactive shared record changed page A projection",
	)
}

func assertDifferentShellProjectionOrder(t *testing.T, pageA, pageB playwright.Page) {
	t.Helper()

	projectionA := readComposedShellProjection(t, pageA)
	projectionB := readComposedShellProjection(t, pageB)
	if strings.Join(projectionA.Labels, "\x00") == strings.Join(projectionB.Labels, "\x00") {
		t.Fatalf("A/B visible Shell order unexpectedly converged: A=%v B=%v", projectionA.Labels, projectionB.Labels)
	}
}

func waitForShellLabel(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	if err := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("wait for visible Shell label %q: %v", label, err)
	}
}

func assertShellLabelAbsent(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	count, err := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).Count()
	if err != nil {
		t.Fatalf("count visible Shell label %q: %v", label, err)
	}
	if count != 0 {
		t.Fatalf("removed Shell label %q remains visible: count=%d", label, count)
	}
}

func renameActiveShellTab(t *testing.T, page playwright.Page, customName string) {
	t.Helper()

	tabs := page.Locator(".flexlayout__tab_button")
	if err := tabs.Last().Dblclick(); err != nil {
		t.Fatalf("start visible Shell tab rename: %v", err)
	}
	input := tabs.Last().Locator("input:visible").First()
	if err := input.Fill(customName); err != nil {
		t.Fatalf("fill visible Shell custom name: %v", err)
	}
	if err := input.Press("Enter"); err != nil {
		t.Fatalf("commit visible Shell custom name: %v", err)
	}
}

func selectShellTabByText(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	tab := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First()
	if err := tab.Click(); err != nil {
		t.Fatalf("select visible Shell tab %q: %v", label, err)
	}
}

func closeShellTabByText(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	tab := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First()
	if err := tab.Click(playwright.LocatorClickOptions{Button: playwright.MouseButtonRight}); err != nil {
		t.Fatalf("open Shell tab context menu %q: %v", label, err)
	}
	closeItem := page.Locator("[role='menuitem']:visible").Filter(playwright.LocatorFilterOptions{
		HasText: "Close Tab",
	}).First()
	if err := closeItem.Click(); err != nil {
		t.Fatalf("close shared Shell tab %q: %v", label, err)
	}
}

func concurrentCreateShellTabs(t *testing.T, pageA, pageB playwright.Page) {
	t.Helper()

	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, page := range []playwright.Page{pageA, pageB} {
		waitGroup.Add(1)
		go func(page playwright.Page) {
			defer waitGroup.Done()
			_, err := page.Evaluate(`() => {
				const button = [...document.querySelectorAll("button[title='New tab']")]
					.find((candidate) => candidate instanceof HTMLElement && candidate.offsetParent !== null)
				if (!button) throw new Error('visible New tab button is missing')
				button.click()
				return true
			}`)
			errs <- err
		}(page)
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent visible Shell record creation: %v", err)
		}
	}
}

func resetShellTabsVisibly(t *testing.T, page playwright.Page) {
	t.Helper()

	if err := page.Locator("button:visible:has-text('View')").First().Click(); err != nil {
		t.Fatalf("open visible View menu: %v", err)
	}
	reset := page.Locator("[role='menuitem']:visible:has-text('Reset Shell Tabs')").First()
	if err := reset.Click(); err != nil {
		t.Fatalf("invoke visible Shell reset: %v", err)
	}
}

func readComposedStartupMarks(t *testing.T, page playwright.Page) []composedStartupMark {
	t.Helper()

	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? {},
	}))`)
	if err != nil {
		t.Fatalf("read production startup marks: %v", err)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("production startup marks invalid: %#v", raw)
	}
	marks := make([]composedStartupMark, 0, len(items))
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}
		detail, _ := value["detail"].(map[string]any)
		marks = append(marks, composedStartupMark{
			Label:    composedString(value["label"]),
			Sequence: composedNumber(value["sequence"]),
			Detail:   detail,
		})
	}
	return marks
}

func lastComposedStartupMark(t *testing.T, page playwright.Page, label string) composedStartupMark {
	t.Helper()

	marks := readComposedStartupMarks(t, page)
	for index := len(marks) - 1; index >= 0; index-- {
		if marks[index].Label == label {
			return marks[index]
		}
	}
	t.Fatalf("production startup mark %q is missing", label)
	return composedStartupMark{}
}

func waitForStartupMark(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	_, err := page.WaitForFunction(`(want) =>
		(globalThis.__swStartupMarks ?? []).some((mark) => mark.label === want)`,
		label, playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(browserWaitMS),
		})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for production startup mark %q: %v", label, err)
	}
}

func assertDedicatedWorkerHost(t *testing.T, page playwright.Page) (string, string) {
	t.Helper()

	lease := lastComposedStartupMark(t, page, "dedicated-host.lease-acquired")
	generation := composedString(lease.Detail["generation"])
	documentID := composedString(lease.Detail["documentId"])
	if generation == "" || documentID == "" {
		t.Fatalf("host lease mark lacks generation/document identity: %#v", lease)
	}
	marks := readComposedStartupMarks(t, page)
	workerCount := 0
	for _, mark := range marks {
		if mark.Label == "runtime.worker-created" &&
			composedString(mark.Detail["mode"]) == "dedicated-worker" {
			workerCount++
		}
	}
	if workerCount != 1 {
		t.Fatalf("supported DedicatedWorker path created %d runtime Workers in host document: %#v", workerCount, marks)
	}
	return generation, documentID
}

func assertNoRuntimeWorkerCreated(t *testing.T, page playwright.Page) {
	t.Helper()

	for _, mark := range readComposedStartupMarks(t, page) {
		if mark.Label == "runtime.worker-created" &&
			composedString(mark.Detail["mode"]) == "dedicated-worker" {
			t.Fatalf("attached document created a runtime Worker: %#v", mark)
		}
	}
}

func assertWarmPresentation(t *testing.T, page playwright.Page, generation, hostDocumentID string, requireAttachment bool) {
	t.Helper()

	marks := readComposedStartupMarks(t, page)
	if requireAttachment {
		ready := lastComposedStartupMark(t, page, "dedicated-host.attach-open-ready")
		if composedString(ready.Detail["hostGeneration"]) != generation ||
			composedString(ready.Detail["hostDocumentId"]) != hostDocumentID {
			t.Fatalf("attach acknowledged an unexpected runtime generation/host: %#v want generation=%q host=%q", ready, generation, hostDocumentID)
		}
	}
	var connected composedStartupMark
	for index := len(marks) - 1; index >= 0; index-- {
		if marks[index].Label == "runtime.connected" &&
			composedString(marks[index].Detail["runtimeGeneration"]) == generation {
			connected = marks[index]
			break
		}
	}
	if connected.Label == "" {
		t.Fatalf("runtime.connected did not carry current generation %q: %#v", generation, marks)
	}
	var neutral, reveal composedStartupMark
	for _, mark := range marks {
		if mark.Label == "webview.neutral-frame" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > connected.Sequence {
			neutral = mark
			break
		}
	}
	for _, mark := range marks {
		if mark.Label == "webview.revealed" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > neutral.Sequence {
			reveal = mark
			break
		}
	}
	if neutral.Label == "" || reveal.Label == "" || neutral.Sequence >= reveal.Sequence {
		t.Fatalf("generation-matched neutral/reveal order missing for %q: connected=%#v neutral=%#v reveal=%#v marks=%#v", generation, connected, neutral, reveal, marks)
	}
	for _, mark := range marks {
		if mark.Label == "runtime.connection-invalidated" && mark.Sequence >= connected.Sequence {
			t.Fatalf("runtime presentation advanced before obsolete generation invalidation: invalidated=%#v connected=%#v", mark, connected)
		}
	}
	raw, err := page.Evaluate(`() => globalThis.__swWebDocumentResumeReady ?? null`)
	if err != nil {
		t.Fatalf("read production resume-ready state: %v", err)
	}
	resume, ok := raw.(map[string]any)
	if !ok || resume["ready"] != true {
		t.Fatalf("production resume-ready surface is not ready: %#v", raw)
	}
	runtimeMark := lastComposedStartupMark(t, page, "runtime.mode-selected")
	if composedString(resume["runtimeId"]) != composedString(runtimeMark.Detail["runtimeId"]) {
		t.Fatalf("resume-ready runtime identity drifted: resume=%#v mode=%#v", resume, runtimeMark)
	}
}

func assertWarmPromotion(t *testing.T, page playwright.Page, oldGeneration string) string {
	t.Helper()

	waitForStartupMark(t, page, "dedicated-host.promoted")
	promoted := lastComposedStartupMark(t, page, "dedicated-host.promoted")
	generation := composedString(promoted.Detail["generation"])
	if generation == "" || generation == oldGeneration {
		t.Fatalf("promotion did not replace runtime generation: old=%q mark=%#v", oldGeneration, promoted)
	}
	_, err := page.WaitForFunction(`(args) => {
		const marks = globalThis.__swStartupMarks ?? []
		return marks.some((mark) =>
			mark.label === 'runtime.connection-invalidated' &&
			mark.detail?.runtimeGeneration === args.oldGeneration &&
			mark.sequence > args.promotedSequence,
		) && marks.some((mark) =>
			mark.label === 'runtime.connected' &&
			mark.detail?.runtimeGeneration === args.generation &&
			mark.sequence > args.promotedSequence,
		)
	}`, map[string]any{
		"oldGeneration":    oldGeneration,
		"generation":       generation,
		"promotedSequence": promoted.Sequence,
	}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for old-generation invalidation and new-generation connection after promotion: %v", err)
	}
	marks := readComposedStartupMarks(t, page)
	var invalidated, connected composedStartupMark
	for _, mark := range marks {
		if mark.Label == "runtime.connection-invalidated" &&
			composedString(mark.Detail["runtimeGeneration"]) == oldGeneration &&
			mark.Sequence > promoted.Sequence &&
			(invalidated.Label == "" || mark.Sequence < invalidated.Sequence) {
			invalidated = mark
		}
		if mark.Label == "runtime.connected" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > promoted.Sequence &&
			(connected.Label == "" || mark.Sequence < connected.Sequence) {
			connected = mark
		}
	}
	if invalidated.Label == "" || connected.Label == "" || invalidated.Sequence >= connected.Sequence {
		t.Fatalf("promotion did not invalidate old runtime before new connection: old=%q new=%q promoted=%#v invalidated=%#v connected=%#v marks=%#v", oldGeneration, generation, promoted, invalidated, connected, marks)
	}
	for _, mark := range marks {
		if mark.Sequence <= promoted.Sequence {
			continue
		}
		if (mark.Label == "webview.revealed" || mark.Label == "runtime.connected") &&
			composedString(mark.Detail["runtimeGeneration"]) == oldGeneration {
			t.Fatalf("obsolete runtime generation advanced presentation after promotion: %#v", mark)
		}
	}
	return generation
}

func assertRelayFailureStayedCold(t *testing.T, page playwright.Page) {
	t.Helper()

	marks := readComposedStartupMarks(t, page)
	var failed composedStartupMark
	for index := len(marks) - 1; index >= 0; index-- {
		if marks[index].Label == "dedicated-host.attach-open-failed" {
			failed = marks[index]
			break
		}
	}
	if failed.Label == "" {
		t.Fatalf("relay failure mark is missing")
	}
	var start composedStartupMark
	for index := len(marks) - 1; index >= 0; index-- {
		if marks[index].Label == "dedicated-host.attach-open-start" &&
			marks[index].Sequence < failed.Sequence {
			start = marks[index]
			break
		}
	}
	if start.Label == "" {
		t.Fatalf("relay failure has no preceding attach-open-start: %#v", marks)
	}
	for _, mark := range marks {
		if mark.Sequence <= start.Sequence || mark.Sequence > failed.Sequence {
			continue
		}
		if mark.Label == "dedicated-host.attach-open-ready" ||
			mark.Label == "runtime.connected" ||
			mark.Label == "webview.neutral-frame" ||
			mark.Label == "webview.revealed" {
			t.Fatalf("relay failure was silently accepted as warm presentation: start=%#v failed=%#v mark=%#v", start, failed, mark)
		}
	}
}

func logWarmAttachCorrectnessMetrics(t *testing.T, page playwright.Page, generation string) {
	t.Helper()

	raw, err := page.Evaluate(`(generation) => {
		const entries = performance.getEntriesByType('mark')
			.filter((entry) => entry.name.startsWith('spacewave.startup.'))
			.map((entry) => ({
				label: entry.name.slice('spacewave.startup.'.length),
				startTime: entry.startTime,
				detail: entry.detail ?? {},
			}))
		const find = (label) => entries.find((entry) =>
			entry.label === label &&
			(entry.detail.runtimeGeneration === generation ||
				entry.detail.hostGeneration === generation),
		)
		const navigation = performance.getEntriesByType('navigation')[0]
		const neutral = find('webview.neutral-frame')
		const attachReady = find('dedicated-host.attach-open-ready')
		const connected = find('runtime.connected')
		const reveal = find('webview.revealed')
		return {
			generation,
			navigationToNeutralMs: neutral && navigation
				? neutral.startTime - navigation.startTime
				: null,
			attachReadyToConnectedMs: attachReady && connected
				? connected.startTime - attachReady.startTime
				: null,
			connectedToRevealMs: connected && reveal
				? reveal.startTime - connected.startTime
				: null,
			navigationToRevealMs: reveal && navigation
				? reveal.startTime - navigation.startTime
				: null,
		}
	}`, generation)
	if err != nil {
		t.Fatalf("read warm-attach correctness metrics: %v", err)
	}
	t.Logf("warm-attach correctness metrics (no speed claim): %#v", raw)
}

func TestGoScriptQuickstartReturnVisitorMountsBodyRoute(t *testing.T) {
	if compiler, err := resolveReleaseWasmCompiler(); err != nil {
		t.Fatalf("resolve release wasm compiler: %v", err)
	} else if compiler != releaseWasmCompilerGoScript {
		t.Skipf("release-WASM return visitor body route gate requires %s=true", E2EReleaseWasmGoScriptEnv)
	}

	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}
	waitForPrerenderRoot(t, pageA)
	waitForBootFunction(t, pageA)
	waitForLiveApp(t, pageA)
	waitForQuickstartAppRoute(t, pageA)
	completeQuickstartDriveIntroIfPresent(t, pageA)
	if err := pageA.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, pageA)
		t.Fatalf("wait for quickstart frame-ready: %v", err)
	}
	hash, err := pageA.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read created direct route hash: %v", err)
	}
	hashText, ok := hash.(string)
	if !ok || !strings.HasPrefix(hashText, "#/u/") || !strings.Contains(hashText, "/so/") {
		t.Fatalf("quickstart did not produce direct SharedObject route hash: %#v", hash)
	}
	directURL := testHarness.getBaseURL() + "/" + hashText

	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(directURL); err != nil {
		t.Fatalf("goto direct SharedObject route: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, pageB)
	waitForBootFunction(t, pageB)
	waitForLiveApp(t, pageB)
	waitForQuickstartAppRoute(t, pageB)
	if err := pageB.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, pageB)
		t.Fatalf("wait for return visitor direct route frame-ready: %v", err)
	}
	if _, err := waitForQuickstartDriveContentReady(t, pageB); err != "" {
		t.Fatalf("wait for return visitor direct route content-ready: %s", err)
	}
	assertReturnVisitorBodyRouteStartupMarks(t, pageB)
	assertReturnVisitorBodyRouteSpaceState(t, pageB)
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
	if os.Getenv("E2E_RELEASE_WASM_RUNTIME_TRACE") != "1" {
		info["skippedReason"] = "set E2E_RELEASE_WASM_RUNTIME_TRACE=1 to capture a Chromium runtime trace"
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

func waitForLiveApp(t *testing.T, page playwright.Page) {
	t.Helper()

	deadline := time.Now().Add(time.Duration(browserWaitMS) * time.Millisecond)
	for {
		timeoutMS := int(time.Until(deadline) / time.Millisecond)
		if timeoutMS <= 0 {
			dumpPageState(t, page)
			t.Fatal("wait for live app: timed out after navigation retries")
		}
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
	stateStr, ok := state.(string)
	if !ok {
		t.Logf("page state: unexpected payload type %T", state)
		return
	}
	writePageStateArtifact(t, stateStr)
	t.Logf("page state: %s", stateStr)
}

func writePageStateArtifact(t *testing.T, state string) {
	t.Helper()

	if testHarness == nil || testHarness.artifactDir == "" {
		return
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	path := filepath.Join(
		testHarness.artifactDir,
		replacer.Replace(strings.ToLower(t.Name()))+"-page-state.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("write page state artifact mkdir %s: %v", path, err)
		return
	}
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		t.Logf("write page state artifact %s: %v", path, err)
		return
	}
	t.Logf("page state artifact: %s", path)
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

func assertReturnVisitorBodyRouteStartupMarks(t testing.TB, page playwright.Page) {
	t.Helper()

	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? null,
	}))`, nil)
	if err != nil {
		t.Fatalf("read return visitor body route startup marks: %v", err)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("unexpected return visitor body route startup marks %T: %#v", raw, raw)
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		mark, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if label, ok := mark["label"].(string); ok {
			labels = append(labels, label)
		}
	}
	for _, label := range []string{
		"quickstart.session-mount-start",
		"quickstart.session-mount-ready",
		"quickstart.shared-object-mount-start",
		"quickstart.shared-object-mount-ready",
		"quickstart.shared-object-body-mount-start",
		"quickstart.shared-object-body-mount-ready",
		"quickstart.space-resource-created",
		"quickstart.space-world-access-ready",
		"quickstart.space-contents-mount-ready",
		"unixfs.browser-mounted",
		"unixfs.seeded-file-visible",
	} {
		if !slices.Contains(labels, label) {
			t.Fatalf("return visitor body route missing startup mark %q; labels=%v", label, labels)
		}
	}
	for _, label := range []string{
		"quickstart.session-handoff-used",
		"quickstart.shared-object-handoff-used",
		"quickstart.shared-object-body-handoff-used",
		"quickstart.space-handoff-used",
		"quickstart.space-world-handoff-used",
		"quickstart.space-contents-handoff-used",
	} {
		if slices.Contains(labels, label) {
			t.Fatalf("return visitor body route unexpectedly used Quickstart handoff mark %q; labels=%v", label, labels)
		}
	}
}

func assertReturnVisitorBodyRouteSpaceState(t testing.TB, page playwright.Page) {
	t.Helper()

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
			return {
				ready: !!state?.ready,
				indexPath: state?.settings?.indexPath ?? '',
				objectKeys: (state?.worldContents?.objects ?? []).map((obj) => obj.objectKey ?? ''),
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

func exerciseQuickstartDriveGoldenPath(t *testing.T, page playwright.Page) (*int, string) {
	t.Helper()

	if err := page.Locator("[data-testid='drive-welcome']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(quickstartContentReadyRecordMS)},
	); err != nil {
		dumpPageState(t, page)
		return nil, "drive welcome guidance did not appear: " + err.Error()
	}
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
	driveGoldenPathReadyMs *int,
	driveGoldenPathError string,
	runtimeTrace map[string]any,
	postLoadSOWorkload map[string]any,
	foregroundResume map[string]any,
) ([]byte, error) {
	var driveContentReadyArg any
	if driveContentReadyMs != nil {
		driveContentReadyArg = *driveContentReadyMs
	}
	var driveGoldenPathReadyArg any
	if driveGoldenPathReadyMs != nil {
		driveGoldenPathReadyArg = *driveGoldenPathReadyMs
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

type moduleLoadDifferentialReport struct {
	ModulePath      string
	BrowserProbe    browserModuleLoadDifferential
	DirectServer    moduleLoadDifferentialDirectServer
	ReleaseArtifact moduleLoadDifferentialArtifacts
}

type moduleLoadDifferentialDirectServer struct {
	Root   moduleBodyProbe
	Sonner moduleBodyProbe
}

type moduleLoadDifferentialArtifacts struct {
	Sonner moduleBodyProbe
}

type browserModuleLoadDifferential struct {
	Location           string
	ControllerURL      string
	RootAssetStatus    *fastjson.Value
	ModuleImportError  *fastjson.Value
	RootFetch          moduleFetchProbe
	SonnerFetch        moduleFetchProbe
	RootImport         moduleImportProbe
	SonnerImport       moduleImportProbe
	PerformanceEntries []modulePerformanceEntry
}

type moduleFetchProbe struct {
	Path           string
	RequestURL     string
	Status         int
	OK             bool
	Headers        map[string]string
	BodyComplete   bool
	BodyReader     string
	BodyChunks     int
	BodyLength     int
	BodyByteLength int
	SHA256         string
	Head           string
	Tail           string
	PartialSHA256  string
	PartialHead    string
	PartialTail    string
	Phase          string
	Name           string
	Message        string
	Stack          string
}

type moduleImportProbe struct {
	Path       string
	RequestURL string
	OK         bool
	ExportKeys []string
	HasDefault bool
	Name       string
	Message    string
	Stack      string
}

type modulePerformanceEntry struct {
	Name            string
	InitiatorType   string
	TransferSize    int
	EncodedBodySize int
	DecodedBodySize int
}

type moduleBodyProbe struct {
	Path            string
	ArtifactRelPath string
	Status          int
	OK              bool
	ContentType     string
	ContentLength   string
	BodyByteLength  int
	SHA256          string
	Head            string
	Tail            string
}

func parseBrowserModuleLoadDifferential(data string) (browserModuleLoadDifferential, error) {
	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		return browserModuleLoadDifferential{}, err
	}
	return browserModuleLoadDifferential{
		Location:           string(value.GetStringBytes("location")),
		ControllerURL:      string(value.GetStringBytes("controllerURL")),
		RootAssetStatus:    value.Get("rootAssetStatus"),
		ModuleImportError:  value.Get("moduleImportError"),
		RootFetch:          parseModuleFetchProbe(value.Get("rootFetch")),
		SonnerFetch:        parseModuleFetchProbe(value.Get("sonnerFetch")),
		RootImport:         parseModuleImportProbe(value.Get("rootImport")),
		SonnerImport:       parseModuleImportProbe(value.Get("sonnerImport")),
		PerformanceEntries: parseModulePerformanceEntries(value.GetArray("performanceEntries")),
	}, nil
}

func parseModuleFetchProbe(value *fastjson.Value) moduleFetchProbe {
	if value == nil {
		return moduleFetchProbe{}
	}
	return moduleFetchProbe{
		Path:           string(value.GetStringBytes("path")),
		RequestURL:     string(value.GetStringBytes("requestURL")),
		Status:         value.GetInt("status"),
		OK:             value.GetBool("ok"),
		Headers:        parseStringMapValue(value.Get("headers")),
		BodyComplete:   value.GetBool("bodyComplete"),
		BodyReader:     string(value.GetStringBytes("bodyReader")),
		BodyChunks:     value.GetInt("bodyChunks"),
		BodyLength:     value.GetInt("bodyLength"),
		BodyByteLength: value.GetInt("bodyByteLength"),
		SHA256:         string(value.GetStringBytes("sha256")),
		Head:           string(value.GetStringBytes("head")),
		Tail:           string(value.GetStringBytes("tail")),
		PartialSHA256:  string(value.GetStringBytes("partialSha256")),
		PartialHead:    string(value.GetStringBytes("partialHead")),
		PartialTail:    string(value.GetStringBytes("partialTail")),
		Phase:          string(value.GetStringBytes("phase")),
		Name:           string(value.GetStringBytes("name")),
		Message:        string(value.GetStringBytes("message")),
		Stack:          string(value.GetStringBytes("stack")),
	}
}

func parseModuleImportProbe(value *fastjson.Value) moduleImportProbe {
	if value == nil {
		return moduleImportProbe{}
	}
	return moduleImportProbe{
		Path:       string(value.GetStringBytes("path")),
		RequestURL: string(value.GetStringBytes("requestURL")),
		OK:         value.GetBool("ok"),
		ExportKeys: parseStringArray(value.GetArray("exportKeys")),
		HasDefault: value.GetBool("hasDefault"),
		Name:       string(value.GetStringBytes("name")),
		Message:    string(value.GetStringBytes("message")),
		Stack:      string(value.GetStringBytes("stack")),
	}
}

func parseModulePerformanceEntries(values []*fastjson.Value) []modulePerformanceEntry {
	entries := make([]modulePerformanceEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, modulePerformanceEntry{
			Name:            string(value.GetStringBytes("name")),
			InitiatorType:   string(value.GetStringBytes("initiatorType")),
			TransferSize:    value.GetInt("transferSize"),
			EncodedBodySize: value.GetInt("encodedBodySize"),
			DecodedBodySize: value.GetInt("decodedBodySize"),
		})
	}
	return entries
}

func parseStringArray(values []*fastjson.Value) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value.GetStringBytes()))
	}
	return out
}

func parseStringMapValue(value *fastjson.Value) map[string]string {
	obj := value.GetObject()
	if obj == nil {
		return nil
	}
	values := make(map[string]string, obj.Len())
	obj.Visit(func(key []byte, value *fastjson.Value) {
		values[string(key)] = string(value.GetStringBytes())
	})
	return values
}

func (r moduleLoadDifferentialReport) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("modulePath", arena.NewString(r.ModulePath))
	obj.Set("browserProbe", r.BrowserProbe.appendJSON(arena))
	directServer := arena.NewObject()
	directServer.Set("root", r.DirectServer.Root.appendJSON(arena))
	directServer.Set("sonner", r.DirectServer.Sonner.appendJSON(arena))
	obj.Set("directServer", directServer)
	releaseArtifact := arena.NewObject()
	releaseArtifact.Set("sonner", r.ReleaseArtifact.Sonner.appendJSON(arena))
	obj.Set("releaseArtifact", releaseArtifact)
	return obj
}

func (p browserModuleLoadDifferential) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("location", arena.NewString(p.Location))
	obj.Set("controllerURL", arena.NewString(p.ControllerURL))
	setRawJSONValue(arena, obj, "rootAssetStatus", p.RootAssetStatus)
	setRawJSONValue(arena, obj, "moduleImportError", p.ModuleImportError)
	obj.Set("rootFetch", p.RootFetch.appendJSON(arena))
	obj.Set("sonnerFetch", p.SonnerFetch.appendJSON(arena))
	obj.Set("rootImport", p.RootImport.appendJSON(arena))
	obj.Set("sonnerImport", p.SonnerImport.appendJSON(arena))
	entries := arena.NewArray()
	for _, entry := range p.PerformanceEntries {
		entries.SetArrayItem(len(entries.GetArray()), entry.appendJSON(arena))
	}
	obj.Set("performanceEntries", entries)
	return obj
}

func (p moduleFetchProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("headers", appendStringMapJSON(arena, p.Headers))
	setBoolJSONField(arena, obj, "bodyComplete", p.BodyComplete)
	obj.Set("bodyReader", arena.NewString(p.BodyReader))
	obj.Set("bodyChunks", arena.NewNumberInt(p.BodyChunks))
	obj.Set("bodyLength", arena.NewNumberInt(p.BodyLength))
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))
	obj.Set("partialSha256", arena.NewString(p.PartialSHA256))
	obj.Set("partialHead", arena.NewString(p.PartialHead))
	obj.Set("partialTail", arena.NewString(p.PartialTail))
	obj.Set("phase", arena.NewString(p.Phase))
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("message", arena.NewString(p.Message))
	obj.Set("stack", arena.NewString(p.Stack))
	return obj
}

func (p moduleImportProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	setBoolJSONField(arena, obj, "ok", p.OK)
	exportKeys := arena.NewArray()
	for _, key := range p.ExportKeys {
		exportKeys.SetArrayItem(len(exportKeys.GetArray()), arena.NewString(key))
	}
	obj.Set("exportKeys", exportKeys)
	setBoolJSONField(arena, obj, "hasDefault", p.HasDefault)
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("message", arena.NewString(p.Message))
	obj.Set("stack", arena.NewString(p.Stack))
	return obj
}

func (p modulePerformanceEntry) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("initiatorType", arena.NewString(p.InitiatorType))
	obj.Set("transferSize", arena.NewNumberInt(p.TransferSize))
	obj.Set("encodedBodySize", arena.NewNumberInt(p.EncodedBodySize))
	obj.Set("decodedBodySize", arena.NewNumberInt(p.DecodedBodySize))
	return obj
}

func (p moduleBodyProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("artifactRelPath", arena.NewString(p.ArtifactRelPath))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("contentType", arena.NewString(p.ContentType))
	obj.Set("contentLength", arena.NewString(p.ContentLength))
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))
	return obj
}

func appendStringMapJSON(arena *fastjson.Arena, values map[string]string) *fastjson.Value {
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		obj.Set(key, arena.NewString(values[key]))
	}
	return obj
}

func setBoolJSONField(arena *fastjson.Arena, obj *fastjson.Value, key string, value bool) {
	if value {
		obj.Set(key, arena.NewTrue())
	} else {
		obj.Set(key, arena.NewFalse())
	}
}

func setRawJSONValue(arena *fastjson.Arena, obj *fastjson.Value, key string, value *fastjson.Value) {
	if value == nil {
		obj.Set(key, arena.NewNull())
		return
	}
	obj.Set(key, value)
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
