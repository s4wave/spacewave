//go:build !skip_e2e && !js

package releasewasm

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/core/cdn"
)

// TestLocalCDNDriveStartup measures the production landing-to-Drive operation
// over localhost distribution metadata and KVF packs in a fresh browser context.
func TestLocalCDNDriveStartup(t *testing.T) {
	// Require the isolated local CDN fixture for the Drive startup measurement.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}

	// Observe delivery at the server without disabling browser HTTP caching.
	server := testHarness.server.Handler.(*localCDNHandler)
	initialDistribution := server.distribution.Load()
	initialRoots := server.roots.Load()
	initialPacks := server.packs.Load()
	page := testHarness.newPage(t)
	if testHarness.browserName == "chromium" {
		cdp, err := testHarness.browser.NewBrowserCDPSession()
		if err != nil {
			t.Fatal(err)
		}
		defer cdp.Detach()
		info, err := cdp.Send("SystemInfo.getInfo", nil)
		if err != nil {
			t.Fatal(err)
		}
		gpu := info.(map[string]any)["gpu"].(map[string]any)
		attributes, _ := gpu["auxAttributes"].(map[string]any)
		t.Logf("Chromium renderer: %v", attributes["glRenderer"])
	}

	// Collect requests that escape the local CDN fixture.
	var mtx sync.Mutex
	var external []string
	page.Context().OnRequest(func(req playwright.Request) {
		// The optional public-content catalog is absent from this fixture.
		// Browser proxy isolation rejects its connection before any remote I/O.
		if req.URL() == cdn.DefaultBaseURL+"/"+cdn.ProvisionedSpaceID+"/root.packedmsg" {
			return
		}
		u, err := url.Parse(req.URL())
		// WebKit reports local module blobs as requests. They perform no
		// network I/O and do not have an HTTP hostname to classify.
		if err == nil && (u.Scheme == "blob" || u.Scheme == "data") {
			return
		}
		if err != nil || u.Hostname() != "127.0.0.1" {
			mtx.Lock()
			external = append(external, req.URL())
			mtx.Unlock()
			return
		}
	})

	// Start exactly where a new visitor starts, before the runtime is booted.
	if _, err := page.Goto(testHarness.baseURL + "/"); err != nil {
		t.Fatal(err)
	}
	clickMS := browserNowMs(t, page)
	if err := page.Locator("a").Filter(playwright.LocatorFilterOptions{HasText: "Create a Drive"}).First().Click(); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	waitForQuickstartAppRoute(t, page)

	// Verify the Drive quickstart content and golden path become ready.
	readyMS, failure := waitForQuickstartDriveContentReady(t, page)
	if failure != "" {
		t.Fatal(failure)
	}
	completeQuickstartDriveIntroIfPresent(t, page)
	if _, failure := exerciseQuickstartDriveGoldenPath(t, page); failure != "" {
		t.Fatal(failure)
	}
	logQuickstartTiming(t, page)

	// Mixed-platform publication must not replace an admitted startup worker.
	starts := make(map[string]int)
	for _, mark := range readBundledStartupMarks(t, page) {
		if mark.Label == "worker.construct-start" {
			starts[composedString(mark.Detail["workerId"])]++
		}
	}
	for _, plugin := range []string{"spacewave-core", "spacewave-web", "spacewave-app"} {
		if count := starts["plugin/"+plugin]; count != 1 {
			t.Errorf("startup created %d workers for %s, want one: %v", count, plugin, starts)
		}
	}
	if releaseStartupTraceEnabled() {
		data, err := captureReleaseStartupTrace(t.Context(), testHarness.browser)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(testHarness.artifactDir, "local-cdn-startup.trace")
		if err := os.MkdirAll(testHarness.artifactDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("startup runtime trace: %s (%d bytes)", path, len(data))
	}

	// A successful timing must include the remote-loading contract it measures.
	mtx.Lock()
	defer mtx.Unlock()
	distribution := server.distribution.Load() - initialDistribution
	root := server.roots.Load() - initialRoots
	ranges := server.packs.Load() - initialPacks
	t.Logf("local CDN startup: navigation=%dms click-to-file=%dms distribution=%d roots=%d pack-requests=%d", *readyMS, *readyMS-clickMS, distribution, root, ranges)
	if len(external) != 0 {
		t.Errorf("startup attempted external requests: %v", external)
	}
	if distribution == 0 || root == 0 || ranges == 0 {
		t.Error("startup did not exercise distribution, CDN root, and pack loading")
	}
}
