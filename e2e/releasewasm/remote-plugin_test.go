//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
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
	waitForQuickstartDriveFrame(t, freshPage, false, "wait for fresh quickstart frame-ready: %v")

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
