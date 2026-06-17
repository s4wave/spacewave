//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
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
// sum of transferred resource bytes and the WASM subtotal. A warm runtime state
// reports a smaller transfer total because cached assets contribute zero
// transfer bytes.
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

// TestGoScriptDriveStartupBench records the time-to-Drive bench for the
// unbundled build across three runtime states on one owned BrowserContext:
//
//   - cold: a fresh context, empty browser storage, and a dedicated WASM
//     process that boots over a cold HTTP cache and empty Space state.
//   - warm: a return visitor; the page is replaced in the same context, so a
//     fresh worker boots over the retained HTTP cache and OPFS Space state.
//   - cache-hot: the asset cache stays warm but the OPFS Space state is cleared
//     before the reboot, isolating asset-load cost from Space-data cost.
//
// Each cell measures navigation start to Drive content-ready milestones, the
// served bundle size, and the Resource SDK connection timing, writing run.json
// under .bldr/e2e-goscript-drive-bench/<stamp>/<cell>/. When the trace service
// is available it also captures the runtime trace, writing runtime.trace and a
// tracetool.txt summary ranked by subsystem prefix. The cache-hot cell needs
// Chromium CDP storage control and is skipped on other browsers. The bench is
// opt-in via E2E_WASM_DRIVE_BENCH so routine e2e does not pay its cost.
func TestGoScriptDriveStartupBench(t *testing.T) {
	skipDriveBenchWhenDisabled(t)

	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve compiler: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()

	h := harness(t)
	// One run stamp groups every cell's artifacts under a single run directory.
	runStamp := time.Now().UTC().Format("20060102-150405")

	// cold: a fresh isolated context boots over empty storage. The session owns
	// this context, which persists across page replacements so the warm and
	// cache-hot cells reuse its retained HTTP cache and OPFS Space state.
	navStart := time.Now()
	sess := h.NewCleanSession(t)
	runDriveBenchCell(t, ctx, h, sess, compiler, driveBenchCellInput{
		runStamp:     runStamp,
		navStart:     navStart,
		liveAppMs:    msSince(navStart),
		buildMode:    "unbundled",
		runtimeState: "cold",
		cell:         "cold-unbundled",
	})

	// warm: a return visitor reboots in the same context with the HTTP cache and
	// OPFS Space state still present. The reboot reuses the persisted browser
	// peer identity, so the harness peer watcher waits for the prior session's
	// mount observation to expire before observing the new one; that wait lands
	// in resourceConnection.durationMs, not the Drive route. The comparable
	// Drive-boot signal across cells is the route-accepted to content-ready
	// segment, where warm Space-state reuse skips content regeneration.
	navStart = time.Now()
	rebootDriveBenchPage(t, ctx, sess)
	runDriveBenchCell(t, ctx, h, sess, compiler, driveBenchCellInput{
		runStamp:     runStamp,
		navStart:     navStart,
		liveAppMs:    msSince(navStart),
		buildMode:    "unbundled",
		runtimeState: "warm",
		cell:         "warm-unbundled",
	})

	// cache-hot: keep the warm asset cache, but clear the OPFS Space state before
	// the reboot so the cell measures a first-run Space with cached assets.
	if h.BrowserName() != "chromium" {
		t.Logf("skipping cache-hot cell: OPFS reset requires chromium CDP, have %q", h.BrowserName())
		return
	}
	navStart = time.Now()
	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page for cache-hot: %v", err)
	}
	if err := resetSpaceStateKeepCache(sess, h.baseURL); err != nil {
		t.Fatalf("reset space state: %v", err)
	}
	bootDriveBenchPage(t, ctx, sess)
	runDriveBenchCell(t, ctx, h, sess, compiler, driveBenchCellInput{
		runStamp:     runStamp,
		navStart:     navStart,
		liveAppMs:    msSince(navStart),
		buildMode:    "unbundled",
		runtimeState: "cache-hot",
		cell:         "cache-hot-unbundled",
	})
}

// driveBenchCellInput carries the per-cell identity and the wall-clock origin a
// cell measures milestones against.
type driveBenchCellInput struct {
	runStamp     string
	navStart     time.Time
	liveAppMs    int64
	buildMode    string
	runtimeState string
	cell         string
}

// runDriveBenchCell opens the Drive route on the session's current page, records
// the cell milestones and bundle/connection summaries, optionally captures the
// runtime trace, and writes the cell's run.json plus trace artifacts.
func runDriveBenchCell(
	t *testing.T,
	ctx context.Context,
	h *Harness,
	sess *TestSession,
	compiler E2EWasmCompiler,
	in driveBenchCellInput,
) {
	t.Helper()
	page := sess.Page()

	var (
		routeAcceptedMs int64
		unixfsVisibleMs int64
		contentReadyMs  int64
		driveReady      DriveReadyResult
	)
	driveOpen := func(context.Context) error {
		NavigateHash(t, h, page, "#/quickstart/drive")
		routeAcceptedMs = msSince(in.navStart)
		WaitForDriveShell(t, page)
		unixfsVisibleMs = msSince(in.navStart)
		driveReady = WaitForDriveReady(t, h, page)
		contentReadyMs = msSince(in.navStart)
		return nil
	}

	var traceData []byte
	traceEnabled := E2EWasmTraceServiceEnabled(compiler)
	if traceEnabled {
		captured, err := sess.CaptureTrace(ctx, "goscript-drive-bench-"+in.runtimeState, driveOpen)
		if err != nil {
			t.Fatalf("capture trace (%s): %v", in.cell, err)
		}
		traceData = captured
	} else if err := driveOpen(ctx); err != nil {
		t.Fatalf("drive open (%s): %v", in.cell, err)
	}

	connTiming := sess.ResourceConnectionTiming()
	run := driveStartupBenchRun{
		Timestamp:    in.navStart.UTC().Format(time.RFC3339Nano),
		Compiler:     string(compiler),
		BuildMode:    in.buildMode,
		RuntimeState: in.runtimeState,
		Cell:         in.cell,
		Milestones: driveStartupBenchMilestones{
			LiveAppMs:       in.liveAppMs,
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

	cellDir := driveBenchCellDir(t, in.runStamp, in.cell)
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
		in.cell, runPath, in.liveAppMs, routeAcceptedMs, unixfsVisibleMs, contentReadyMs, traceEnabled)
}

// rebootDriveBenchPage replaces the session page in its existing warm context
// and reboots the app: a fresh dedicated WASM worker over the retained HTTP
// cache and OPFS Space state.
func rebootDriveBenchPage(t *testing.T, ctx context.Context, sess *TestSession) {
	t.Helper()
	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page: %v", err)
	}
	bootDriveBenchPage(t, ctx, sess)
}

// bootDriveBenchPage loads the app into the session's current (already replaced)
// page and reconnects the Resource SDK client.
func bootDriveBenchPage(t *testing.T, ctx context.Context, sess *TestSession) {
	t.Helper()
	if err := sess.LoadApp(); err != nil {
		t.Fatalf("load app: %v", err)
	}
	WaitForApp(t, sess.Page())
	if err := sess.ConnectResources(ctx); err != nil {
		t.Fatalf("connect resources: %v", err)
	}
}

// resetSpaceStateKeepCache clears the origin's OPFS Space-state storage while
// leaving the HTTP/module asset cache warm, producing the cache-hot runtime
// state. It runs after the page (and the dedicated WASM worker that held OPFS
// access handles) has been replaced, so no worker holds an OPFS lock during the
// clear. Chromium-only: it drives the CDP Storage domain.
func resetSpaceStateKeepCache(sess *TestSession, origin string) error {
	cdp, err := sess.BrowserContext().NewCDPSession(sess.Page())
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
// repo-root .bldr tree, grouped by the run stamp shared across the run's cells.
func driveBenchCellDir(t testing.TB, runStamp string, cell string) string {
	t.Helper()
	repoRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	return filepath.Join(repoRoot, ".bldr", "e2e-goscript-drive-bench", runStamp, cell)
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

// benchTracePrefixes are the Spacewave runtime trace task-type namespaces the
// bench breaks out, so per-cell summaries rank the slowest work by subsystem.
var benchTracePrefixes = []string{"alpha", "hydra", "provider", "bldr"}

// benchTaskAgg accumulates per-task-type timing across a captured trace.
type benchTaskAgg struct {
	typ      string
	count    int
	totalDur time.Duration
	maxDur   time.Duration
}

// summarizeTrace walks the captured trace with the upstream Go trace reader, the
// same parser tracetool's fork is built on, and renders a per-subsystem summary:
// overall task/region/log counts, then one section per known task-type prefix
// (alpha/, hydra/, provider/, bldr/) plus other, ranking task types by total
// active duration with count as the tie-breaker. It returns the rendered text
// and the overall task, region, and log counts.
func summarizeTrace(t testing.TB, data []byte) (string, int, int, int) {
	t.Helper()
	reader, err := exptrace.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("trace reader rejected the capture: %v", err)
	}

	type openTask struct {
		typ   string
		begin exptrace.Time
	}
	var (
		tasks   int
		regions int
		logs    int
	)
	open := map[exptrace.TaskID]openTask{}
	aggByType := map[string]*benchTaskAgg{}
	aggFor := func(typ string) *benchTaskAgg {
		agg := aggByType[typ]
		if agg == nil {
			agg = &benchTaskAgg{typ: typ}
			aggByType[typ] = agg
		}
		return agg
	}

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
			tk := ev.Task()
			open[tk.ID] = openTask{typ: tk.Type, begin: ev.Time()}
		case exptrace.EventTaskEnd:
			tk := ev.Task()
			ot, ok := open[tk.ID]
			if !ok {
				continue
			}
			delete(open, tk.ID)
			typ := ot.typ
			if typ == "" {
				typ = tk.Type
			}
			dur := ev.Time().Sub(ot.begin)
			agg := aggFor(typ)
			agg.count++
			agg.totalDur += dur
			if dur > agg.maxDur {
				agg.maxDur = dur
			}
		case exptrace.EventRegionBegin:
			regions++
		case exptrace.EventLog:
			logs++
		}
	}
	// Tasks that began but never ended still count by type; their duration is
	// unknown, so they contribute count only.
	for _, ot := range open {
		aggFor(ot.typ).count++
	}

	return renderTraceSummary(aggByType, tasks, regions, logs), tasks, regions, logs
}

// renderTraceSummary formats the per-prefix ranked task breakdown for
// tracetool.txt. Each prefix section lists its task types ranked by total
// duration, then count, capped at the slowest twenty.
func renderTraceSummary(aggByType map[string]*benchTaskAgg, tasks, regions, logs int) string {
	var b strings.Builder
	b.WriteString("tasks\t" + strconv.Itoa(tasks) + "\n")
	b.WriteString("regions\t" + strconv.Itoa(regions) + "\n")
	b.WriteString("logs\t" + strconv.Itoa(logs) + "\n")

	buckets := map[string][]*benchTaskAgg{}
	for _, agg := range aggByType {
		prefix := benchTracePrefixFor(agg.typ)
		buckets[prefix] = append(buckets[prefix], agg)
	}

	order := append(slices.Clone(benchTracePrefixes), "other")
	for _, prefix := range order {
		aggs := buckets[prefix]
		if len(aggs) == 0 {
			continue
		}
		slices.SortFunc(aggs, func(a, c *benchTaskAgg) int {
			if a.totalDur != c.totalDur {
				if a.totalDur > c.totalDur {
					return -1
				}
				return 1
			}
			return c.count - a.count
		})
		b.WriteString("\n[" + prefix + "]\n")
		for _, agg := range aggs[:min(len(aggs), 20)] {
			b.WriteString(agg.typ +
				"\tcount=" + strconv.Itoa(agg.count) +
				"\ttotal_us=" + strconv.FormatInt(agg.totalDur.Microseconds(), 10) +
				"\tmax_us=" + strconv.FormatInt(agg.maxDur.Microseconds(), 10) + "\n")
		}
	}
	return b.String()
}

// benchTracePrefixFor returns the known task-type prefix for typ, or "other"
// when its leading path segment is not a tracked subsystem.
func benchTracePrefixFor(typ string) string {
	seg := typ
	if before, _, ok := strings.Cut(typ, "/"); ok {
		seg = before
	}
	if slices.Contains(benchTracePrefixes, seg) {
		return seg
	}
	return "other"
}

// numberFieldInt64 reads a numeric field from a Playwright eval result, which
// decodes JS numbers as float64.
func numberFieldInt64(fields map[string]any, key string) int64 {
	if v, ok := fields[key].(float64); ok {
		return int64(v)
	}
	return 0
}
