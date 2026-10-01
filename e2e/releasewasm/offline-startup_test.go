//go:build !skip_e2e && !js

package releasewasm

import (
	"net/http/httptest"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

func TestGoScriptOfflineStartup(t *testing.T) {
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatal(err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skip("requires the GoScript release build")
	}
	// Own the origin so stopping it leaves the suite's server available. Browser
	// offline emulation also disables WebKit service-worker cache responses.
	h := *testHarness
	origin := httptest.NewUnstartedServer(nil)
	t.Cleanup(origin.Close)
	h.baseURL = "http://" + origin.Listener.Addr().String()
	origin.Config.Handler = releaseHandler(h.distDirs.releaseDist, h.distDirs.prerender, h.baseURL)
	origin.Start()
	// A persistent profile allows the whole browser runtime to stop without
	// deleting the release cache or the user's Drive content.
	profile := t.TempDir()
	ctx := h.newPersistentBrowserContext(t, profile)
	t.Cleanup(func() {
		if err := ctx.Close(); err != nil {
			t.Logf("close offline browser context: %v", err)
		}
	})
	page := h.newPageInContext(t, ctx)
	if _, err := page.Goto(h.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		dumpPageState(t, page)
		t.Fatal(err)
	}
	waitForMaterializerPluginRunningMark(t, page)
	if _, err := page.WaitForFunction(`async () => {
		const cache = await caches.open('bldr-control')
		const response = await cache.match('/__bldr/browser-release-state.json')
		return response && (await response.json()).promotedCurrent
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		t.Fatalf("complete background offline cache: %v", err)
	}
	desc, err := h.browserRelease(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var packPath string
	for _, asset := range desc.RequiredStaticAssets {
		if strings.HasSuffix(asset, ".kvfile") {
			packPath = asset
			break
		}
	}
	if packPath == "" {
		t.Fatal("offline inventory has no kvfile")
	}
	origin.Close()
	if _, err := page.Evaluate(`async path => {
		const response = await fetch(path, {headers: {Range: 'bytes=0-15'}})
		const body = await response.arrayBuffer()
		if (response.status !== 206 || body.byteLength !== 16) {
			throw new Error('cached kvfile range: status=' + response.status + ' bytes=' + body.byteLength)
		}
	}`, packPath); err != nil {
		t.Fatal(err)
	}
	// Closing the context terminates its shared workers before reopening.
	if err := ctx.Close(); err != nil {
		t.Fatal(err)
	}
	ctx = h.newPersistentBrowserContext(t, profile)
	page = h.newPageInContext(t, ctx)
	if _, err := page.Goto(h.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("restart Drive offline after terminating its runtime: %v", err)
	}
	if _, err := waitForQuickstartDriveContentReady(t, page); err != "" {
		t.Fatalf("read persisted Drive content offline: %s", err)
	}
	waitForMaterializerPluginRunningMark(t, page)

	// Git has not been used in this context. Its first operation must load the
	// deferred implementation from the completed offline release inventory.
	if _, err := page.Evaluate(`() => { location.hash = '/quickstart/git' }`); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("New Git Repository", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("open Git wizard offline: %v", err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Next", Exact: new(true)}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByPlaceholder("Enter repository name...").Fill("Offline repository"); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Create", Exact: new(true)}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("This repository has no commits yet.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("create and open a Git repository on first use offline: %v", err)
	}
}

// waitForMaterializerPluginRunningMark waits until the document startup marks
// record the materializer plugin worker reaching its running state.
func waitForMaterializerPluginRunningMark(t *testing.T, page playwright.Page) {
	t.Helper()

	if _, err := page.WaitForFunction(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		return marks.some((mark) =>
			mark.name === 'spacewave.startup.plugin.running' &&
			mark.detail?.workerId?.startsWith('plugin/bldr-materializer/'),
		)
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("materializer plugin running startup mark: %v", err)
	}
}
