//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/sirupsen/logrus"
)

// releaseUpgradePatchEnv names a patch applied to the checkout to build the
// newer release. The patch is reverted when the test ends.
const releaseUpgradePatchEnv = "E2E_RELEASE_WASM_UPGRADE_PATCH"

// releaseUpgradeRestartScript reports whether a second spacewave-core worker
// started, which happens when the cached generation boots first and is then
// replaced by the newer release.
const releaseUpgradeRestartScript = `() => performance.getEntriesByType('mark')
	.filter((m) => m.detail?.label === 'worker.construct-start' &&
		m.detail?.workerId?.startsWith('plugin/spacewave-core//'))
	.length >= 2`

// releaseUpgradeRestartWaitMS bounds the wait for a replacement core worker
// after Drive content shows. A cached generation is replaced within seconds of
// the announcement.
const releaseUpgradeRestartWaitMS = 15_000

// TestLocalCDNReleaseUpgrade measures a returning visitor's first load after a
// newer release is published: the time to Drive content, the workers started,
// the manifest copies, and the pack bytes delivered by the local CDN.
func TestLocalCDNReleaseUpgrade(t *testing.T) {
	// Require the local CDN fixture and the patch that defines the new release.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}
	patch := os.Getenv(releaseUpgradePatchEnv)
	if patch == "" {
		t.Skip("set " + releaseUpgradePatchEnv + " to a patch that defines the newer release")
	}
	patch, err := filepath.Abs(patch)
	if err != nil {
		t.Fatal(err)
	}

	// Visit the current release and wait until it is fully installed.
	server := testHarness.server.Handler.(*localCDNHandler)
	ctx := testHarness.newBrowserContext(t)
	page := installCurrentRelease(t, ctx)
	logReleaseUpgradeMarks(t, page, "current")

	// Closing the only page stops the runtime's shared workers.
	if err := page.Close(); err != nil {
		t.Fatal(err)
	}

	// Build and publish the newer release at the same origin.
	git := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = testHarness.repoRoot
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := git("apply", patch); err != nil {
		t.Fatalf("apply %s: %v", patch, err)
	}
	t.Cleanup(func() {
		if err := git("apply", "-R", patch); err != nil {
			t.Errorf("revert %s: %v", patch, err)
		}
	})
	le := logrus.NewEntry(logrus.StandardLogger())
	if _, err := prepareLocalCDN(t.Context(), le, testHarness.repoRoot, testHarness.baseURL); err != nil {
		t.Fatal(err)
	}

	// Count only the delivery caused by the return visit.
	initialRoots := server.roots.Load()
	initialPacks := server.packs.Load()
	initialPackBytes := server.packBytes.Load()

	// Return to the same page as the returning visitor.
	page = testHarness.newPageInContext(t, ctx)
	if _, err := page.Goto(testHarness.baseURL + localCDNDrivePath); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	readyMS, failure := waitForQuickstartDriveContentReady(t, page)
	if failure != "" {
		t.Fatal(failure)
	}

	// Wait for a replacement core worker, so a double boot shows in the worker
	// counts below. A timeout means the newer release booted directly.
	_, _ = page.WaitForFunction(releaseUpgradeRestartScript, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(releaseUpgradeRestartWaitMS),
	})

	// Report the upgrade timeline and capture the runtime trace.
	starts := logReleaseUpgradeMarks(t, page, "newer")
	if releaseStartupTraceEnabled() {
		data, err := captureReleaseStartupTrace(t.Context(), testHarness.browser)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(testHarness.artifactDir, "local-cdn-upgrade.trace")
		if err := os.MkdirAll(testHarness.artifactDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("upgrade runtime trace: %s (%d bytes)", path, len(data))
	}

	// The upgrade must boot each startup plugin once and fetch only new blocks.
	t.Logf("local CDN upgrade: content=%dms roots=%d pack-requests=%d pack-bytes=%d",
		*readyMS, server.roots.Load()-initialRoots, server.packs.Load()-initialPacks, server.packBytes.Load()-initialPackBytes)
	for _, plugin := range []string{"spacewave-core", "spacewave-web", "spacewave-app"} {
		if count := starts[plugin]; count != 1 {
			t.Errorf("upgrade started %d workers for %s, want one: %v", count, plugin, starts)
		}
	}
}

// localCDNDrivePath is the Drive quickstart route a returning visitor opens.
const localCDNDrivePath = "/quickstart/drive"

// installCurrentRelease opens Drive in ctx and returns the page once Drive
// shows and the offline inventory of the current release is promoted.
func installCurrentRelease(t *testing.T, ctx playwright.BrowserContext) playwright.Page {
	// Open Drive and wait for its content.
	t.Helper()
	page := testHarness.newPageInContext(t, ctx)
	if _, err := page.Goto(testHarness.baseURL + localCDNDrivePath); err != nil {
		t.Fatal(err)
	}
	waitForLiveApp(t, page)
	if _, failure := waitForQuickstartDriveContentReady(t, page); failure != "" {
		t.Fatal(failure)
	}

	// Wait for the service worker to promote the release for offline use.
	if _, err := page.WaitForFunction(`async () => {
		const cache = await caches.open('bldr-control')
		const response = await cache.match('/__bldr/browser-release-state.json')
		return response && (await response.json()).promotedCurrent
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		t.Fatalf("install the current release: %v", err)
	}
	return page
}

// logReleaseUpgradeMarks logs the worker starts, plugin readiness and manifest
// copies on the page's startup timeline, and returns the worker start count of
// each plugin.
func logReleaseUpgradeMarks(t *testing.T, page playwright.Page, release string) map[string]int {
	t.Helper()
	starts := make(map[string]int)
	for _, mark := range readBundledStartupMarks(t, page) {
		d := mark.Detail
		switch {
		case mark.Label == "worker.construct-start":
			worker := composedString(d["workerId"])
			starts[workerPlugin(worker)]++
			t.Logf("%s %6dms worker start %s", release, mark.StartMs, worker)
		case mark.Label == "plugin.running":
			t.Logf("%s %6dms plugin running %s", release, mark.StartMs, composedString(d["workerId"]))
		case strings.HasPrefix(mark.Label, "manifest-copy."):
			t.Logf("%s %6dms %s %s copied=%v existing=%v source-bytes=%v demand-bytes=%v",
				release, mark.StartMs, mark.Label, composedString(d["pluginId"]),
				d["blocksCopied"], d["blocksExisting"], d["logicalSourceBytes"], d["demandReadBytes"])
		}
	}
	return starts
}
