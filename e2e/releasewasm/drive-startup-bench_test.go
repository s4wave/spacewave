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

// driveBenchCPUProfileEnv opts each bundled bench cell into a Chromium trace
// carrying the V8 sampling profiler for every worker, written beside run.json
// as cpu-profile.trace.json. Profiled cells include sampling overhead and
// compare only with each other.
const driveBenchCPUProfileEnv = "E2E_WASM_DRIVE_BENCH_CPU_PROFILE"

// driveBenchCPUProfileCategories selects the trace events needed to attribute
// worker CPU to the generated Go functions that were running.
var driveBenchCPUProfileCategories = []string{
	"devtools.timeline",
	"disabled-by-default-devtools.timeline",
	"v8",
	"v8.execute",
	"disabled-by-default-v8.cpu_profiler",
	"blink.user_timing",
}

// TestGoScriptDriveStartupBenchBundled records the time-to-Drive bench for the
// production bundled build across four runtime states on one owned
// BrowserContext, writing the same run.json schema as the unbundled bench so
// cells compare across harnesses. The bundled production path has no Resource
// SDK client. An opt-in root-runtime trace is read directly from its browser
// SharedWorker target.
//
//   - cold: a fresh context boots the bundled worker runtime over a cold
//     HTTP cache and empty OPFS Space state.
//   - live: a second tab attaches while the cold tab keeps the runtime alive.
//   - warm: a return visitor. The cold page is closed to terminate the
//     runtime, then a fresh page in the same context reboots over the
//     retained HTTP cache and OPFS Space state.
//   - cache-hot: the asset cache stays warm but the OPFS Space state is cleared
//     before the reboot, isolating asset-load cost from Space-data cost.
//
// The bench is opt-in via E2E_WASM_DRIVE_BENCH and requires the GoScript release
// build. The cache-hot state additionally needs Chromium CDP storage control.
func TestGoScriptDriveStartupBenchBundled(t *testing.T) {
	// Require the bundled Drive benchmark opt-in and GoScript compiler.
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

	// A second tab must reuse the live host. Warm startup below measures a
	// different operation because closing every tab destroys that host.
	livePage := testHarness.newPageInContext(t, benchCtx)
	runBundledDriveBenchCell(t, livePage, bundledDriveBenchCellInput{
		runStamp:     runStamp,
		compiler:     string(compiler),
		runtimeState: "live",
		cell:         "live-bundled",
	})
	if err := livePage.Close(); err != nil {
		t.Fatalf("close live page: %v", err)
	}

	// warm: a return visitor. Closing the cold page terminates the worker
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
	// Resolve the artifact directory for this bundled Drive benchmark cell.
	t.Helper()
	cellDir, err := drivebench.CellDir(in.runStamp, in.cell)
	if err != nil {
		t.Fatalf("resolve cell dir (%s): %v", in.cell, err)
	}

	// Start the optional CPU profile before the navigation that boots the runtime.
	stopCPUProfile := startBundledCPUProfile(t, page, cellDir)

	// Navigate the benchmark page to the Drive quickstart and enable timing.
	navStart := time.Now()
	if _, err := page.Goto(testHarness.getBaseURL() + "/quickstart/drive"); err != nil {
		t.Fatalf("goto quickstart drive (%s): %v", in.cell, err)
	}
	enableQuickstartTimingLogs(t, page)

	// Record the live application and accepted route milestones.
	waitForPrerenderRootOrLiveApp(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)
	liveAppMs := browserNowMs(t, page)
	waitForQuickstartAppRoute(t, page)
	routeAcceptedMs := browserNowMs(t, page)

	// The first-run intro overlays the already-mounted viewer. The
	// benchmark observes readiness without adding an interaction to the
	// startup interval.
	if err := waitForDriveFrameReady(page, browserWaitMS); err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for quickstart frame-ready (%s): %v", in.cell, err)
	}
	unixfsVisibleMs := browserNowMs(t, page)
	contentReadyMs, contentReadyErr := waitForQuickstartDriveContentReady(t, page)
	if contentReadyErr != "" {
		t.Errorf("quickstart content-ready not reached (%s): %s", in.cell, contentReadyErr)
	}
	logQuickstartTiming(t, page)
	stopCPUProfile()

	// Capture the optional bundled startup runtime trace.
	var startupTrace []byte
	if releaseStartupTraceEnabled() {
		startupTrace, err = captureReleaseStartupTrace(t.Context(), testHarness.browser)
		if err != nil {
			t.Fatalf("capture startup trace (%s): %v", in.cell, err)
		}
	}

	// Measure the served production bundle.
	distDir := filepath.Join(testHarness.repoRoot, releaseDistRelPath)
	bundle, err := drivebench.MeasureBundleDir(distDir)
	if err != nil {
		t.Fatalf("measure served bundle (%s): %v", in.cell, err)
	}

	// Build the bundled benchmark record from its readiness milestones.
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

	// Verify a live second tab reuses its existing runtime.
	if in.runtimeState == "live" {
		for _, mark := range run.Browser.StartupMarks {
			if mark.Label == "runtime.worker-created" && mark.Source == "browser" {
				t.Error("second tab created a runtime instead of attaching to the live host")
			}
		}
	}

	// Preserve the runtime trace and attach it to the benchmark record.
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

	// Preserve the bundle measurements and benchmark record for this cell.
	t.Logf("bundled served bundle (%s): totalBytes=%d wasmBytes=%d fileCount=%d",
		in.cell, bundle.TotalBytes, bundle.WasmBytes, bundle.FileCount)
	runPath, err := drivebench.WriteRun(cellDir, run)
	if err != nil {
		t.Fatalf("write run.json (%s): %v", in.cell, err)
	}
	t.Logf("bundled drive bench cell %s written to %s (live=%dms route=%dms unixfs=%dms content=%dms)",
		in.cell, runPath, liveAppMs, routeAcceptedMs, unixfsVisibleMs, contentReadyValue)
}

// startBundledCPUProfile starts the opt-in Chromium CPU profile for one cell
// and returns the function that stops it and writes the trace into cellDir. It
// returns a no-op when the profile is not requested or the browser is not
// Chromium, the only engine with browser tracing.
func startBundledCPUProfile(t *testing.T, page playwright.Page, cellDir string) func() {
	t.Helper()
	if os.Getenv(driveBenchCPUProfileEnv) != "1" || testHarness.browserName != "chromium" {
		return func() {}
	}

	path := filepath.Join(cellDir, "cpu-profile.trace.json")
	if err := os.MkdirAll(cellDir, 0o755); err != nil {
		t.Fatalf("create cell dir for cpu profile: %v", err)
	}
	screenshots := false
	if err := testHarness.browser.StartTracing(playwright.BrowserStartTracingOptions{
		Page:        page,
		Path:        &path,
		Categories:  driveBenchCPUProfileCategories,
		Screenshots: &screenshots,
	}); err != nil {
		t.Fatalf("start cpu profile: %v", err)
	}
	return func() {
		if _, err := testHarness.browser.StopTracing(); err != nil {
			t.Fatalf("stop cpu profile: %v", err)
		}
		t.Logf("bundled drive bench cpu profile written to %s", path)
	}
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
	// Read the browser quickstart timing with the content milestone as fallback.
	t.Helper()
	raw, err := page.Evaluate(`() => globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null`)
	if err != nil {
		t.Logf("read quickstart timing: %v", err)
		return drivebench.Browser{ContentReadyMs: contentReadyMs}
	}

	// Decode the browser timing record or retain the measured content milestone.
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
	// Open and release a CDP session for clearing the Space state.
	cdp, err := page.Context().NewCDPSession(page)
	if err != nil {
		return errors.Wrap(err, "new cdp session")
	}
	defer cdp.Detach()

	// Clear the origin's OPFS state while retaining its asset cache.
	if _, err := cdp.Send("Storage.clearDataForOrigin", map[string]any{
		"origin":       origin,
		"storageTypes": "file_systems",
	}); err != nil {
		return errors.Wrap(err, "clear opfs storage")
	}
	return nil
}
