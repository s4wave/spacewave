//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestLocalCDNCachedStartupDuringOutage verifies that a returning visitor
// opens Drive from the installed release while the CDN is unreachable, so
// startup waits on the release announcement only while the CDN can answer.
func TestLocalCDNCachedStartupDuringOutage(t *testing.T) {
	// Require the local CDN fixture, which serves the release announcement.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}

	// Install the current release, then stop its runtime.
	server := testHarness.server.Handler.(*localCDNHandler)
	ctx := testHarness.newBrowserContext(t)
	page := installCurrentRelease(t, ctx)
	if err := page.Close(); err != nil {
		t.Fatal(err)
	}

	// Return while every CDN request fails.
	server.cdnDown.Store(true)
	t.Cleanup(func() { server.cdnDown.Store(false) })
	page = testHarness.newPageInContext(t, ctx)
	if _, err := page.Goto(testHarness.baseURL + localCDNDrivePath); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	readyMS, failure := waitForQuickstartDriveContentReady(t, page)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Logf("local CDN outage: content=%dms", *readyMS)
}

// TestLocalCDNStalledStartupOffersRetry verifies that a boot stuck before the
// app is revealed reaches the failure screen at the web view deadline, and
// that Retry boots the app once the stall clears. The page has no console
// diagnostics, since the deadline's failure is logged as an error by design.
func TestLocalCDNStalledStartupOffersRetry(t *testing.T) {
	// Require the local CDN fixture, which can hold its requests.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}

	// Open a new visitor's session while the CDN never answers.
	server := testHarness.server.Handler.(*localCDNHandler)
	release := server.holdCDN()
	t.Cleanup(release)
	ctx := testHarness.newBrowserContext(t)
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/quickstart/local"); err != nil {
		t.Fatal(err)
	}

	// The web view deadline fails the boot and offers Retry.
	retry := button(page, "Retry")
	if err := retry.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(150_000)}); err != nil {
		t.Fatalf("stalled startup did not offer Retry: %v", err)
	}
	text, err := page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Unable to open Spacewave") || !strings.Contains(text, "web view did not become ready") {
		t.Fatalf("stalled startup shows %q", text)
	}
	t.Logf("stalled startup shows %q", text)

	// Retry reloads the page and boots once the CDN answers.
	release()
	if err := retry.Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => /^#\/u\/\d+\/?$/.test(location.hash)`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("Retry did not create a session: %v", err)
	}
	waitForLiveApp(t, page)
}

// runtimeFaultScript makes each document start its runtime with a goroutine
// that panics, until a page sets the e2e-runtime-fault key to "off".
const runtimeFaultScript = `
if (localStorage.getItem('e2e-runtime-fault') !== 'off') {
	globalThis.BLDR_RUNTIME_WASM_ENV = Object.assign({}, globalThis.BLDR_RUNTIME_WASM_ENV || {}, {BLDR_RUNTIME_FAULT: 'goroutine-panic'});
}
`

// TestLocalCDNRuntimePanicOffersRetry verifies that a runtime whose Go program
// panics fails every document at once, including one that attaches after the
// panic, and that Retry in either document boots a new runtime once the fault
// clears, while the other document stays open on the failure. That document's
// own Retry then joins the new runtime. The pages have no console diagnostics,
// since the panic is logged.
func TestLocalCDNRuntimePanicOffersRetry(t *testing.T) {
	// Require the local CDN fixture, which serves the release.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}
	for _, tc := range []struct {
		name      string
		hostRetry bool
	}{
		{name: "host retries", hostRetry: true},
		{name: "attached retries", hostRetry: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testHarness.newBrowserContext(t)
			script := runtimeFaultScript
			if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
				t.Fatal(err)
			}

			// The host document's runtime panics after it starts, and a
			// document that attaches after the panic fails at once, well
			// before the web view deadline.
			host := openRuntimeFailure(t, ctx, 150_000)
			if os.Getenv("E2E_RELEASE_WASM_CONSOLE_TRACE") == "1" {
				host.OnConsole(func(msg playwright.ConsoleMessage) {
					t.Logf("host page %s: %s", msg.Type(), msg.Text())
				})
			}
			attached := openRuntimeFailure(t, ctx, 45_000)

			retrying, survivor := attached, host
			if tc.hostRetry {
				retrying, survivor = host, attached
			}

			// Retry, with the fault cleared, boots a new runtime while the
			// other document stays open.
			if _, err := retrying.Evaluate(`() => localStorage.setItem('e2e-runtime-fault', 'off')`); err != nil {
				t.Fatal(err)
			}
			retryIntoSession(t, "retrying", retrying)

			// The survivor still shows the failure; its Retry joins the new
			// runtime.
			text, err := survivor.Locator("body").InnerText()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "Unable to open Spacewave") {
				t.Fatalf("survivor left the failure screen: %q", text)
			}
			retryIntoSession(t, "survivor", survivor)
		})
	}
}

// retryIntoSession presses Retry on the page named role and waits for the
// live app. On a timeout it reports the last hash the wait sampled and the
// page's address.
func retryIntoSession(t *testing.T, role string, page playwright.Page) {
	t.Helper()
	if err := button(page, "Retry").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => { globalThis.__e2eLastHash = location.hash; return /^#\/u\/\d+\/?$/.test(location.hash) }`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(180_000),
	}); err != nil {
		last, _ := page.Evaluate(`() => globalThis.__e2eLastHash`)
		href, _ := page.Evaluate(`() => location.href`)
		text, _ := page.Locator("body").InnerText()
		t.Fatalf("%s Retry did not create a session: %v; last sampled hash %v; href %v; page shows %q", role, err, last, href, text)
	}
	waitForLiveApp(t, page)
}

// openRuntimeFailure opens the quickstart in a new page of ctx and waits for
// the runtime failure screen, not the web view deadline's, for up to
// timeoutMS.
func openRuntimeFailure(t *testing.T, ctx playwright.BrowserContext, timeoutMS float64) playwright.Page {
	t.Helper()
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/quickstart/local"); err != nil {
		t.Fatal(err)
	}
	if err := button(page, "Retry").WaitFor(playwright.LocatorWaitForOptions{Timeout: new(timeoutMS)}); err != nil {
		t.Fatalf("runtime panic did not offer Retry: %v", err)
	}
	text, err := page.Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Unable to open Spacewave") || strings.Contains(text, "web view did not become ready") {
		t.Fatalf("runtime panic shows %q", text)
	}
	t.Logf("runtime panic shows %q", text)
	return page
}
