//go:build !skip_e2e && !js

package releasewasm

import (
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
	page := testHarness.newPage(t)
	ctx := page.Context()
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`async () => {
		const cache = await caches.open('bldr-control')
		const response = await cache.match('/__bldr/browser-release-state.json')
		return response && (await response.json()).promotedCurrent
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		t.Fatalf("complete background offline cache: %v", err)
	}
	desc, err := testHarness.browserRelease(t.Context())
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
	if err := ctx.SetOffline(true); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async path => {
		const response = await fetch(path, {headers: {Range: 'bytes=0-15'}})
		const body = await response.arrayBuffer()
		if (response.status !== 206 || body.byteLength !== 16) {
			throw new Error('cached kvfile range: status=' + response.status + ' bytes=' + body.byteLength)
		}
	}`, packPath); err != nil {
		t.Fatal(err)
	}
	if err := page.Close(); err != nil {
		t.Fatal(err)
	}
	page = testHarness.newPageInContext(t, ctx)
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(); err != nil {
		dumpPageState(t, page)
		t.Fatalf("restart Drive offline after terminating its runtime: %v", err)
	}
	if _, err := waitForQuickstartDriveContentReady(t, page); err != "" {
		t.Fatalf("read persisted Drive content offline: %s", err)
	}
}
