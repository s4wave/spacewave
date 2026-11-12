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
