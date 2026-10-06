//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"testing"
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
