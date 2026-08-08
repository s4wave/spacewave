//go:build !skip_e2e && !js

package wasm

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/e2e/drivebench"
	exptrace "golang.org/x/exp/trace"
)

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
	cellDir, err := drivebench.CellDir(in.runStamp, in.cell)
	if err != nil {
		t.Fatalf("resolve cell dir: %v", err)
	}

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
	openAndTrace := func(ctx context.Context) error {
		if traceEnabled {
			captured, err := sess.CaptureTrace(ctx, "goscript-drive-bench-"+in.runtimeState, driveOpen)
			if err != nil {
				return errors.Wrapf(err, "capture trace (%s)", in.cell)
			}
			traceData = captured
			return nil
		}
		return driveOpen(ctx)
	}
	browserProfile, err := captureDriveBenchBrowserProfile(t, ctx, h, sess, cellDir, openAndTrace)
	if err != nil {
		t.Fatalf("drive open (%s): %v", in.cell, err)
	}

	connTiming := sess.ResourceConnectionTiming()
	run := drivebench.Run{
		Timestamp:    in.navStart.UTC().Format(time.RFC3339Nano),
		Compiler:     string(compiler),
		BuildMode:    in.buildMode,
		RuntimeState: in.runtimeState,
		Cell:         in.cell,
		Milestones: drivebench.Milestones{
			LiveAppMs:       in.liveAppMs,
			RouteAcceptedMs: routeAcceptedMs,
			UnixfsVisibleMs: unixfsVisibleMs,
			ContentReadyMs:  contentReadyMs,
		},
		Browser: drivebench.BrowserFromQuickstartTiming(
			driveReady.ContentReadyMs,
			driveReady.QuickstartTiming,
		),
		ResourceConnection: summarizeResourceConnection(connTiming),
	}
	if browserProfile != nil {
		run.BrowserProfile = browserProfile
	}
	if raw, err := page.Evaluate(drivebench.StartupMarksScript); err != nil {
		t.Logf("read startup marks (%s): %v", in.cell, err)
	} else {
		run.Browser.StartupMarks = drivebench.ParseStartupMarks(raw)
	}
	// ServedBundle stays nil: bundle size is a bundled-build metric the
	// releasewasm bench measures from the built production bundle on disk. The
	// unbundled dev build serves an unbundled module graph with no single bundle,
	// and its worker-loaded modules never appear on the page resource timeline.

	if traceEnabled {
		tracePath := filepath.Join(cellDir, "runtime.trace")
		if err := drivebench.WriteArtifact(tracePath, traceData); err != nil {
			t.Fatalf("write runtime.trace: %v", err)
		}
		tracetoolPath := filepath.Join(cellDir, "tracetool.txt")
		summary, tasks, regions, logs, taskAggs, operationShape := summarizeTrace(t, traceData)
		if err := drivebench.WriteArtifact(tracetoolPath, []byte(summary)); err != nil {
			t.Fatalf("write tracetool.txt: %v", err)
		}
		run.Trace = &drivebench.Trace{
			Bytes:            len(traceData),
			RuntimeTracePath: tracePath,
			TracetoolPath:    tracetoolPath,
			UserTasks:        tasks,
			UserRegions:      regions,
			UserLogs:         logs,
			Tasks:            taskAggs,
		}
		run.OperationShape = operationShape
	}

	runPath, err := drivebench.WriteRun(cellDir, run)
	if err != nil {
		t.Fatalf("write run.json: %v", err)
	}
	t.Logf("drive bench cell %s written to %s (live=%dms route=%dms unixfs=%dms content=%dms trace=%v)",
		in.cell, runPath, in.liveAppMs, routeAcceptedMs, unixfsVisibleMs, contentReadyMs, traceEnabled)
}

func captureDriveBenchBrowserProfile(
	t testing.TB,
	ctx context.Context,
	h *Harness,
	sess *TestSession,
	cellDir string,
	work func(context.Context) error,
) (*drivebench.BrowserProfile, error) {
	t.Helper()
	if !E2EWasmDriveBenchJSProfileEnabled() {
		return nil, work(ctx)
	}
	profile := &drivebench.BrowserProfile{
		Captured:      false,
		CaptureWindow: "drive-open",
	}
	if h.BrowserName() != "chromium" {
		profile.SkippedReason = "Chromium Profiler is only available for the chromium Drive bench browser"
		return profile, work(ctx)
	}

	cdp, err := sess.BrowserContext().NewCDPSession(sess.Page())
	if err != nil {
		return nil, errors.Wrap(err, "new cdp session")
	}
	defer cdp.Detach()
	if _, err := cdp.Send("Profiler.enable", nil); err != nil {
		return nil, errors.Wrap(err, "enable profiler")
	}
	if _, err := cdp.Send("Profiler.start", nil); err != nil {
		return nil, errors.Wrap(err, "start profiler")
	}
	started := time.Now().UTC()
	workErr := work(ctx)
	resp, stopErr := cdp.Send("Profiler.stop", nil)
	stopped := time.Now().UTC()
	if workErr != nil {
		return nil, workErr
	}
	if stopErr != nil {
		return nil, errors.Wrap(stopErr, "stop profiler")
	}

	rawProfile := resp
	if respObj, ok := resp.(map[string]any); ok {
		rawProfile = respObj
		if profileValue, ok := respObj["profile"]; ok {
			rawProfile = profileValue
		}
	}
	data := marshalBrowserProfileJSON(rawProfile)
	profilePath := filepath.Join(cellDir, "browser-js-profile.cpuprofile")
	if err := drivebench.WriteArtifact(profilePath, data); err != nil {
		return nil, errors.Wrap(err, "write browser JS profile")
	}
	buckets := summarizeBrowserCPUProfile(rawProfile)
	summaryPath := filepath.Join(cellDir, "browser-js-profile-summary.txt")
	if err := drivebench.WriteArtifact(summaryPath, renderBrowserProfileSummary(buckets)); err != nil {
		return nil, errors.Wrap(err, "write browser JS profile summary")
	}

	profile.Captured = true
	profile.ProfilePath = profilePath
	profile.SummaryPath = summaryPath
	profile.StartedAt = started.Format(time.RFC3339Nano)
	profile.StoppedAt = stopped.Format(time.RFC3339Nano)
	profile.Bytes = len(data)
	profile.Buckets = buckets
	return profile, nil
}

func marshalBrowserProfileJSON(value any) []byte {
	var arena fastjson.Arena
	data := marshalCDPValue(&arena, value).MarshalTo(nil)
	data = append(data, '\n')
	return data
}

func marshalCDPValue(arena *fastjson.Arena, value any) *fastjson.Value {
	switch typed := value.(type) {
	case nil:
		return arena.NewNull()
	case bool:
		if typed {
			return arena.NewTrue()
		}
		return arena.NewFalse()
	case string:
		return arena.NewString(typed)
	case int:
		return arena.NewNumberInt(typed)
	case int64:
		return arena.NewNumberString(strconv.FormatInt(typed, 10))
	case uint64:
		return arena.NewNumberString(strconv.FormatUint(typed, 10))
	case float64:
		return arena.NewNumberString(strconv.FormatFloat(typed, 'f', -1, 64))
	case []any:
		arr := arena.NewArray()
		for _, item := range typed {
			arr.SetArrayItem(len(arr.GetArray()), marshalCDPValue(arena, item))
		}
		return arr
	case map[string]any:
		obj := arena.NewObject()
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			obj.Set(key, marshalCDPValue(arena, typed[key]))
		}
		return obj
	default:
		return arena.NewNull()
	}
}

func summarizeBrowserCPUProfile(profile any) []drivebench.ProfileBucket {
	obj, ok := profile.(map[string]any)
	if !ok {
		return nil
	}
	nodesRaw, _ := obj["nodes"].([]any)
	if len(nodesRaw) == 0 {
		return nil
	}
	type nodeInfo struct {
		bucket string
	}
	nodes := make(map[int64]nodeInfo, len(nodesRaw))
	parentByID := make(map[int64]int64, len(nodesRaw))
	for _, raw := range nodesRaw {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := cdpInt64(node["id"])
		if id == 0 {
			continue
		}
		callFrame, _ := node["callFrame"].(map[string]any)
		bucket := browserProfileBucketName(cdpString(callFrame["url"]), cdpString(callFrame["functionName"]))
		if rawChildren, _ := node["children"].([]any); len(rawChildren) != 0 {
			for _, childRaw := range rawChildren {
				childID := cdpInt64(childRaw)
				if childID != 0 {
					parentByID[childID] = id
				}
			}
		}
		nodes[id] = nodeInfo{bucket: bucket}
	}

	buckets := map[string]*drivebench.ProfileBucket{}
	addSelf := func(name string, delta int64) {
		bucket := profileBucket(buckets, name)
		bucket.Count++
		bucket.SelfUs += delta
	}
	addTotal := func(name string, delta int64) {
		profileBucket(buckets, name).TotalUs += delta
	}

	samples, _ := obj["samples"].([]any)
	timeDeltas, _ := obj["timeDeltas"].([]any)
	for i, sampleRaw := range samples {
		id := cdpInt64(sampleRaw)
		info, ok := nodes[id]
		if !ok {
			continue
		}
		var delta int64
		if i < len(timeDeltas) {
			delta = cdpInt64(timeDeltas[i])
		}
		addSelf(info.bucket, delta)
		seenTotal := map[string]struct{}{}
		for nodeID := id; nodeID != 0; nodeID = parentByID[nodeID] {
			node, ok := nodes[nodeID]
			if !ok {
				break
			}
			if _, ok := seenTotal[node.bucket]; ok {
				continue
			}
			seenTotal[node.bucket] = struct{}{}
			addTotal(node.bucket, delta)
		}
	}
	if len(samples) == 0 {
		for _, raw := range nodesRaw {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id := cdpInt64(node["id"])
			if id == 0 {
				continue
			}
			hitCount := cdpInt64(node["hitCount"])
			if hitCount == 0 {
				continue
			}
			addSelf(nodes[id].bucket, 0)
			buckets[nodes[id].bucket].Count += int(hitCount - 1)
		}
	}

	out := make([]drivebench.ProfileBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, *bucket)
	}
	slices.SortFunc(out, func(a, b drivebench.ProfileBucket) int {
		if a.SelfUs != b.SelfUs {
			if a.SelfUs > b.SelfUs {
				return -1
			}
			return 1
		}
		if a.TotalUs != b.TotalUs {
			if a.TotalUs > b.TotalUs {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func profileBucket(buckets map[string]*drivebench.ProfileBucket, name string) *drivebench.ProfileBucket {
	bucket := buckets[name]
	if bucket != nil {
		return bucket
	}
	bucket = &drivebench.ProfileBucket{Name: name}
	buckets[name] = bucket
	return bucket
}

func browserProfileBucketName(url, functionName string) string {
	text := strings.ToLower(url + " " + functionName)
	switch {
	case strings.Contains(text, "gs/builtin") ||
		strings.Contains(text, "goscript") ||
		strings.Contains(text, "$."):
		return "goscript-runtime"
	case strings.Contains(text, "opfs") ||
		strings.Contains(text, "blockshard") ||
		strings.Contains(text, "web lock"):
		return "storage-opfs"
	case strings.Contains(text, "db/block") ||
		strings.Contains(text, "block-gc") ||
		strings.Contains(text, "cayley") ||
		strings.Contains(text, "world-graph"):
		return "storage-block-graph"
	case strings.Contains(text, "quickstart") ||
		strings.Contains(text, "drive"):
		return "app-quickstart"
	case url == "":
		return "browser-runtime"
	default:
		return "other-js"
	}
}

func cdpString(value any) string {
	text, _ := value.(string)
	return text
}

func cdpInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case uint64:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func renderBrowserProfileSummary(buckets []drivebench.ProfileBucket) []byte {
	var b strings.Builder
	b.WriteString("bucket\tcount\tself_us\ttotal_us\n")
	for _, bucket := range buckets {
		b.WriteString(bucket.Name)
		b.WriteByte('\t')
		b.WriteString(strconv.Itoa(bucket.Count))
		b.WriteByte('\t')
		b.WriteString(strconv.FormatInt(bucket.SelfUs, 10))
		b.WriteByte('\t')
		b.WriteString(strconv.FormatInt(bucket.TotalUs, 10))
		b.WriteByte('\n')
	}
	return []byte(b.String())
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

// summarizeResourceConnection reduces the session connection timing to the cell
// schema fields.
func summarizeResourceConnection(timing ResourceConnectionTiming) drivebench.ResourceConn {
	var durationMs int64
	if !timing.StartedAt.IsZero() && !timing.CompletedAt.IsZero() {
		durationMs = timing.CompletedAt.Sub(timing.StartedAt).Milliseconds()
	}
	return drivebench.ResourceConn{
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
// active duration with count as the tie-breaker. It returns the rendered text,
// raw task aggregates, and a compact operation-shape projection from task timing
// plus numeric trace-log payloads.
func summarizeTrace(t testing.TB, data []byte) (string, int, int, int, []drivebench.Task, *drivebench.OperationShape) {
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
	operationShape := newOperationShapeCollector()
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
			operationShape.addTask(typ, dur)
		case exptrace.EventRegionBegin:
			regions++
		case exptrace.EventLog:
			logs++
			operationShape.addLog(ev.Log())
		}
	}
	// Tasks that began but never ended still count by type; their duration is
	// unknown, so they contribute count only.
	for _, ot := range open {
		aggFor(ot.typ).count++
		operationShape.addOpenTask(ot.typ)
	}

	taskAggs := make([]drivebench.Task, 0, len(aggByType))
	for _, agg := range aggByType {
		taskAggs = append(taskAggs, drivebench.Task{
			Type:    agg.typ,
			Count:   agg.count,
			TotalUs: agg.totalDur.Microseconds(),
			MaxUs:   agg.maxDur.Microseconds(),
		})
	}
	slices.SortFunc(taskAggs, func(a, c drivebench.Task) int {
		if a.TotalUs != c.TotalUs {
			if a.TotalUs > c.TotalUs {
				return -1
			}
			return 1
		}
		if a.Count != c.Count {
			return c.Count - a.Count
		}
		return strings.Compare(a.Type, c.Type)
	})

	return renderTraceSummary(aggByType, tasks, regions, logs), tasks, regions, logs, taskAggs, operationShape.build()
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

var operationShapeOrder = []string{
	"write-transaction",
	"block-write",
	"cayley-delta",
	"gc-wal",
	"opfs-publish",
	"startup-replay",
}

type operationShapeCollector struct {
	ops map[string]*operationShapeSummary
}

type operationShapeSummary struct {
	name     string
	count    int
	totalUs  int64
	maxUs    int64
	logCount int
	fields   map[string]*drivebench.OperationField
}

func newOperationShapeCollector() *operationShapeCollector {
	return &operationShapeCollector{ops: map[string]*operationShapeSummary{}}
}

func (c *operationShapeCollector) addTask(typ string, dur time.Duration) {
	op := operationNameForTraceTask(typ)
	if op == "" {
		return
	}
	summary := c.summary(op)
	summary.count++
	durUs := dur.Microseconds()
	summary.totalUs += durUs
	if durUs > summary.maxUs {
		summary.maxUs = durUs
	}
}

func (c *operationShapeCollector) addOpenTask(typ string) {
	op := operationNameForTraceTask(typ)
	if op == "" {
		return
	}
	c.summary(op).count++
}

func (c *operationShapeCollector) addLog(log exptrace.Log) {
	op := operationNameForTraceLog(log.Category)
	if op == "" {
		return
	}
	summary := c.summary(op)
	summary.logCount++
	for key, value := range parseTraceLogNumericFields(log.Message) {
		fieldName := operationTraceLogFieldName(log.Category, key)
		field := summary.fields[fieldName]
		if field == nil {
			field = &drivebench.OperationField{Name: fieldName}
			summary.fields[fieldName] = field
		}
		field.Samples++
		field.Sum += value
		field.Last = value
		if field.Samples == 1 || value > field.Max {
			field.Max = value
		}
	}
}

func (c *operationShapeCollector) summary(name string) *operationShapeSummary {
	summary := c.ops[name]
	if summary != nil {
		return summary
	}
	summary = &operationShapeSummary{name: name, fields: map[string]*drivebench.OperationField{}}
	c.ops[name] = summary
	return summary
}

func (c *operationShapeCollector) build() *drivebench.OperationShape {
	if len(c.ops) == 0 {
		return nil
	}
	shape := &drivebench.OperationShape{}
	for _, name := range operationShapeOrder {
		summary := c.ops[name]
		if summary == nil {
			continue
		}
		shape.Operations = append(shape.Operations, summary.build())
	}
	return shape
}

func (s *operationShapeSummary) build() drivebench.OperationSummary {
	fields := make([]drivebench.OperationField, 0, len(s.fields))
	for _, field := range s.fields {
		fields = append(fields, *field)
	}
	slices.SortFunc(fields, func(a, b drivebench.OperationField) int {
		return strings.Compare(a.Name, b.Name)
	})
	return drivebench.OperationSummary{
		Name:     s.name,
		Count:    s.count,
		TotalUs:  s.totalUs,
		MaxUs:    s.maxUs,
		LogCount: s.logCount,
		Fields:   fields,
	}
}

func operationNameForTraceTask(typ string) string {
	switch {
	case typ == "alpha/so-engine/write-tx/hold-write-mtx" ||
		strings.HasPrefix(typ, "alpha/so-engine/write-tx/"):
		return "write-transaction"
	case typ == "hydra/block/transaction/write-at-root" ||
		strings.HasPrefix(typ, "hydra/block/transaction/write-at-root/") ||
		strings.HasPrefix(typ, "hydra/block/buffered-store/"):
		return "block-write"
	case typ == "cayley/kv/apply-deltas" ||
		strings.HasPrefix(typ, "cayley/kv/apply-deltas/"):
		return "cayley-delta"
	case strings.HasPrefix(typ, "hydra/world-graph/"):
		return "cayley-delta"
	case typ == "hydra/block-gc/manager/startup-replay" ||
		strings.HasPrefix(typ, "hydra/block-gc/manager/startup-replay/"):
		return "startup-replay"
	case strings.HasPrefix(typ, "hydra/block-gc/"):
		return "gc-wal"
	case strings.HasPrefix(typ, "hydra/opfs-blockshard/block-store/"):
		return "opfs-publish"
	case typ == "hydra/opfs-blockshard/run-actor/publish" ||
		strings.HasPrefix(typ, "hydra/opfs-blockshard/run-actor/publish/") ||
		strings.HasPrefix(typ, "hydra/opfs-blockshard/shard/publish/"):
		return "opfs-publish"
	default:
		return ""
	}
}

func operationNameForTraceLog(category string) string {
	if category == "coalesce" {
		return "opfs-publish"
	}
	return operationNameForTraceTask(category)
}

func operationTraceLogFieldName(category, key string) string {
	prefix := category
	for _, base := range []string{
		"alpha/so-engine/write-tx/",
		"hydra/block/transaction/write-at-root/",
		"hydra/block/buffered-store/",
		"hydra/block-gc/store/flush-pending/",
		"hydra/block-gc/refgraph/apply-ref-batch/",
		"hydra/block-gc/wal/",
		"hydra/opfs-blockshard/run-actor/publish/",
		"hydra/opfs-blockshard/block-store/",
		"hydra/opfs-blockshard/shard/publish/",
		"hydra/world-graph/",
		"cayley/kv/apply-deltas/",
	} {
		if after, ok := strings.CutPrefix(category, base); ok {
			prefix = after
			break
		}
	}
	prefix = strings.ReplaceAll(prefix, "/", ".")
	return prefix + "." + key
}

func parseTraceLogNumericFields(message string) map[string]int64 {
	fields := map[string]int64{}
	for part := range strings.FieldsSeq(message) {
		key, valueText, ok := strings.Cut(part, "=")
		if !ok || key == "" || valueText == "" {
			continue
		}
		valueText = strings.TrimRight(valueText, ",;")
		value, err := strconv.ParseInt(valueText, 10, 64)
		if err != nil {
			continue
		}
		fields[key] = value
	}
	return fields
}
