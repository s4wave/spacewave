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
	dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
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
	// Configure the release harness logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Require the release harness opt-in before starting browsers.
	if !E2EReleaseWasmEnabled() {
		le.Info("skipping e2e/releasewasm package; set ENABLE_E2E_RELEASE_WASM=true to run")
		os.Exit(0)
	}

	// Apply the release startup trace configuration.
	if err := applyReleaseStartupTraceEnv(); err != nil {
		le.WithError(err).Fatal("apply release wasm startup trace env")
	}

	// Bound the release harness lifetime and release its context.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Boot the release harness and expose it to the tests.
	h, err := boot(ctx, le)
	if err != nil {
		le.WithError(err).Fatal("boot release wasm harness")
	}
	testHarness = h

	// Run the release tests and release their browser harness.
	code := m.Run()
	h.release(le)
	os.Exit(code)
}

func TestBrowserReleaseDescriptorIncludesPrerenderedShell(t *testing.T) {
	// Read the production browser release descriptor.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the release schema and generation identity.
	if desc.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", desc.SchemaVersion)
	}
	if desc.GenerationID == "" {
		t.Fatal("expected generation id")
	}

	// Verify the release shell includes its entrypoint and workers.
	if desc.ShellAssets.Entrypoint == "" {
		t.Fatal("expected shellAssets.entrypoint")
	}
	if desc.ShellAssets.ServiceWorker == "" {
		t.Fatal("expected shellAssets.serviceWorker")
	}
	if desc.ShellAssets.SharedWorker == "" {
		t.Fatal("expected shellAssets.sharedWorker")
	}

	// Verify the release includes the root and Drive prerendered routes.
	if !slices.Contains(desc.PrerenderedRoutes, "/") {
		t.Fatalf("expected / in prerendered routes: %v", desc.PrerenderedRoutes)
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/quickstart/drive") {
		t.Fatalf("expected /quickstart/drive in prerendered routes: %v", desc.PrerenderedRoutes)
	}
}

func TestLaunchPostPrerenderAssets(t *testing.T) {
	// Open the prerendered launch post.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/blog/2026/04/launch"); err != nil {
		t.Fatalf("goto launch post: %v", err)
	}

	// Inspect the launch post styles and image delivery.
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

	// Decode the launch post inspection result.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected launch post state %T: %#v", raw, raw)
	}

	// Verify the launch post assets and signoff structure.
	for _, key := range []string{"linkedHydrateCss", "avatarLoaded", "avatarSameOrigin", "signoffHasHardBreaks"} {
		if !releaseBoolField(state, key) {
			t.Errorf("expected %s: %#v", key, state)
		}
	}

	// Verify the launch post typography and spacing.
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
	// Open the prerendered release root.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle from the prerendered root.
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
	// Require the GoScript compiler for the dedicated-worker smoke test.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript dedicated-worker release smoke", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with a dedicated worker.
	page := testHarness.newDedicatedWorkerPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle and verify its dedicated-worker mode.
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
	// Run only against the GoScript release build.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript ServiceWorker plugin dist module probe", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with HTTP tracing.
	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production bundle into the quickstart Drive route.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}

	// Wait for the probed plugins to run.
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-core",
		"plugin/spacewave-launcher",
	})

	// Reach the quickstart Drive frame.
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

	// Exercise Drive so the plugins serve real traffic before the probe.
	t.Log("drive gate: wait for content ready")
	if _, quickstartErr := waitForQuickstartDriveContentReady(t, page); quickstartErr != "" {
		t.Fatalf("wait for quickstart drive content ready: %s", quickstartErr)
	}
	t.Log("drive gate: exercise golden path")
	if _, quickstartErr := exerciseQuickstartDriveGoldenPath(t, page); quickstartErr != "" {
		t.Fatalf("exercise quickstart drive golden path: %s", quickstartErr)
	}

	// Fetch each plugin entry and its relative module graph through the
	// ServiceWorker. The entry is a small loader that imports the GoScript
	// program chunks, so the size floor applies to the whole graph.
	raw, err := page.Evaluate(`async (args) => {
		await navigator.serviceWorker.ready
		const controllerURL = navigator.serviceWorker.controller?.scriptURL || ''
		if (!controllerURL) {
			throw new Error('page is not controlled by the release ServiceWorker')
		}

		const hasDefaultExport = (text) =>
			/\bexport\s*\{[^}]*\bas\s+default\b[^}]*\}\s*;?\s*$/.test(text) ||
			/\bexport\s+default\b/.test(text)
		const relativeImports = (text, baseURL) =>
			Array.from(
				text.matchAll(/\b(?:from|import)\s*\(?\s*["'\x60](\.{1,2}\/[^"'\x60]+\.mjs)["'\x60]/g),
				(match) => new URL(match[1], baseURL).href,
			)
		const failures = []
		const graphs = []

		// fetchModule returns one module body, or '' after recording a failure.
		const fetchModule = async (url, round) => {
			const requestURL = url + '?sw_module_integrity=' + round + '-' + Date.now()
			const response = await fetch(requestURL, { cache: 'reload' })
			const text = await response.text()
			const result = {
				path: new URL(url).pathname,
				round,
				status: response.status,
				contentType: response.headers.get('content-type') ?? '',
				bodyLength: text.length,
				head: text.slice(0, 120),
			}
			if (!response.ok) {
				failures.push({ ...result, reason: 'non-OK response' })
				return ''
			}
			if (/^\s*</.test(text)) {
				failures.push({ ...result, reason: 'response looks like HTML instead of JavaScript' })
				return ''
			}
			return text
		}

		for (let round = 0; round < args.rounds; round++) {
			for (const path of args.paths) {
				const entryURL = new URL(path, location.origin).href
				const entry = await fetchModule(entryURL, round)
				if (!entry) {
					continue
				}
				if (!hasDefaultExport(entry)) {
					failures.push({ path, round, tail: entry.slice(-180), reason: 'module body has no default export shape' })
				}

				const seen = new Set([entryURL])
				const queue = relativeImports(entry, entryURL)
				let graphLength = entry.length
				while (queue.length) {
					const url = queue.shift()
					if (seen.has(url)) {
						continue
					}
					seen.add(url)
					const text = await fetchModule(url, round)
					graphLength += text.length
					queue.push(...relativeImports(text, url))
				}

				const graph = { path, round, moduleCount: seen.size, graphLength }
				graphs.push(graph)
				if (graphLength < args.minGraphLength) {
					failures.push({ ...graph, reason: 'module graph shorter than expected minimum' })
				}
			}
		}

		if (failures.length) {
			throw new Error(
				'ServiceWorker plugin dist module integrity probe failed: ' +
					JSON.stringify({ controllerURL, failures, graphs }, null, 2),
			)
		}
		return { controllerURL, graphs }
	}`, map[string]any{
		"paths": []string{
			"/b/pd/spacewave-core/spacewave-core.mjs",
			"/b/pd/spacewave-launcher/spacewave-launcher.mjs",
		},
		"rounds":         3,
		"minGraphLength": 1024 * 1024,
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("probe ServiceWorker plugin dist module integrity: %v", err)
	}
	t.Logf("ServiceWorker plugin dist module integrity probe: %#v", raw)
}

func TestBrowserReleaseLazyPluginRemoteSupplyAndDurableRestart(t *testing.T) {
	// Require the lazy-plugin release fixture opt-in.
	if os.Getenv("E2E_RELEASE_WASM_LAZY_PLUGIN_FIXTURE") != "1" {
		t.Skip("set E2E_RELEASE_WASM_LAZY_PLUGIN_FIXTURE=1 to run the release-world lazy-plugin fixture")
	}

	// Resolve the fixture's Release World CDN pack prefix.
	releaseWorld, err := releaseWorldFixtureConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	releasePackPrefix := strings.TrimRight(releaseWorld.cdnBase, "/") + "/" + releaseWorld.spaceID + "/packs/"

	// Verify the descriptor leaves CLI plugin delivery to the Release World.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range desc.RequiredStaticAssets {
		if strings.Contains(asset, "spacewave-cli-plugin") {
			t.Fatalf("lazy fixture descriptor embeds CLI plugin asset %q", asset)
		}
	}

	// Collect Release World pack requests and route-abort failures.
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

		// Ignore requests outside the fixture's Release World pack prefix.
		if !strings.HasPrefix(req.URL(), releasePackPrefix) {
			return
		}

		// Record the Release World request and its Range header under the lock.
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

	// Boot the lazy CLI plugin from the release root.
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto lazy-plugin fixture root: %v", err)
	}
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	if _, err := page.Evaluate(`() => globalThis.__swBoot('#/')`); err != nil {
		t.Fatalf("boot lazy-plugin fixture: %v", err)
	}

	// Wait for the CLI plugin and its durable manifest copy.
	waitForLiveApp(t, page)
	waitForPluginWorkersRunning(t, page, []string{
		"plugin/spacewave-cli-plugin",
	})
	waitForCliTerminalPrompt(t, page)
	waitForPluginManifestCopyDone(t, page, "spacewave-cli-plugin")

	// Snapshot the first startup's Release World requests.
	requestsMu.Lock()
	firstRequests := slices.Clone(releaseWorldRequests)
	requestsMu.Unlock()

	// Verify first startup fetched a Release World pack by range.
	firstRangeCount := 0
	for _, request := range firstRequests {
		if request.rangeHeader != "" {
			firstRangeCount++
		}
	}
	if firstRangeCount == 0 {
		t.Fatal("lazy plugin became ready without a Release World CDN Range request")
	}

	// Close the first page while retaining its browser context.
	mutePageDiagnostics()
	if err := page.Close(); err != nil {
		t.Fatalf("close first lazy-plugin fixture page: %v", err)
	}

	// Open a restart page with diagnostics and cleanup.
	restartPage, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("create lazy-plugin restart page: %v", err)
	}
	muteRestartPageDiagnostics := testHarness.attachPageDiagnostics(t, restartPage)
	t.Cleanup(func() {
		muteRestartPageDiagnostics()
		_ = restartPage.Close()
	})

	// Clear the HTTP cache and reject Release World pack delivery on restart.
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

	// Boot the restart page with remote pack delivery disabled.
	if _, err := restartPage.Goto(testHarness.getBaseURL() + "/"); err != nil {
		dumpPageState(t, restartPage)
		t.Fatalf("durable local restart failed with Release World pack requests aborted: %v", err)
	}
	waitForPrerenderRoot(t, restartPage)
	waitForBootFunction(t, restartPage)
	if _, err := restartPage.Evaluate(`() => globalThis.__swBoot('#/')`); err != nil {
		t.Fatalf("boot lazy-plugin fixture after Release World pack route: %v", err)
	}

	// Verify the cached CLI plugin reaches its terminal prompt.
	waitForLiveApp(t, restartPage)
	waitForPluginWorkersRunning(t, restartPage, []string{
		"plugin/spacewave-cli-plugin",
	})
	waitForCliTerminalPrompt(t, restartPage)

	// Verify restart uses the durable cache without remote pack requests.
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

	// Close the cached restart page.
	muteRestartPageDiagnostics()
	if err := restartPage.Close(); err != nil {
		t.Fatalf("close restart lazy-plugin fixture page: %v", err)
	}

	// Open a fresh browser context for the Drive quickstart.
	freshContext, err := testHarness.browser.NewContext(testHarness.newContextOptions(t))
	if err != nil {
		t.Fatalf("create fresh quickstart browser context: %v", err)
	}
	t.Cleanup(func() {
		if err := freshContext.Close(); err != nil {
			t.Logf("close fresh quickstart browser context: %v", err)
		}
	})

	// Navigate a fresh page to the Drive quickstart.
	freshPage, err := freshContext.NewPage()
	if err != nil {
		t.Fatalf("create fresh quickstart page: %v", err)
	}
	muteFreshPageDiagnostics := testHarness.attachPageDiagnostics(t, freshPage)
	if _, err := freshPage.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		dumpPageState(t, freshPage)
		t.Fatalf("goto fresh quickstart drive: %v", err)
	}

	// Wait for the fresh Drive quickstart frame.
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

	// Verify the fresh Drive quickstart reaches content readiness.
	if _, err := waitForQuickstartDriveContentReady(t, freshPage); err != "" {
		dumpPageState(t, freshPage)
		t.Fatalf("fresh quickstart Drive content-ready failed: %s", err)
	}

	// Close the fresh quickstart page after its proof.
	muteFreshPageDiagnostics()
	if err := freshPage.Close(); err != nil {
		t.Fatalf("close fresh quickstart page: %v", err)
	}
}

func releaseWorldFixtureConfig(t *testing.T) (releaseWorldConfigValues, error) {
	// Evaluate the release fixture's Bldr configuration.
	t.Helper()
	result, err := bldr_project_starlark.Evaluate(filepath.Join(testHarness.repoRoot, "bldr.star"))
	if err != nil {
		return releaseWorldConfigValues{}, err
	}

	// Select the lazy-plugin fixture and its distribution override.
	build := result.Config.GetBuild()["release-web-lazy-plugin-fixture"]
	if build == nil {
		return releaseWorldConfigValues{}, errors.New("missing release-web-lazy-plugin-fixture build")
	}
	distOverride := build.GetManifestOverrides()["spacewave-browser"]
	if distOverride == nil {
		return releaseWorldConfigValues{}, errors.New("missing lazy fixture distribution override")
	}

	// Decode the distribution configuration and select its Release World.
	var distConf dist_compiler.Config
	if err := distConf.UnmarshalJSON(distOverride.GetConfig()); err != nil {
		return releaseWorldConfigValues{}, errors.Wrap(err, "decode lazy fixture distribution config")
	}
	hostConfig := distConf.GetHostConfigSet()["release-world"]
	if hostConfig == nil {
		return releaseWorldConfigValues{}, errors.New("missing lazy fixture Release World host config")
	}

	// Decode the fixture's Release World configuration.
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
	// Create a local quickstart session for the CLI proof.
	t.Helper()
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/quickstart/local"); err != nil {
		t.Fatalf("open local quickstart for CLI terminal proof: %v", err)
	}
	if _, err := page.WaitForFunction(`() => /^#\/u\/\d+\/?$/.test(window.location.hash)`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("local quickstart did not create a session for CLI RPC proof: %v", err)
	}

	// Read and validate the local session route.
	hash, err := page.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read local session route for CLI RPC proof: %v", err)
	}
	hashString, ok := hash.(string)
	if !ok {
		t.Fatalf("local session route has unexpected type %T", hash)
	}

	// Extract the session index for the CLI settings route.
	sessionIndex := strings.TrimSuffix(strings.TrimPrefix(hashString, "#/u/"), "/")
	if sessionIndex == "" {
		t.Fatalf("local session route %q has no session index for CLI RPC proof", hashString)
	}

	// Open the session's CLI settings and wait for its terminal action.
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/u/" + sessionIndex + "/settings/cli"); err != nil {
		t.Fatalf("open CLI settings for session %s: %v", sessionIndex, err)
	}
	openCLIButton := page.Locator("button:has-text('Open CLI terminal')").First()
	if err := openCLIButton.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("CLI settings did not expose Open CLI terminal: %v", err)
	}

	// Open the CLI terminal and wait for its route.
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

	// Wait for the terminal screen and the CLI stream prompt.
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
	// Wait for the plugin manifest copy's terminal state.
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

	// Decode and verify the plugin manifest copy completed.
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
	// Require the GoScript compiler for the Drive module probe.
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("set %s=true to run GoScript quickstart Drive app module probe", E2EReleaseWasmGoScriptEnv)
	}

	// Open the release root with HTTP tracing.
	t.Setenv("E2E_RELEASE_WASM_HTTP_TRACE", "1")
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the Drive quickstart from the release root.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/quickstart/drive')
	}`)
	if err != nil {
		t.Fatalf("start root production goscript bundle: %v", err)
	}

	// Wait for the core plugins and the Drive app module.
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

func waitForQuickstartDriveAppModule(t *testing.T, page playwright.Page) {
	// Wait for the Drive app module to load or fail.
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

	// Decode the Drive app module probe result.
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

	// Collect module delivery evidence when the Drive module fails.
	if state["state"] == "failed" {
		modulePath, _ := state["modulePath"].(string)
		if modulePath != "" {
			collectQuickstartModuleLoadDifferential(t, page, modulePath)
		}
		dumpPageState(t, page)
		t.Fatalf("quickstart Drive app module failed to load: %v", state["modulePath"])
	}

	// Record the loaded Drive app module state.
	t.Logf("quickstart Drive app module loaded: %#v", state)
}

func collectQuickstartModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) {
	// Compare browser module delivery with the server and release artifact.
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

	// Serialize and preserve the module delivery comparison.
	var arena fastjson.Arena
	reportJSON := report.appendJSON(&arena).MarshalTo(nil)
	t.Logf("quickstart module load differential: %s", string(reportJSON))
	writeModuleLoadDifferentialArtifact(t, string(reportJSON))

	// Verify browser module delivery is complete and matches the artifact.
	assertBrowserModuleFetchComplete(t, "root App module", browserProbe.RootFetch)
	assertBrowserModuleFetchMatchesArtifact(t, "Sonner module", browserProbe.SonnerFetch, sonnerArtifact)
}

func collectBrowserModuleLoadDifferential(t *testing.T, page playwright.Page, modulePath string) browserModuleLoadDifferential {
	// Collect browser fetch and import evidence for the app and Sonner modules.
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

	// Decode the browser module comparison payload.
	encoded, ok := raw.(string)
	if !ok {
		t.Fatalf("unexpected browser module load differential payload %T", raw)
	}

	// Parse the browser module comparison record.
	probe, err := parseBrowserModuleLoadDifferential(encoded)
	if err != nil {
		t.Fatalf("parse browser module load differential payload: %v", err)
	}
	return probe
}

func directServerModuleProbe(t *testing.T, modulePath string) moduleBodyProbe {
	// Require an absolute module request path.
	t.Helper()
	if !strings.HasPrefix(modulePath, "/") {
		t.Fatalf("module path must be absolute: %q", modulePath)
	}

	// Build the direct server request for the module.
	req, err := http.NewRequest(http.MethodGet, testHarness.getBaseURL()+modulePath, nil)
	if err != nil {
		t.Fatalf("build direct module request %q: %v", modulePath, err)
	}

	// Fetch the module response and release its body.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("direct module request %q: %v", modulePath, err)
	}
	defer resp.Body.Close()

	// Read the complete direct module response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read direct module response %q: %v", modulePath, err)
	}

	// Attach HTTP response metadata to the module body summary.
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.Status = resp.StatusCode
	probe.ContentType = resp.Header.Get("Content-Type")
	probe.ContentLength = resp.Header.Get("Content-Length")
	return probe
}

func releaseWebPkgArtifactProbe(t *testing.T, modulePath string) moduleBodyProbe {
	// Read the module's release package artifact.
	t.Helper()
	artifactRelPath := releaseWebPkgArtifactRelPath(t, modulePath)
	body, err := os.ReadFile(filepath.Join(testHarness.repoRoot, artifactRelPath))
	if err != nil {
		t.Fatalf("read release web package artifact %s for %s: %v", artifactRelPath, modulePath, err)
	}

	// Attach the artifact identity to its module body summary.
	probe := summarizeModuleBody(body)
	probe.Path = modulePath
	probe.ArtifactRelPath = artifactRelPath
	probe.OK = true
	return probe
}

func releaseWebPkgArtifactRelPath(t *testing.T, modulePath string) string {
	// Require the module path to name a release web package.
	t.Helper()
	const prefix = "/b/pkg/"
	if !strings.HasPrefix(modulePath, prefix) {
		t.Fatalf("release web package artifact path must begin with %s: %q", prefix, modulePath)
	}

	// Normalize the package path and reject escapes from its artifact root.
	pkgPath := strings.TrimPrefix(modulePath, prefix)
	cleanPkgPath := path.Clean(pkgPath)
	if cleanPkgPath == "." || cleanPkgPath == ".." || strings.HasPrefix(cleanPkgPath, "../") || path.IsAbs(cleanPkgPath) {
		t.Fatalf("release web package artifact path escapes package root: %q", modulePath)
	}
	return filepath.ToSlash(filepath.Join(webPkgArtifactRelDir, filepath.FromSlash(cleanPkgPath)))
}

func summarizeModuleBody(body []byte) moduleBodyProbe {
	// Hash the module body and retain its leading diagnostic text.
	sum := sha256.Sum256(body)
	bodyText := string(body)
	head := bodyText
	if len(head) > 160 {
		head = head[:160]
	}

	// Retain the module body's trailing diagnostic text.
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
	// Verify browser module delivery succeeded through the complete body.
	t.Helper()
	if browser.Status != http.StatusOK {
		t.Fatalf("%s browser probe did not return 200: %#v", label, browser)
	}
	if !browser.OK {
		t.Fatalf("%s browser body failed after headers: %#v", label, browser)
	}
}

func assertBrowserModuleFetchMatchesArtifact(t *testing.T, label string, browser moduleFetchProbe, artifact moduleBodyProbe) {
	// Verify the browser module body matches the release artifact.
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
	// Require a configured artifact directory for module evidence.
	t.Helper()
	if testHarness == nil || testHarness.artifactDir == "" {
		return
	}

	// Create the module comparison artifact's parent directory.
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	path := filepath.Join(
		testHarness.artifactDir,
		replacer.Replace(strings.ToLower(t.Name()))+"-module-load-differential.json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("write module load differential artifact mkdir %s: %v", path, err)
		return
	}

	// Write and report the module comparison artifact.
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		t.Logf("write module load differential artifact %s: %v", path, err)
		return
	}
	t.Logf("module load differential artifact: %s", path)
}

func TestProductionRuntimeMatchesReleaseDescriptor(t *testing.T) {
	// Read the descriptor for the runtime identity comparison.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Open the release root for the runtime comparison.
	page := testHarness.newPage(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/"); err != nil {
		t.Fatalf("goto root: %v", err)
	}

	// Boot the production runtime from its prerendered root.
	waitForPrerenderRoot(t, page)
	waitForBootFunction(t, page)
	_, err = page.Evaluate(`() => {
		globalThis.__swBoot('#/')
	}`)
	if err != nil {
		t.Fatalf("start root production wasm: %v", err)
	}
	waitForLiveApp(t, page)

	// Read the running generation and ServiceWorker identities.
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

	// Decode the runtime identity probe.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected production runtime state %T", raw)
	}

	// Verify the running generation matches the release descriptor.
	generationID, _ := state["generationId"].(string)
	if generationID != desc.GenerationID {
		t.Fatalf("generation id=%q want %q", generationID, desc.GenerationID)
	}

	// Verify the controlling and active ServiceWorkers match the release descriptor.
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
	// Read the release descriptor for the quickstart smoke artifact.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Remove the previous quickstart smoke artifact.
	path := testHarness.quickstartSmokeArtifactPath(t)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove previous quickstart smoke artifact: %v", err)
	}

	// Open a traced Drive quickstart page with its source revision.
	source := sourceRevision(t)
	page := testHarness.newPage(t)
	traceCapture := beginQuickstartRuntimeTrace(t, page)
	defer traceCapture.cleanup(t)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}
	enableQuickstartTimingLogs(t, page)

	// Wait for the prerendered Drive quickstart to mount its frame.
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

	// Record Drive content and invitation readiness.
	driveFrameReadyMs := browserNowMs(t, page)
	driveContentReadyMs, driveContentReadyError := waitForQuickstartDriveContentReady(t, page)
	if driveContentReadyError != "" {
		t.Logf("quickstart content-ready not reached: %s", driveContentReadyError)
	}
	driveGoldenPathReadyMs, driveGoldenPathError := exerciseQuickstartDriveGoldenPath(t, page)
	if driveGoldenPathError != "" {
		t.Logf("quickstart golden path not reached: %s", driveGoldenPathError)
	}

	// Measure the post-load workload and foreground resume, then stop tracing.
	postLoadSOWorkload := runQuickstartPostLoadSOWorkload(t, page, driveContentReadyMs != nil)
	foregroundResume := collectForegroundResumeEvidence(t, page)
	logQuickstartTiming(t, page)
	runtimeTrace := traceCapture.stop(t)

	// Collect the quickstart smoke artifact from the measured browser state.
	data, err := collectQuickstartSmokeArtifact(page, desc, source, driveFrameReadyMs, driveContentReadyMs, driveContentReadyError, driveGoldenPathReadyMs, driveGoldenPathError, runtimeTrace, postLoadSOWorkload, foregroundResume)
	if err != nil {
		t.Fatalf("collect quickstart smoke artifact: %v", err)
	}

	// Preserve and report the quickstart smoke artifact.
	if err := writeQuickstartSmokeArtifact(path, data); err != nil {
		t.Fatalf("write quickstart smoke artifact: %v", err)
	}
	t.Logf("quickstart smoke artifact written to %s (%d bytes)", path, len(data))
}

func TestQuickstartSecondTabReusesRuntimeAndCloseKeepsFirstTab(t *testing.T) {
	// Open the first Drive quickstart page.
	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto first quickstart drive: %v", err)
	}

	// Wait for the first Drive quickstart frame.
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

	// Mark the first page to detect reloads and cross-tab navigation.
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

	// Open a second Drive page in the retained browser context.
	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(quickstartURL); err != nil {
		t.Fatalf("goto second quickstart drive: %v", err)
	}

	// Wait for the second Drive quickstart frame.
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

	// Close the second Drive page and foreground the first.
	if err := pageB.Close(); err != nil {
		t.Fatalf("close second quickstart tab: %v", err)
	}
	if err := pageA.BringToFront(); err != nil {
		t.Fatalf("bring first quickstart tab to front: %v", err)
	}

	// Inspect the first page after the second page closes.
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

	// Decode the surviving page's state.
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected first tab close state %T", raw)
	}

	// Verify the first page retains its URL, document and Drive frame.
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
	// Seed an obsolete Shell snapshot into the first document.
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

	// Boot the first document as the dedicated-worker host.
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, pageA, quickstartURL)
	assertRuntimeWorkerMode(t, pageA, "dedicated-worker")
	hostGeneration, hostDocumentID := assertDedicatedWorkerHost(t, pageA)
	assertWarmPresentation(t, pageA, hostGeneration, hostDocumentID, false)

	// Check the obsolete snapshot was reset, not imported.
	waitForShellRecordCount(t, pageA, 1)
	initialSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(initialSnapshot.Records) != 1 {
		t.Fatalf("old Shell state was not cleanly initialized: %#v", initialSnapshot)
	}
	if initialSnapshot.Records[0].ID == "legacy-tab" || initialSnapshot.Records[0].Path == "/legacy" {
		t.Fatalf("legacy Shell record was imported instead of reset: %#v", initialSnapshot.Records[0])
	}

	// Check the obsolete storage key was removed.
	legacyAfterInit, err := pageA.Evaluate(`() => sessionStorage.getItem('shell-tabs-state')`)
	if err != nil {
		t.Fatalf("read obsolete Shell snapshot after clean initialization: %v", err)
	}
	if legacyAfterInit != nil {
		t.Fatalf("obsolete shell-tabs-state survived clean initialization: %#v", legacyAfterInit)
	}
	firstURL := pageA.URL()

	// Attach a second document to the warm host.
	pageB := testHarness.newPageInContext(t, ctx)
	openQuickstartReleasePage(t, pageB, quickstartURL)
	assertRuntimeWorkerMode(t, pageB, "dedicated-worker")
	assertWarmPresentation(t, pageB, hostGeneration, hostDocumentID, true)
	waitForShellRecordCount(t, pageA, 2)
	waitForShellRecordCount(t, pageB, 2)
	logWarmAttachCorrectnessMetrics(t, pageB, hostGeneration)

	// Check the second document added one shared record without taking the first URL.
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

	// Move the shared record to /docs from the second document.
	logShellDiagnostic(t, pageA, "before_docs_hash_page_a")
	logShellDiagnostic(t, pageB, "before_docs_hash_page_b")
	setShellHash(t, pageB, "#/docs")
	logShellDiagnostic(t, pageB, "after_docs_hash_page_b")
	waitForBrowserShellRecordPath(t, pageB, secondRecordID, "/docs")
	if pageA.URL() != firstURL {
		t.Fatalf("inactive shared path update changed first document hash: got %s want %s", pageA.URL(), firstURL)
	}

	// Rename the shared record and check both fields converge.
	renameActiveShellTab(t, pageB, "Shared Docs")
	waitForShellLabel(t, pageA, "Shared Docs")
	sharedSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	sharedRecord := findBrowserShellRecord(sharedSnapshot, secondRecordID)
	if sharedRecord == nil || sharedRecord.Path != "/docs" || sharedRecord.CustomName != "Shared Docs" {
		t.Fatalf("shared path/name/customName did not converge: %#v", sharedSnapshot)
	}

	// Create one tab in each document at once.
	beforeConcurrentA := readComposedShellProjection(t, pageA)
	beforeConcurrentB := readComposedShellProjection(t, pageB)
	concurrentCreateShellTabs(t, pageA, pageB)
	waitForShellRecordCount(t, pageA, 4)
	waitForShellRecordCount(t, pageB, 4)

	// Check both documents hold the same four records.
	concurrentSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(concurrentSnapshot.Records) != 4 {
		t.Fatalf("concurrent Shell creation lost a record: %#v", concurrentSnapshot)
	}
	if !sameBrowserShellRecordIDs(t, concurrentSnapshot, readBrowserShellTabsSnapshot(t, pageB)) {
		t.Fatalf("A/B shared record inventories diverged after concurrent creation")
	}

	// Route each document's new tab to its own static page.
	waitForBrowserShellActiveRecordChange(t, pageA, beforeConcurrentA.ActiveTabID)
	waitForBrowserShellActiveRecordChange(t, pageB, beforeConcurrentB.ActiveTabID)
	setShellHash(t, pageA, "#/pricing")
	setShellHash(t, pageB, "#/licenses")
	waitForBrowserShellActivePath(t, pageA, "/pricing")
	waitForBrowserShellActivePath(t, pageB, "/licenses")
	if pageA.URL() == pageB.URL() {
		t.Fatalf("A/B active selection and hash are not independent: A=%s B=%s", pageA.URL(), pageB.URL())
	}

	// Record page A's settled projection.
	waitForShellLabel(t, pageA, "Shared Docs")
	waitForShellLabel(t, pageB, "Shared Docs")
	independentProjectionA := readComposedShellProjection(t, pageA)

	// Pop out the shared record with its retained Shell Tab ID.
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

	// Check the popout kept the retained ID and the shared record.
	openQuickstartReleasePage(t, popup, popup.URL())
	retainedURL = popup.URL()
	if !strings.Contains(retainedURL, "shellTabId="+secondRecordID) {
		t.Fatalf("popout URL lost retained Shell Tab ID: %s", retainedURL)
	}
	waitForShellRecordCount(t, popup, 4)
	waitForShellLabel(t, popup, "Shared Docs")

	// Open the retained-ID URL in a copied document.
	copied := testHarness.newPageInContext(t, ctx)
	if _, err := copied.Goto(retainedURL); err != nil {
		t.Fatalf("goto copied retained-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, copied, retainedURL)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")

	// Reload the copied document and check it keeps the shared record.
	if _, err := copied.Reload(); err != nil {
		t.Fatalf("reload copied retained-ID URL: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, copied)
	waitForLiveApp(t, copied)
	waitForShellTabButtons(t, copied)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")

	// Check the retained-ID transitions left page A unchanged.
	beforeCloseA := readComposedShellProjection(t, pageA)
	assertSameComposedShellProjection(
		t,
		beforeCloseA,
		independentProjectionA,
		"retained-ID transitions changed page A",
	)

	// Close the shared record from page B while page A shows another tab.
	closeShellTabByText(t, pageB, "Shared Docs")
	waitForShellRecordCount(t, pageA, 3)
	waitForShellRecordCount(t, pageB, 3)
	afterCloseA := readComposedShellProjection(t, pageA)
	assertInactiveClosePreservedProjection(t, beforeCloseA, afterCloseA, "Shared Docs")
	assertShellLabelAbsent(t, pageA, "Shared Docs")
	assertShellLabelAbsent(t, pageB, "Shared Docs")

	// Open the retained URL after its record was removed.
	removedIDURL := retainedURL
	removed := testHarness.newPageInContext(t, ctx)
	if _, err := removed.Goto(removedIDURL); err != nil {
		t.Fatalf("goto removed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, removed, removedIDURL)

	// Check the closed record stays closed and a fresh /docs record replaces it.
	waitForShellRecordCount(t, removed, 4)
	removedSnapshot := readBrowserShellTabsSnapshot(t, removed)
	if findBrowserShellRecord(removedSnapshot, secondRecordID) != nil {
		t.Fatalf("removed-ID handoff resurrected the closed record: %#v", removedSnapshot)
	}
	if removedSnapshot.Records[len(removedSnapshot.Records)-1].Path != "/docs" {
		t.Fatalf("removed-ID fallback did not create a fresh /docs record: %#v", removedSnapshot)
	}

	// Open a malformed-ID URL, which falls back to a new record.
	invalidURL := strings.Replace(retainedURL, "shellTabId="+secondRecordID, "shellTabId=!malformed", 1)
	invalid := testHarness.newPageInContext(t, ctx)
	if _, err := invalid.Goto(invalidURL); err != nil {
		t.Fatalf("goto malformed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, invalid, invalidURL)
	waitForShellRecordCount(t, invalid, 5)
	invalidHash := invalid.URL()

	// Check a reload keeps the fallback URL stable.
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

	// Close the retained-ID proof documents.
	for _, page := range []playwright.Page{popup, copied, removed, invalid} {
		if err := page.Close(); err != nil {
			t.Fatalf("close completed retained-ID proof page: %v", err)
		}
	}

	// Open a document attached to the elected host.
	pageAttached := testHarness.newPageInContext(t, ctx)
	if _, err := pageAttached.Goto(retainedURL); err != nil {
		t.Fatalf("goto retained URL for host-loss proof: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, pageAttached)
	waitForBootFunction(t, pageAttached)
	waitForLiveApp(t, pageAttached)
	waitForStartupMark(t, pageAttached, "dedicated-host.attach-open-ready")

	// Close the host and check the survivor is promoted.
	if err := pageA.Close(); err != nil {
		t.Fatalf("close elected host document: %v", err)
	}
	promotionGeneration := assertWarmPromotion(t, pageB, hostGeneration)
	assertWarmPresentation(t, pageB, promotionGeneration, "", false)

	// Check the attached document rejoins the promoted host without a cold boot
	// and keeps the Shell inventory.
	assertReattachedWithoutColdBoot(t, pageAttached, promotionGeneration)
	waitForShellRecordCount(t, pageAttached, 6)
	expectedInventory := browserShellRecordIDs(readBrowserShellTabsSnapshot(t, pageAttached))
	if !sameStringSet(browserShellRecordIDs(readBrowserShellTabsSnapshot(t, pageB)), expectedInventory) {
		t.Fatalf("Shell inventory did not survive host loss/promotion")
	}

	// Check the promoted survivor is usable.
	if err := pageB.BringToFront(); err != nil {
		t.Fatalf("bring survivor document to front: %v", err)
	}
	if err := pageB.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		t.Fatalf("wait for promoted survivor usability: %v", err)
	}

	// Close every document to reach zero documents.
	preservedSnapshot := readBrowserShellTabsSnapshot(t, pageB)
	for _, page := range ctx.Pages() {
		if page.IsClosed() {
			continue
		}
		if err := page.Close(); err != nil {
			t.Fatalf("close document before zero-document retention check: %v", err)
		}
	}

	// Reopen and check the retained records are unchanged plus one new route record.
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

	// Reset Shell tabs and check the epoch advances to one record.
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
	// Require Chromium for the persistent-profile restart proof.
	if testHarness.browserName != "chromium" {
		t.Skip("persistent-profile release proof requires Chromium")
	}

	// Open the first persistent browser context with cleanup.
	userDataDir := t.TempDir()
	firstContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	firstContextClosed := false
	t.Cleanup(func() {

		// Leave an explicitly closed persistent context alone during cleanup.
		if firstContextClosed {
			return
		}

		// Close the first persistent context if the test exits early.
		if err := firstContext.Close(); err != nil {
			t.Logf("close first persistent browser context during cleanup: %v", err)
		}
	})

	// Create a second Shell record in the persistent profile.
	firstPage := testHarness.newPageInContext(t, firstContext)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, firstPage, quickstartURL)
	waitForShellRecordCount(t, firstPage, 1)
	if err := firstPage.Locator("button[title='New tab']").First().Click(); err != nil {
		t.Fatalf("create persistent-profile Shell record: %v", err)
	}

	// Verify both Shell records exist before restarting the browser.
	waitForShellRecordCount(t, firstPage, 2)
	beforeRestart := readBrowserShellTabsSnapshot(t, firstPage)
	if len(beforeRestart.Records) != 2 {
		t.Fatalf("persistent-profile setup did not create two records: %#v", beforeRestart)
	}

	// Terminate the first browser context while preserving its profile.
	if err := firstContext.Close(); err != nil {
		t.Fatalf("close persistent browser context for restart: %v", err)
	}
	firstContextClosed = true

	// Reopen the retained browser profile with cleanup.
	secondContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	t.Cleanup(func() {
		if err := secondContext.Close(); err != nil {
			t.Logf("close relaunched persistent browser context: %v", err)
		}
	})

	// Verify browser restart preserves the Shell records and adds a fresh entry.
	secondPage := testHarness.newPageInContext(t, secondContext)
	openQuickstartReleasePage(t, secondPage, quickstartURL)
	waitForShellRecordCount(t, secondPage, len(beforeRestart.Records)+1)
	afterRestart := readBrowserShellTabsSnapshot(t, secondPage)
	if findNewBrowserShellRecord(beforeRestart, afterRestart) == "" ||
		!sameBrowserShellRecordValues(beforeRestart, afterRestart) {
		t.Fatalf("persistent-profile browser restart did not preserve records while creating fresh entry: before=%#v after=%#v", beforeRestart, afterRestart)
	}
	waitForShellLabel(t, secondPage, beforeRestart.Records[0].Name)

	// Reset the persistent Shell inventory and verify its epoch advances.
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
	// Collect the document's Shell routes, storage and visible panel.
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

	// Decode the Shell diagnostic record.
	record, err := decodeShellDiagnosticRecord(raw)
	if err != nil {
		t.Fatalf("shell_diagnostic label=%s decode_error=%v payload_type=%T", label, err, raw)
	}

	// Report the Shell diagnostic record with its collection label.
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
	// Require an object payload for the Shell diagnostic record.
	value, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticRecord{}, errors.Errorf("expected object, got %T", raw)
	}

	// Decode the Shell document's location fields.
	locationHref, err := requiredShellDiagnosticString(value, "locationHref", true)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	locationHash, err := requiredShellDiagnosticString(value, "locationHash", false)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}

	// Decode the document and shared Shell storage records.
	sessionDocumentState, err := decodeShellDiagnosticStorage(value, "sessionDocumentState")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	browserShellTabs, err := decodeShellDiagnosticStorage(value, "browserShellTabs")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}

	// Decode the Shell document's visible active panel identity.
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
	// Require the requested Shell diagnostic field to exist.
	raw, ok := value[field]
	if !ok {
		return "", errors.Errorf("missing %s", field)
	}

	// Require a string value for the Shell diagnostic field.
	text, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("%s has type %T, want string", field, raw)
	}

	// Enforce the Shell diagnostic field's nonempty contract.
	if nonEmpty && text == "" {
		return "", errors.Errorf("%s is empty", field)
	}
	return text, nil
}

func decodeShellDiagnosticStorage(value map[string]any, field string) (shellDiagnosticStorage, error) {
	// Require the Shell storage diagnostic field to be an object.
	raw, ok := value[field]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s", field)
	}
	storage, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s has type %T, want object", field, raw)
	}

	// Decode the storage presence flag and raw snapshot value.
	present, ok := storage["present"].(bool)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s.present has type %T, want bool", field, storage["present"])
	}
	rawValue, ok := storage["raw"]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s.raw", field)
	}

	// Verify a null storage value agrees with the presence flag.
	if rawValue == nil {
		if present {
			return shellDiagnosticStorage{}, errors.Errorf("%s.raw is null while present", field)
		}
		return shellDiagnosticStorage{}, nil
	}

	// Verify a non-null storage value is a present string snapshot.
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
	// Open the requested release page and wait for the live application.
	t.Helper()
	if _, err := page.Goto(targetURL); err != nil {
		t.Fatalf("goto composed Shell page %s: %v", targetURL, err)
	}
	waitForPrerenderRootOrLiveApp(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)

	// Wait for the Drive frame when the target is a Drive quickstart.
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

	// Wait for Shell tabs on other release routes.
	waitForShellTabButtons(t, page)
}

func readBrowserShellTabsSnapshot(t *testing.T, page playwright.Page) composedShellSnapshot {
	// Read the persisted browser Shell Tabs snapshot.
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const value = localStorage.getItem('browser-shell-tabs')
		return value ? JSON.parse(value) : null
	}`)
	if err != nil {
		t.Fatalf("read browser Shell Tabs snapshot: %v", err)
	}

	// Decode the Shell snapshot's metadata and record inventory.
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

	// Decode each stored Shell record into the snapshot.
	snapshot.Records = make([]composedShellRecord, 0, len(rawRecords))
	for _, rawRecord := range rawRecords {

		// Require each Shell record to be an object.
		record, ok := rawRecord.(map[string]any)
		if !ok {
			t.Fatalf("browser Shell Tabs record invalid: %#v", rawRecord)
		}

		// Append the Shell record's identity and presentation fields.
		snapshot.Records = append(snapshot.Records, composedShellRecord{
			ID:               composedString(record["id"]),
			Path:             composedString(record["path"]),
			Name:             composedString(record["name"]),
			CustomName:       composedString(record["customName"]),
			CreationSequence: composedNumber(record["creationSequence"]),
		})
	}

	// Verify the browser Shell snapshot uses the expected schema.
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
	// Index the Shell record IDs present before the operation.
	beforeIDs := make(map[string]bool, len(before.Records))
	for _, record := range before.Records {
		beforeIDs[record.ID] = true
	}

	// Identify the sole added Shell record.
	var added string
	for _, record := range after.Records {
		if !beforeIDs[record.ID] {
			if added != "" {
				return ""
			}
			added = record.ID
		}
	}

	// Verify the Shell inventory grew by exactly one record.
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
	// Compare the Shell snapshots by their record IDs.
	t.Helper()
	leftIDs := browserShellRecordIDs(left)
	rightIDs := browserShellRecordIDs(right)
	return sameStringSet(leftIDs, rightIDs)
}

func sameBrowserShellRecordValues(before, after composedShellSnapshot) bool {
	// Index the updated Shell records by identity.
	afterByID := make(map[string]composedShellRecord, len(after.Records))
	for _, record := range after.Records {
		afterByID[record.ID] = record
	}

	// Verify every retained Shell record preserves its fields.
	for _, record := range before.Records {
		if afterRecord, ok := afterByID[record.ID]; !ok || afterRecord != record {
			return false
		}
	}
	return true
}

func sameStringSet(left, right []string) bool {
	// Require equal cardinality before comparing string sets.
	if len(left) != len(right) {
		return false
	}

	// Index the first string set's members.
	values := make(map[string]bool, len(left))
	for _, value := range left {
		values[value] = true
	}

	// Verify every member of the second string set is present.
	for _, value := range right {
		if !values[value] {
			return false
		}
	}
	return true
}

func waitForShellRecordCount(t *testing.T, page playwright.Page, count int) {
	// Wait for the persisted Shell inventory to reach the expected size.
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
	// Record the Shell document before waiting for a shared path update.
	t.Helper()
	t.Logf("shell_record_path_wait phase=before target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_before")

	// Wait for the shared Shell record to reach its target path.
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

	// Record the Shell document after its shared path update.
	t.Logf("shell_record_path_wait phase=success target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_success")
}

func waitForBrowserShellActiveRecordChange(t *testing.T, page playwright.Page, previousID string) {
	// Wait for the newly created Shell record to become active.
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
	// Wait for the active Shell record and document hash to agree.
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
	// Wait for a visible Shell tab button.
	t.Helper()
	tabs := page.Locator(".flexlayout__tab_button:visible").First()
	if err := tabs.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for visible Shell tab buttons: %v", err)
	}

	// Verify the visible Shell tab button accepts interaction.
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
	// Read the document's visible Shell projection.
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

	// Decode the Shell projection and its visible labels.
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("visible Shell projection invalid: %#v", raw)
	}
	rawLabels, ok := value["labels"].([]any)
	if !ok {
		t.Fatalf("visible Shell labels invalid: %#v", value)
	}

	// Collect the visible Shell labels in their displayed order.
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

	// Verify the document's Shell projection matches the expected state.
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

	// Remove the closed label from the expected Shell projection.
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

	// Verify the closed label existed in the original projection.
	if !removed {
		t.Fatalf("inactive close precondition lacks label %q: %#v", removedLabel, before)
	}

	// Verify closing an inactive Shell record preserved the document selection.
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
	// Verify the removed Shell label is absent from the visible tabs.
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
	// Open the active Shell tab's rename editor.
	t.Helper()
	tabs := page.Locator(".flexlayout__tab_button")
	if err := tabs.Last().Dblclick(); err != nil {
		t.Fatalf("start visible Shell tab rename: %v", err)
	}

	// Enter and commit the Shell tab's custom name.
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
	// Open the selected Shell tab's context menu.
	t.Helper()
	tab := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First()
	if err := tab.Click(playwright.LocatorClickOptions{Button: playwright.MouseButtonRight}); err != nil {
		t.Fatalf("open Shell tab context menu %q: %v", label, err)
	}

	// Invoke the context menu action that closes the shared tab.
	closeItem := page.Locator("[role='menuitem']:visible").Filter(playwright.LocatorFilterOptions{
		HasText: "Close Tab",
	}).First()
	if err := closeItem.Click(); err != nil {
		t.Fatalf("close shared Shell tab %q: %v", label, err)
	}
}

func concurrentCreateShellTabs(t *testing.T, pageA, pageB playwright.Page) {
	// Create Shell records concurrently in both documents.
	t.Helper()
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, page := range []playwright.Page{pageA, pageB} {

		// Track the lifetime of each document's tab creation.
		waitGroup.Add(1)
		go func(page playwright.Page) {

			// Click the document's visible New tab button and report its result.
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

	// Wait for both Shell tab creations and report their failures.
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent visible Shell record creation: %v", err)
		}
	}
}

// resetShellTabsVisibly resets the Shell tabs through the View menu of a
// quickstart Drive page.
func resetShellTabsVisibly(t *testing.T, page playwright.Page) {
	// Report visible Shell reset failures at the caller.
	t.Helper()

	// Wait for the quickstart to settle on the Drive file browser. Its route
	// change closes an open menu, detaching the item before the click.
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		t.Fatalf("wait for Drive file browser before Shell reset: %v", err)
	}

	// Open the View menu and invoke the reset.
	if err := page.Locator("button:visible:has-text('View')").First().Click(); err != nil {
		t.Fatalf("open visible View menu: %v", err)
	}
	reset := page.Locator("[role='menuitem']:visible:has-text('Reset Shell Tabs')").First()
	if err := reset.Click(); err != nil {
		t.Fatalf("invoke visible Shell reset: %v", err)
	}
}

func readComposedStartupMarks(t *testing.T, page playwright.Page) []composedStartupMark {
	// Read the production document's startup marks.
	t.Helper()
	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? {},
	}))`)
	if err != nil {
		t.Fatalf("read production startup marks: %v", err)
	}

	// Decode the startup mark inventory.
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("production startup marks invalid: %#v", raw)
	}

	// Collect valid startup marks and their details.
	marks := make([]composedStartupMark, 0, len(items))
	for _, item := range items {

		// Ignore startup mark entries with an invalid object shape.
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Append the startup mark's label, sequence and detail.
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
	// Find the newest startup mark with the requested label.
	t.Helper()
	marks := readComposedStartupMarks(t, page)
	for _, mark := range slices.Backward(marks) {
		if mark.Label == label {
			return mark
		}
	}

	// Fail the proof when its required startup mark is absent.
	t.Fatalf("production startup mark %q is missing", label)
	return composedStartupMark{}
}

func waitForStartupMark(t *testing.T, page playwright.Page, label string) {
	// Wait for the requested production startup mark.
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
	// Verify the host lease includes its generation and document identity.
	t.Helper()
	lease := lastComposedStartupMark(t, page, "dedicated-host.lease-acquired")
	generation := composedString(lease.Detail["generation"])
	documentID := composedString(lease.Detail["documentId"])
	if generation == "" || documentID == "" {
		t.Fatalf("host lease mark lacks generation/document identity: %#v", lease)
	}

	// Verify the host constructed exactly one dedicated runtime worker.
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
	// Verify an attached document acknowledges the expected host generation.
	t.Helper()
	marks := readComposedStartupMarks(t, page)
	if requireAttachment {
		ready := lastComposedStartupMark(t, page, "dedicated-host.attach-open-ready")
		if composedString(ready.Detail["hostGeneration"]) != generation ||
			composedString(ready.Detail["hostDocumentId"]) != hostDocumentID {
			t.Fatalf("attach acknowledged an unexpected runtime generation/host: %#v want generation=%q host=%q", ready, generation, hostDocumentID)
		}
	}

	// Find the latest connection to the current runtime generation.
	var connected composedStartupMark
	for _, mark := range slices.Backward(marks) {
		if mark.Label == "runtime.connected" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation {
			connected = mark
			break
		}
	}
	if connected.Label == "" {
		t.Fatalf("runtime.connected did not carry current generation %q: %#v", generation, marks)
	}

	// Find the current generation's neutral frame and subsequent reveal.
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

	// Verify the neutral frame precedes the current generation's reveal.
	if neutral.Label == "" || reveal.Label == "" || neutral.Sequence >= reveal.Sequence {
		t.Fatalf("generation-matched neutral/reveal order missing for %q: connected=%#v neutral=%#v reveal=%#v marks=%#v", generation, connected, neutral, reveal, marks)
	}

	// Verify no connection invalidation followed the current connection.
	for _, mark := range marks {
		if mark.Label == "runtime.connection-invalidated" && mark.Sequence >= connected.Sequence {
			t.Fatalf("runtime presentation advanced before obsolete generation invalidation: invalidated=%#v connected=%#v", mark, connected)
		}
	}

	// Read and verify the document's resume readiness.
	raw, err := page.Evaluate(`() => globalThis.__swWebDocumentResumeReady ?? null`)
	if err != nil {
		t.Fatalf("read production resume-ready state: %v", err)
	}
	resume, ok := raw.(map[string]any)
	if !ok || resume["ready"] != true {
		t.Fatalf("production resume-ready surface is not ready: %#v", raw)
	}

	// Verify resume readiness names the selected runtime.
	runtimeMark := lastComposedStartupMark(t, page, "runtime.mode-selected")
	if composedString(resume["runtimeId"]) != composedString(runtimeMark.Detail["runtimeId"]) {
		t.Fatalf("resume-ready runtime identity drifted: resume=%#v mode=%#v", resume, runtimeMark)
	}
}

func assertWarmPromotion(t *testing.T, page playwright.Page, oldGeneration string) string {
	// Wait for promotion to a replacement host generation.
	t.Helper()
	waitForStartupMark(t, page, "dedicated-host.promoted")
	promoted := lastComposedStartupMark(t, page, "dedicated-host.promoted")
	generation := composedString(promoted.Detail["generation"])
	if generation == "" || generation == oldGeneration {
		t.Fatalf("promotion did not replace runtime generation: old=%q mark=%#v", oldGeneration, promoted)
	}

	// Wait for old-generation invalidation and replacement connection.
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

	// Locate the first invalidation and connection after promotion.
	marks := readComposedStartupMarks(t, page)
	var invalidated, connected composedStartupMark
	for _, mark := range marks {

		// Find the old generation's first post-promotion invalidation.
		if mark.Label == "runtime.connection-invalidated" &&
			composedString(mark.Detail["runtimeGeneration"]) == oldGeneration &&
			mark.Sequence > promoted.Sequence &&
			(invalidated.Label == "" || mark.Sequence < invalidated.Sequence) {
			invalidated = mark
		}

		// Find the new generation's first post-promotion connection.
		if mark.Label == "runtime.connected" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > promoted.Sequence &&
			(connected.Label == "" || mark.Sequence < connected.Sequence) {
			connected = mark
		}
	}

	// Verify promotion invalidates the old runtime before connecting the replacement.
	if invalidated.Label == "" || connected.Label == "" || invalidated.Sequence >= connected.Sequence {
		t.Fatalf("promotion did not invalidate old runtime before new connection: old=%q new=%q promoted=%#v invalidated=%#v connected=%#v marks=%#v", oldGeneration, generation, promoted, invalidated, connected, marks)
	}

	// Verify the obsolete generation cannot advance presentation after promotion.
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

// assertReattachedWithoutColdBoot waits for an attached document to rejoin the
// promoted host generation and checks it never started a runtime worker.
func assertReattachedWithoutColdBoot(t *testing.T, page playwright.Page, generation string) {
	// Wait for an attach to the promoted generation.
	t.Helper()
	_, err := page.WaitForFunction(`(generation) =>
		(globalThis.__swStartupMarks ?? []).some((mark) =>
			mark.label === 'dedicated-host.attach-open-ready' &&
			mark.detail?.hostGeneration === generation)`,
		generation, playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(browserWaitMS),
		})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for attached document to rejoin promoted host %q: %v", generation, err)
	}

	// An attached document relays to the host; a worker of its own is a cold boot.
	for _, mark := range readComposedStartupMarks(t, page) {
		if mark.Label == "runtime.worker-created" {
			t.Fatalf("attached document cold-booted a runtime worker after host loss: %#v", mark)
		}
	}
}

func logWarmAttachCorrectnessMetrics(t *testing.T, page playwright.Page, generation string) {
	// Read and report the warm attachment's readiness intervals.
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
	// Require GoScript for the return visitor route proof.
	if compiler, err := resolveReleaseWasmCompiler(); err != nil {
		t.Fatalf("resolve release wasm compiler: %v", err)
	} else if compiler != releaseWasmCompilerGoScript {
		t.Skipf("release-WASM return visitor body route gate requires %s=true", E2EReleaseWasmGoScriptEnv)
	}

	// Open the first Drive quickstart page.
	pageA := testHarness.newPage(t)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	if _, err := pageA.Goto(quickstartURL); err != nil {
		t.Fatalf("goto quickstart drive: %v", err)
	}

	// Wait for the initial Drive frame and its created route.
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

	// Read and validate the direct SharedObject route.
	hash, err := pageA.Evaluate(`() => window.location.hash`)
	if err != nil {
		t.Fatalf("read created direct route hash: %v", err)
	}
	hashText, ok := hash.(string)
	if !ok || !strings.HasPrefix(hashText, "#/u/") || !strings.Contains(hashText, "/so/") {
		t.Fatalf("quickstart did not produce direct SharedObject route hash: %#v", hash)
	}
	directURL := testHarness.getBaseURL() + "/" + hashText

	// Open the saved SharedObject route in a second document.
	pageB := testHarness.newPageInContext(t, pageA.Context())
	if _, err := pageB.Goto(directURL); err != nil {
		t.Fatalf("goto direct SharedObject route: %v", err)
	}

	// Wait for the return visitor's Drive frame.
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

	// Verify the return visitor mounts persisted content through the body route.
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

func assertReturnVisitorBodyRouteStartupMarks(t testing.TB, page playwright.Page) {
	// Read the return visitor's body route startup marks.
	t.Helper()
	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? null,
	}))`, nil)
	if err != nil {
		t.Fatalf("read return visitor body route startup marks: %v", err)
	}

	// Decode the return visitor's startup mark inventory.
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("unexpected return visitor body route startup marks %T: %#v", raw, raw)
	}

	// Collect labels from valid return visitor startup marks.
	labels := make([]string, 0, len(items))
	for _, item := range items {

		// Ignore startup marks with an invalid object shape.
		mark, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Retain the startup mark's string label.
		if label, ok := mark["label"].(string); ok {
			labels = append(labels, label)
		}
	}

	// Verify the return visitor mounted each required body route component.
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

	// Verify the return visitor did not consume quickstart handoffs.
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
	// Parse the browser module comparison JSON record.
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
	// Decode the module resource timing entries.
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
	// Decode the JSON string array.
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value.GetStringBytes()))
	}
	return out
}

func parseStringMapValue(value *fastjson.Value) map[string]string {
	// Require a JSON object for the string map.
	obj := value.GetObject()
	if obj == nil {
		return nil
	}

	// Decode the JSON object's string values.
	values := make(map[string]string, obj.Len())
	obj.Visit(func(key []byte, value *fastjson.Value) {
		values[string(key)] = string(value.GetStringBytes())
	})
	return values
}

func (r moduleLoadDifferentialReport) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module identity and browser comparison.
	obj := arena.NewObject()
	obj.Set("modulePath", arena.NewString(r.ModulePath))
	obj.Set("browserProbe", r.BrowserProbe.appendJSON(arena))

	// Serialize the direct server module probes.
	directServer := arena.NewObject()
	directServer.Set("root", r.DirectServer.Root.appendJSON(arena))
	directServer.Set("sonner", r.DirectServer.Sonner.appendJSON(arena))
	obj.Set("directServer", directServer)

	// Serialize the release package artifact probes.
	releaseArtifact := arena.NewObject()
	releaseArtifact.Set("sonner", r.ReleaseArtifact.Sonner.appendJSON(arena))
	obj.Set("releaseArtifact", releaseArtifact)
	return obj
}

func (p browserModuleLoadDifferential) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the browser location and module loading diagnostics.
	obj := arena.NewObject()
	obj.Set("location", arena.NewString(p.Location))
	obj.Set("controllerURL", arena.NewString(p.ControllerURL))
	setRawJSONValue(arena, obj, "rootAssetStatus", p.RootAssetStatus)
	setRawJSONValue(arena, obj, "moduleImportError", p.ModuleImportError)

	// Serialize browser module fetch and import results.
	obj.Set("rootFetch", p.RootFetch.appendJSON(arena))
	obj.Set("sonnerFetch", p.SonnerFetch.appendJSON(arena))
	obj.Set("rootImport", p.RootImport.appendJSON(arena))
	obj.Set("sonnerImport", p.SonnerImport.appendJSON(arena))

	// Serialize the browser module resource timing entries.
	entries := arena.NewArray()
	for _, entry := range p.PerformanceEntries {
		entries.SetArrayItem(len(entries.GetArray()), entry.appendJSON(arena))
	}
	obj.Set("performanceEntries", entries)
	return obj
}

func (p moduleFetchProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module fetch path, HTTP status and headers.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("headers", appendStringMapJSON(arena, p.Headers))

	// Serialize the complete module body's reader, size and fingerprint.
	setBoolJSONField(arena, obj, "bodyComplete", p.BodyComplete)
	obj.Set("bodyReader", arena.NewString(p.BodyReader))
	obj.Set("bodyChunks", arena.NewNumberInt(p.BodyChunks))
	obj.Set("bodyLength", arena.NewNumberInt(p.BodyLength))
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))

	// Serialize partial module body evidence and fetch failures.
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
	// Serialize the module import request and success status.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("requestURL", arena.NewString(p.RequestURL))
	setBoolJSONField(arena, obj, "ok", p.OK)

	// Serialize the imported module's export inventory.
	exportKeys := arena.NewArray()
	for _, key := range p.ExportKeys {
		exportKeys.SetArrayItem(len(exportKeys.GetArray()), arena.NewString(key))
	}
	obj.Set("exportKeys", exportKeys)

	// Serialize the module's default export and import failure details.
	setBoolJSONField(arena, obj, "hasDefault", p.HasDefault)
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("message", arena.NewString(p.Message))
	obj.Set("stack", arena.NewString(p.Stack))
	return obj
}

func (p modulePerformanceEntry) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module resource timing record.
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(p.Name))
	obj.Set("initiatorType", arena.NewString(p.InitiatorType))
	obj.Set("transferSize", arena.NewNumberInt(p.TransferSize))
	obj.Set("encodedBodySize", arena.NewNumberInt(p.EncodedBodySize))
	obj.Set("decodedBodySize", arena.NewNumberInt(p.DecodedBodySize))
	return obj
}

func (p moduleBodyProbe) appendJSON(arena *fastjson.Arena) *fastjson.Value {
	// Serialize the module body probe's request and artifact identity.
	obj := arena.NewObject()
	obj.Set("path", arena.NewString(p.Path))
	obj.Set("artifactRelPath", arena.NewString(p.ArtifactRelPath))
	obj.Set("status", arena.NewNumberInt(p.Status))
	setBoolJSONField(arena, obj, "ok", p.OK)
	obj.Set("contentType", arena.NewString(p.ContentType))
	obj.Set("contentLength", arena.NewString(p.ContentLength))

	// Serialize the module body size, fingerprint and diagnostic excerpts.
	obj.Set("bodyByteLength", arena.NewNumberInt(p.BodyByteLength))
	obj.Set("sha256", arena.NewString(p.SHA256))
	obj.Set("head", arena.NewString(p.Head))
	obj.Set("tail", arena.NewString(p.Tail))
	return obj
}

func appendStringMapJSON(arena *fastjson.Arena, values map[string]string) *fastjson.Value {
	// Collect the string map's keys for stable JSON output.
	obj := arena.NewObject()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	// Serialize string map values in sorted key order.
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
