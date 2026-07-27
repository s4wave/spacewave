//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/e2e/drivebench"
)

// driveBenchEnv opts the bundled Drive startup bench in alongside the unbundled
// e2e/wasm bench, so one flag enables both and their run.json cells share a
// directory tree.
const driveBenchEnv = "E2E_WASM_DRIVE_BENCH"

// TestGoScriptDriveStartupBenchBundled records the time-to-Drive bench for the
// production bundled build across three runtime states on one owned
// BrowserContext, writing the same run.json schema as the unbundled bench so
// cells compare across harnesses. The bundled production path has no Resource
// SDK client. An opt-in root-runtime trace is read directly from its browser
// SharedWorker target.
//
//   - cold: a fresh context boots the bundled SharedWorker runtime over a cold
//     HTTP cache and empty OPFS Space state.
//   - warm: a return visitor; the cold page is closed to terminate the
//     SharedWorker, then a fresh page in the same context reboots over the
//     retained HTTP cache and OPFS Space state.
//   - cache-hot: the asset cache stays warm but the OPFS Space state is cleared
//     before the reboot, isolating asset-load cost from Space-data cost.
//
// The bench is opt-in via E2E_WASM_DRIVE_BENCH and requires the GoScript release
// build; cache-hot additionally needs Chromium CDP storage control.
func TestGoScriptDriveStartupBenchBundled(t *testing.T) {
	if os.Getenv(driveBenchEnv) != "1" {
		t.Skipf("bundled drive bench disabled; set %s=1 to run", driveBenchEnv)
	}
	compiler, err := resolveReleaseWasmCompiler()
	if err != nil {
		t.Fatalf("resolve release wasm compiler: %v", err)
	}
	if compiler != releaseWasmCompilerGoScript {
		t.Skipf("bundled drive bench requires %s=true", E2EReleaseWasmGoScriptEnv)
	}
	if releaseStartupTraceEnabled() {
		if err := checkReleaseStartupTraceBrowser(); err != nil {
			t.Fatalf("startup trace capture: %v", err)
		}
	}

	// One run stamp groups every cell's artifacts under a single run directory.
	runStamp := time.Now().UTC().Format("20060102-150405")

	// cold: a fresh context boots over empty storage. It persists across page
	// replacements so the warm and cache-hot cells reuse its retained HTTP cache
	// and OPFS Space state.
	coldPage := testHarness.newPage(t)
	benchCtx := coldPage.Context()
	runBundledDriveBenchCell(t, coldPage, bundledDriveBenchCellInput{
		runStamp:     runStamp,
		compiler:     string(compiler),
		runtimeState: "cold",
		cell:         "cold-bundled",
	})

	// warm: a return visitor. Closing the cold page terminates the SharedWorker
	// runtime (no remaining clients) so the next page boots a fresh worker over
	// the warm HTTP cache and retained OPFS Space state.
	if err := coldPage.Close(); err != nil {
		t.Fatalf("close cold page: %v", err)
	}
	warmPage := testHarness.newPageInContext(t, benchCtx)
	runBundledDriveBenchCell(t, warmPage, bundledDriveBenchCellInput{
		runStamp:     runStamp,
		compiler:     string(compiler),
		runtimeState: "warm",
		cell:         "warm-bundled",
	})

	// cache-hot: keep the warm asset cache, but clear the OPFS Space state before
	// the reboot so the cell measures a first-run Space with cached assets.
	if testHarness.browserName != "chromium" {
		t.Logf("skipping cache-hot cell: OPFS reset requires chromium CDP, have %q", testHarness.browserName)
		return
	}
	if err := warmPage.Close(); err != nil {
		t.Fatalf("close warm page: %v", err)
	}
	cacheHotPage := testHarness.newPageInContext(t, benchCtx)
	if err := resetBundledSpaceStateKeepCache(cacheHotPage, testHarness.getBaseURL()); err != nil {
		t.Fatalf("reset space state: %v", err)
	}
	runBundledDriveBenchCell(t, cacheHotPage, bundledDriveBenchCellInput{
		runStamp:     runStamp,
		compiler:     string(compiler),
		runtimeState: "cache-hot",
		cell:         "cache-hot-bundled",
	})
}

// bundledDriveBenchCellInput carries the per-cell identity for one bundled bench
// cell.
type bundledDriveBenchCellInput struct {
	runStamp     string
	compiler     string
	runtimeState string
	cell         string
}

// runBundledDriveBenchCell boots the bundled Drive route on page, records the
// cell milestones and bundle summary from the browser performance timeline, and
// writes the cell's run.json. Milestones are browser performance.now values,
// which reset at each page navigation, so each is milliseconds from this cell's
// navigation start.
func runBundledDriveBenchCell(t *testing.T, page playwright.Page, in bundledDriveBenchCellInput) {
	t.Helper()

	cellDir, err := drivebench.CellDir(in.runStamp, in.cell)
	if err != nil {
		t.Fatalf("resolve cell dir (%s): %v", in.cell, err)
	}

	navStart := time.Now()
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive (%s): %v", in.cell, err)
	}
	enableQuickstartTimingLogs(t, page)

	waitForPrerenderRootOrLiveApp(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	liveAppMs := browserNowMs(t, page)
	waitForQuickstartAppRoute(t, page)
	routeAcceptedMs := browserNowMs(t, page)
	// The first-run intro overlays the already-mounted viewer; the benchmark
	// observes readiness without adding an interaction to the startup interval.
	if err := page.Locator("[data-testid='unixfs-browser']:visible").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)},
	); err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart frame-ready (%s): %v", in.cell, err)
	}
	unixfsVisibleMs := browserNowMs(t, page)
	contentReadyMs, contentReadyErr := waitForQuickstartDriveContentReady(t, page)
	if contentReadyErr != "" {
		t.Logf("quickstart content-ready not reached (%s): %s", in.cell, contentReadyErr)
	}
	logQuickstartTiming(t, page)

	var startupTrace []byte
	if releaseStartupTraceEnabled() {
		startupTrace, err = captureReleaseStartupTrace(t.Context(), testHarness.browser)
		if err != nil {
			t.Fatalf("capture startup trace (%s): %v", in.cell, err)
		}
	}

	distDir := filepath.Join(testHarness.repoRoot, releaseDistRelPath)
	bundle, err := drivebench.MeasureBundleDir(distDir)
	if err != nil {
		t.Fatalf("measure served bundle (%s): %v", in.cell, err)
	}

	var contentReadyValue int
	if contentReadyMs != nil {
		contentReadyValue = *contentReadyMs
	}
	run := drivebench.Run{
		Timestamp:    navStart.UTC().Format(time.RFC3339Nano),
		Compiler:     in.compiler,
		BuildMode:    "bundled",
		RuntimeState: in.runtimeState,
		Cell:         in.cell,
		Milestones: drivebench.Milestones{
			LiveAppMs:       int64(liveAppMs),
			RouteAcceptedMs: int64(routeAcceptedMs),
			UnixfsVisibleMs: int64(unixfsVisibleMs),
			ContentReadyMs:  int64(contentReadyValue),
		},
		Browser:      readBundledBrowser(t, page, contentReadyValue),
		ServedBundle: bundle,
	}
	run.Browser.StartupMarks = readBundledStartupMarks(t, page)
	if len(startupTrace) != 0 {
		tracePath := filepath.Join(cellDir, "runtime.trace")
		if err := drivebench.WriteArtifact(tracePath, startupTrace); err != nil {
			t.Fatalf("write runtime trace (%s): %v", in.cell, err)
		}
		run.Trace = &drivebench.Trace{
			Bytes:            len(startupTrace),
			RuntimeTracePath: tracePath,
		}
	}
	t.Logf("bundled served bundle (%s): totalBytes=%d wasmBytes=%d fileCount=%d",
		in.cell, bundle.TotalBytes, bundle.WasmBytes, bundle.FileCount)

	runPath, err := drivebench.WriteRun(cellDir, run)
	if err != nil {
		t.Fatalf("write run.json (%s): %v", in.cell, err)
	}
	t.Logf("bundled drive bench cell %s written to %s (live=%dms route=%dms unixfs=%dms content=%dms)",
		in.cell, runPath, liveAppMs, routeAcceptedMs, unixfsVisibleMs, contentReadyValue)
}

// readBundledStartupMarks reads the page startup-mark timeline emitted by
// markStartupBoundary so the bundled pre-quickstart gap can be attributed to the
// marks that bracket it. A read failure logs and yields no marks rather than
// failing the cell, since the marks are diagnostic, not a pass criterion.
func readBundledStartupMarks(t *testing.T, page playwright.Page) []drivebench.StartupMark {
	t.Helper()

	raw, err := page.Evaluate(drivebench.StartupMarksScript)
	if err != nil {
		t.Logf("read startup marks: %v", err)
		return nil
	}
	return drivebench.ParseStartupMarks(raw)
}

// readBundledBrowser reads the browser-side quickstart timing reported by the
// Drive viewer, falling back to the measured content-ready milestone when the
// viewer timing object is absent.
func readBundledBrowser(t *testing.T, page playwright.Page, contentReadyMs int) drivebench.Browser {
	t.Helper()

	raw, err := page.Evaluate(`() => globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null`)
	if err != nil {
		t.Logf("read quickstart timing: %v", err)
		return drivebench.Browser{ContentReadyMs: contentReadyMs}
	}
	timing, ok := raw.(map[string]any)
	if !ok {
		return drivebench.Browser{ContentReadyMs: contentReadyMs}
	}
	return drivebench.BrowserFromQuickstartTiming(contentReadyMs, timing)
}

// resetBundledSpaceStateKeepCache clears the origin's OPFS Space-state storage
// while leaving the HTTP/module asset cache warm, producing the cache-hot
// runtime state. It runs on a fresh page before its Drive navigation, so no
// SharedWorker holds an OPFS lock during the clear. Chromium-only: it drives the
// CDP Storage domain.
func resetBundledSpaceStateKeepCache(page playwright.Page, origin string) error {
	cdp, err := page.Context().NewCDPSession(page)
	if err != nil {
		return errors.Wrap(err, "new cdp session")
	}
	defer cdp.Detach()
	if _, err := cdp.Send("Storage.clearDataForOrigin", map[string]any{
		"origin":       origin,
		"storageTypes": "file_systems",
	}); err != nil {
		return errors.Wrap(err, "clear opfs storage")
	}
	return nil
}
