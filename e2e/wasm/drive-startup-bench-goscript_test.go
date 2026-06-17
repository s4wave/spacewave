//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	playwright "github.com/playwright-community/playwright-go"
	exptrace "golang.org/x/exp/trace"
)

// driveStartupBenchRun is one bench cell artifact: the resolved build identity,
// wall-clock milestones from navigation start to Drive content-ready, the
// browser-observed quickstart timing, the served bundle size, the Resource SDK
// connection timing, and, when the trace service is available, the captured
// runtime trace summary. It is written as run.json per cell so cells compare
// across runtime states and build modes.
type driveStartupBenchRun struct {
	Timestamp          string                        `json:"timestamp"`
	Compiler           string                        `json:"compiler"`
	BuildMode          string                        `json:"buildMode"`
	RuntimeState       string                        `json:"runtimeState"`
	Cell               string                        `json:"cell"`
	Milestones         driveStartupBenchMilestones   `json:"milestones"`
	Browser            driveStartupBenchBrowser      `json:"browser"`
	ServedBundle       driveStartupBenchBundle       `json:"servedBundle"`
	ResourceConnection driveStartupBenchResourceConn `json:"resourceConnection"`
	Trace              *driveStartupBenchTrace       `json:"trace,omitempty"`
}

// driveStartupBenchMilestones records wall-clock milliseconds from navigation
// start to each boot milestone. ContentReadyMs is the moment getting-started.md
// is present in the file browser, which is also when the first file row renders.
type driveStartupBenchMilestones struct {
	LiveAppMs       int64 `json:"liveAppMs"`
	RouteAcceptedMs int64 `json:"routeAcceptedMs"`
	UnixfsVisibleMs int64 `json:"unixfsVisibleMs"`
	ContentReadyMs  int64 `json:"contentReadyMs"`
}

// driveStartupBenchBrowser carries the browser-side quickstart timing reported
// by the Drive viewer, independent of the Go-side wall clock.
type driveStartupBenchBrowser struct {
	ContentReadyMs            int    `json:"contentReadyMs"`
	QuickstartState           string `json:"quickstartState"`
	QuickstartProgressReadyMs *int   `json:"quickstartProgressReadyMs,omitempty"`
	QuickstartContentReadyMs  *int   `json:"quickstartContentReadyMs,omitempty"`
	QuickstartFinishedMs      *int   `json:"quickstartFinishedMs,omitempty"`
}

// driveStartupBenchBundle is the served bundle size observed by the browser, the
// sum of transferred resource bytes and the WASM subtotal.
type driveStartupBenchBundle struct {
	TotalTransferBytes int64 `json:"totalTransferBytes"`
	WasmTransferBytes  int64 `json:"wasmTransferBytes"`
	ResourceCount      int   `json:"resourceCount"`
}

// driveStartupBenchResourceConn summarizes the Resource SDK connection timing
// for the session.
type driveStartupBenchResourceConn struct {
	DurationMs     int64 `json:"durationMs"`
	Attempts       int   `json:"attempts"`
	StartupReloads int   `json:"startupReloads"`
}

// driveStartupBenchTrace summarizes the captured runtime trace and the paths of
// the raw trace plus its tracetool extraction.
type driveStartupBenchTrace struct {
	Bytes            int    `json:"bytes"`
	RuntimeTracePath string `json:"runtimeTracePath"`
	TracetoolPath    string `json:"tracetoolPath"`
	UserTasks        int    `json:"userTasks"`
	UserRegions      int    `json:"userRegions"`
	UserLogs         int    `json:"userLogs"`
}

// TestGoScriptDriveStartupBench records one cold, unbundled time-to-Drive cell
// through the real e2e/wasm harness. It measures navigation start to Drive
// content-ready milestones, the served bundle size, and the Resource SDK
// connection timing, and writes run.json under
// .bldr/e2e-goscript-drive-bench/<timestamp>/cold-unbundled/. When the trace
// service is available for the resolved compiler it also captures the runtime
// trace, writing runtime.trace and a tracetool.txt user-event summary. The
// bench is opt-in via E2E_WASM_DRIVE_BENCH so routine e2e does not pay its cost.
func TestGoScriptDriveStartupBench(t *testing.T) {
	skipDriveBenchWhenDisabled(t)

	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve compiler: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	// A clean session is the cold runtime state: fresh browser storage, a
	// dedicated WASM process, and a fresh boot to live app plus resource
	// connection, all bracketed as the live-app milestone.
	navStart := time.Now()
	sess := harness(t).NewCleanSession(t)
	liveAppMs := msSince(navStart)
	page := sess.Page()

	var (
		routeAcceptedMs int64
		unixfsVisibleMs int64
		contentReadyMs  int64
		driveReady      DriveReadyResult
	)
	driveOpen := func(context.Context) error {
		NavigateHash(t, harness(t), page, "#/quickstart/drive")
		routeAcceptedMs = msSince(navStart)
		WaitForDriveShell(t, page)
		unixfsVisibleMs = msSince(navStart)
		driveReady = WaitForDriveReady(t, harness(t), page)
		contentReadyMs = msSince(navStart)
		return nil
	}

	var traceData []byte
	traceEnabled := E2EWasmTraceServiceEnabled(compiler)
	if traceEnabled {
		traceData, err = sess.CaptureTrace(ctx, "goscript-drive-bench", driveOpen)
		if err != nil {
			t.Fatalf("capture trace: %v", err)
		}
	} else if err := driveOpen(ctx); err != nil {
		t.Fatalf("drive open: %v", err)
	}

	connTiming := sess.ResourceConnectionTiming()
	run := driveStartupBenchRun{
		Timestamp:    navStart.UTC().Format(time.RFC3339Nano),
		Compiler:     string(compiler),
		BuildMode:    "unbundled",
		RuntimeState: "cold",
		Cell:         "cold-unbundled",
		Milestones: driveStartupBenchMilestones{
			LiveAppMs:       liveAppMs,
			RouteAcceptedMs: routeAcceptedMs,
			UnixfsVisibleMs: unixfsVisibleMs,
			ContentReadyMs:  contentReadyMs,
		},
		Browser: driveStartupBenchBrowser{
			ContentReadyMs:            driveReady.ContentReadyMs,
			QuickstartState:           driveReady.QuickstartState,
			QuickstartProgressReadyMs: driveReady.QuickstartProgressReadyMs,
			QuickstartContentReadyMs:  driveReady.QuickstartContentReadyMs,
			QuickstartFinishedMs:      driveReady.QuickstartFinishedMs,
		},
		ServedBundle:       measureServedBundle(t, page),
		ResourceConnection: summarizeResourceConnection(connTiming),
	}

	cellDir := driveBenchCellDir(t, navStart, run.Cell)
	if traceEnabled {
		tracePath := filepath.Join(cellDir, "runtime.trace")
		if err := WriteTraceArtifact(tracePath, traceData); err != nil {
			t.Fatalf("write runtime.trace: %v", err)
		}
		tracetoolPath := filepath.Join(cellDir, "tracetool.txt")
		summary, tasks, regions, logs := summarizeTrace(t, traceData)
		if err := WriteTraceArtifact(tracetoolPath, []byte(summary)); err != nil {
			t.Fatalf("write tracetool.txt: %v", err)
		}
		run.Trace = &driveStartupBenchTrace{
			Bytes:            len(traceData),
			RuntimeTracePath: tracePath,
			TracetoolPath:    tracetoolPath,
			UserTasks:        tasks,
			UserRegions:      regions,
			UserLogs:         logs,
		}
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal run.json: %v", err)
	}
	runPath := filepath.Join(cellDir, "run.json")
	if err := WriteTraceArtifact(runPath, data); err != nil {
		t.Fatalf("write run.json: %v", err)
	}
	t.Logf("drive bench cell %s written to %s (live=%dms route=%dms unixfs=%dms content=%dms trace=%v)",
		run.Cell, runPath, liveAppMs, routeAcceptedMs, unixfsVisibleMs, contentReadyMs, traceEnabled)
}

// skipDriveBenchWhenDisabled skips the bench unless E2E_WASM_DRIVE_BENCH opts in.
func skipDriveBenchWhenDisabled(t testing.TB) {
	t.Helper()
	if !E2EWasmDriveBenchEnabled() {
		t.Skipf("drive bench disabled; set %s=1 to run", E2EWasmDriveBenchEnv)
	}
}

// msSince returns whole milliseconds elapsed since start.
func msSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

// driveBenchCellDir returns the artifact directory for one bench cell under the
// repo-root .bldr tree, grouped by run timestamp.
func driveBenchCellDir(t testing.TB, runStart time.Time, cell string) string {
	t.Helper()
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	stamp := runStart.UTC().Format("20060102-150405")
	return filepath.Join(repoRoot, ".bldr", "e2e-goscript-drive-bench", stamp, cell)
}

// measureServedBundle sums the browser's transferred resource bytes, reporting
// the total, the WASM subtotal, and the resource count. transferSize falls back
// to encodedBodySize for entries without a populated transfer size.
func measureServedBundle(t testing.TB, page playwright.Page) driveStartupBenchBundle {
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const entries = performance.getEntriesByType('resource')
		let total = 0
		let wasm = 0
		for (const entry of entries) {
			const size = entry.transferSize || entry.encodedBodySize || 0
			total += size
			if (entry.name.endsWith('.wasm')) {
				wasm += size
			}
		}
		return { total, wasm, count: entries.length }
	}`)
	if err != nil {
		t.Fatalf("measure served bundle: %v", err)
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected served bundle result %T", raw)
	}
	return driveStartupBenchBundle{
		TotalTransferBytes: numberFieldInt64(fields, "total"),
		WasmTransferBytes:  numberFieldInt64(fields, "wasm"),
		ResourceCount:      int(numberFieldInt64(fields, "count")),
	}
}

// summarizeResourceConnection reduces the session connection timing to the cell
// schema fields.
func summarizeResourceConnection(timing ResourceConnectionTiming) driveStartupBenchResourceConn {
	var durationMs int64
	if !timing.StartedAt.IsZero() && !timing.CompletedAt.IsZero() {
		durationMs = timing.CompletedAt.Sub(timing.StartedAt).Milliseconds()
	}
	return driveStartupBenchResourceConn{
		DurationMs:     durationMs,
		Attempts:       len(timing.Attempts),
		StartupReloads: timing.StartupReloads,
	}
}

// summarizeTrace walks the captured trace with the upstream Go trace reader, the
// same parser tracetool's fork is built on, and renders a user-event summary:
// one line per user task, region, and log with its monotonic offset. It returns
// the rendered text and the task, region, and log counts.
func summarizeTrace(t testing.TB, data []byte) (string, int, int, int) {
	t.Helper()
	reader, err := exptrace.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("trace reader rejected the capture: %v", err)
	}
	var (
		b       strings.Builder
		tasks   int
		regions int
		logs    int
	)
	for {
		ev, err := reader.ReadEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("trace reader failed mid-capture: %v", err)
		}
		switch ev.Kind() {
		case exptrace.EventTaskBegin:
			tasks++
			b.WriteString("task\t" + ev.Task().Type + "\n")
		case exptrace.EventRegionBegin:
			regions++
			b.WriteString("region\t" + ev.Region().Type + "\n")
		case exptrace.EventLog:
			logs++
			lg := ev.Log()
			b.WriteString("log\t" + lg.Category + "\t" + lg.Message + "\n")
		}
	}
	return b.String(), tasks, regions, logs
}

// numberFieldInt64 reads a numeric field from a Playwright eval result, which
// decodes JS numbers as float64.
func numberFieldInt64(fields map[string]any, key string) int64 {
	if v, ok := fields[key].(float64); ok {
		return int64(v)
	}
	return 0
}
