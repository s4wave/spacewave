//go:build !skip_e2e && !js

package memlab

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	wasm "github.com/s4wave/spacewave/e2e/wasm"
	"github.com/sirupsen/logrus"
)

var testHarness *wasm.Harness

type runtimePairCount struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Count   int    `json:"count"`
}

type runtimeResourceIDCount struct {
	ResourceID int `json:"resourceId"`
	Count      int `json:"count"`
}

type runtimeRPCSummary struct {
	TotalCount   int                `json:"totalCount"`
	ActiveCount  int                `json:"activeCount"`
	ClosedCount  int                `json:"closedCount"`
	ActiveByPair []runtimePairCount `json:"activeByPair"`
	ClosedByPair []runtimePairCount `json:"closedByPair"`
}

type runtimeResourceSummary struct {
	ClientTotalCount         int                      `json:"clientTotalCount"`
	ClientActiveCount        int                      `json:"clientActiveCount"`
	ClientDisposedCount      int                      `json:"clientDisposedCount"`
	RefTotalCount            int                      `json:"refTotalCount"`
	RefActiveCount           int                      `json:"refActiveCount"`
	RefReleasedCount         int                      `json:"refReleasedCount"`
	ActiveRefsByResourceID   []runtimeResourceIDCount `json:"activeRefsByResourceId"`
	ReleasedRefsByResourceID []runtimeResourceIDCount `json:"releasedRefsByResourceId"`
}

type runtimeDebugSummary struct {
	RPC      runtimeRPCSummary      `json:"rpc"`
	Resource runtimeResourceSummary `json:"resource"`
}

type runtimePairDelta struct {
	Service       string
	Method        string
	BaselineCount int
	FinalCount    int
	Delta         int
}

func TestMain(m *testing.M) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if !wasm.E2EWasmEnabled() {
		le.Info("skipping e2e/wasm/memlab package; set ENABLE_E2E_WASM=true to run")
		os.Exit(0)
	}

	ctx := context.Background()

	h, err := wasm.Boot(ctx, le, wasm.WithHeadless(true))
	if err != nil {
		le.WithError(err).Fatal("boot wasm harness")
	}

	if err := h.LaunchBrowser(); err != nil {
		h.Release()
		le.WithError(err).Fatal("launch browser")
	}

	if err := h.CompileScripts(".."); err != nil {
		h.Release()
		le.WithError(err).Fatal("compile test scripts")
	}

	testHarness = h

	// Ensure Node.js analysis dependencies are installed.
	if err := EnsureDeps(); err != nil {
		h.Release()
		le.WithError(err).Fatal("install memlab deps")
	}

	code := m.Run()
	h.Release()
	os.Exit(code)
}

// snapshotDir returns a per-test snapshot directory under testdata/.
func snapshotDir(t *testing.T) string {
	dir := filepath.Join(scriptDir(), "testdata", t.Name())
	os.MkdirAll(dir, 0o755)
	return dir
}

func captureRuntimeDebugSummary(
	t *testing.T,
	s *wasm.TestSession,
) (runtimeDebugSummary, bool) {
	t.Helper()

	h := testHarness
	snapshotScript := h.Script("runtime-debug.ts")
	snapshotArgs := map[string]any{"op": "snapshot"}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if raw, err := s.Page().Evaluate(snapshotScript, snapshotArgs); err == nil {
			if rawJSON, ok := raw.(string); ok && rawJSON != "null" {
				summary, err := parseRuntimeDebugSummary(rawJSON)
				if err != nil {
					t.Fatalf("parse runtime debug summary: %v", err)
				}
				return summary, true
			}
		}
		for _, w := range s.Workers() {
			raw, err := w.Evaluate(snapshotScript, snapshotArgs)
			if err != nil {
				continue
			}
			rawJSON, ok := raw.(string)
			if !ok || rawJSON == "null" {
				continue
			}
			summary, err := parseRuntimeDebugSummary(rawJSON)
			if err != nil {
				t.Fatalf("parse runtime debug summary: %v", err)
			}
			return summary, true
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	typeofArgs := map[string]any{"op": "typeof"}
	workerURLs := make([]string, 0, len(s.Workers()))
	workerTypes := make([]string, 0, len(s.Workers()))
	for _, w := range s.Workers() {
		workerURLs = append(workerURLs, w.URL())
		raw, err := w.Evaluate(snapshotScript, typeofArgs)
		if err != nil {
			workerTypes = append(workerTypes, "eval-error:"+err.Error())
			continue
		}
		workerTypes = append(workerTypes, raw.(string))
	}
	pageType := "unknown"
	if raw, err := s.Page().Evaluate(snapshotScript, typeofArgs); err == nil {
		if v, ok := raw.(string); ok {
			pageType = v
		}
	}
	t.Logf(
		"runtime debug summary unavailable, continuing without it: page typeof=%s workers=%d urls=%v types=%v",
		pageType,
		len(workerURLs),
		workerURLs,
		workerTypes,
	)
	return runtimeDebugSummary{}, false
}

func parseRuntimeDebugSummary(dat string) (runtimeDebugSummary, error) {
	var p fastjson.Parser
	v, err := p.Parse(dat)
	if err != nil {
		return runtimeDebugSummary{}, err
	}
	return runtimeDebugSummary{
		RPC: runtimeRPCSummary{
			TotalCount:   v.GetInt("rpc", "totalCount"),
			ActiveCount:  v.GetInt("rpc", "activeCount"),
			ClosedCount:  v.GetInt("rpc", "closedCount"),
			ActiveByPair: parseRuntimePairCounts(v.GetArray("rpc", "activeByPair")),
			ClosedByPair: parseRuntimePairCounts(v.GetArray("rpc", "closedByPair")),
		},
		Resource: runtimeResourceSummary{
			ClientTotalCount:         v.GetInt("resource", "clientTotalCount"),
			ClientActiveCount:        v.GetInt("resource", "clientActiveCount"),
			ClientDisposedCount:      v.GetInt("resource", "clientDisposedCount"),
			RefTotalCount:            v.GetInt("resource", "refTotalCount"),
			RefActiveCount:           v.GetInt("resource", "refActiveCount"),
			RefReleasedCount:         v.GetInt("resource", "refReleasedCount"),
			ActiveRefsByResourceID:   parseRuntimeResourceIDCounts(v.GetArray("resource", "activeRefsByResourceId")),
			ReleasedRefsByResourceID: parseRuntimeResourceIDCounts(v.GetArray("resource", "releasedRefsByResourceId")),
		},
	}, nil
}

func parseRuntimePairCounts(values []*fastjson.Value) []runtimePairCount {
	counts := make([]runtimePairCount, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		counts = append(counts, runtimePairCount{
			Service: string(value.GetStringBytes("service")),
			Method:  string(value.GetStringBytes("method")),
			Count:   value.GetInt("count"),
		})
	}
	return counts
}

func parseRuntimeResourceIDCounts(values []*fastjson.Value) []runtimeResourceIDCount {
	counts := make([]runtimeResourceIDCount, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		counts = append(counts, runtimeResourceIDCount{
			ResourceID: value.GetInt("resourceId"),
			Count:      value.GetInt("count"),
		})
	}
	return counts
}

func buildPairDeltas(
	baseline []runtimePairCount,
	final []runtimePairCount,
) []runtimePairDelta {
	baselineMap := make(map[string]int, len(baseline))
	for _, entry := range baseline {
		baselineMap[entry.Service+"/"+entry.Method] = entry.Count
	}
	finalMap := make(map[string]int, len(final))
	for _, entry := range final {
		finalMap[entry.Service+"/"+entry.Method] = entry.Count
	}
	keys := make(map[string]struct{}, len(baselineMap)+len(finalMap))
	for key := range baselineMap {
		keys[key] = struct{}{}
	}
	for key := range finalMap {
		keys[key] = struct{}{}
	}
	deltas := make([]runtimePairDelta, 0, len(keys))
	for key := range keys {
		slash := len(key)
		for i := len(key) - 1; i >= 0; i-- {
			if key[i] == '/' {
				slash = i
				break
			}
		}
		baselineCount := baselineMap[key]
		finalCount := finalMap[key]
		delta := finalCount - baselineCount
		if delta == 0 {
			continue
		}
		deltas = append(deltas, runtimePairDelta{
			Service:       key[:slash],
			Method:        key[slash+1:],
			BaselineCount: baselineCount,
			FinalCount:    finalCount,
			Delta:         delta,
		})
	}
	slices.SortFunc(deltas, func(a, b runtimePairDelta) int {
		if a.Delta != b.Delta {
			return b.Delta - a.Delta
		}
		if a.FinalCount != b.FinalCount {
			return b.FinalCount - a.FinalCount
		}
		if a.Service != b.Service {
			if a.Service < b.Service {
				return -1
			}
			return 1
		}
		if a.Method < b.Method {
			return -1
		}
		if a.Method > b.Method {
			return 1
		}
		return 0
	})
	return deltas
}

func logRuntimeDebugDelta(
	t *testing.T,
	baseline runtimeDebugSummary,
	final runtimeDebugSummary,
) {
	t.Helper()

	t.Logf(
		"runtime RPC counts: active %d -> %d, closed %d -> %d, total %d -> %d",
		baseline.RPC.ActiveCount,
		final.RPC.ActiveCount,
		baseline.RPC.ClosedCount,
		final.RPC.ClosedCount,
		baseline.RPC.TotalCount,
		final.RPC.TotalCount,
	)
	t.Logf(
		"runtime resource counts: clients active %d -> %d, refs active %d -> %d, refs released %d -> %d",
		baseline.Resource.ClientActiveCount,
		final.Resource.ClientActiveCount,
		baseline.Resource.RefActiveCount,
		final.Resource.RefActiveCount,
		baseline.Resource.RefReleasedCount,
		final.Resource.RefReleasedCount,
	)

	activePairDeltas := buildPairDeltas(
		baseline.RPC.ActiveByPair,
		final.RPC.ActiveByPair,
	)
	if len(activePairDeltas) > 0 {
		t.Log("runtime active RPC pair deltas:")
		for _, entry := range activePairDeltas {
			t.Logf(
				"  %s/%s: %d -> %d (delta %+d)",
				entry.Service,
				entry.Method,
				entry.BaselineCount,
				entry.FinalCount,
				entry.Delta,
			)
		}
	}

	closedPairDeltas := buildPairDeltas(
		baseline.RPC.ClosedByPair,
		final.RPC.ClosedByPair,
	)
	if len(closedPairDeltas) > 0 {
		t.Log("runtime closed RPC pair deltas:")
		for _, entry := range closedPairDeltas {
			t.Logf(
				"  %s/%s: %d -> %d (delta %+d)",
				entry.Service,
				entry.Method,
				entry.BaselineCount,
				entry.FinalCount,
				entry.Delta,
			)
		}
	}
}

func maybeLogRuntimeDebugDelta(
	t *testing.T,
	baseline runtimeDebugSummary,
	haveBaseline bool,
	final runtimeDebugSummary,
	haveFinal bool,
) {
	t.Helper()

	if !haveBaseline && !haveFinal {
		t.Log("runtime debug summary unavailable in both snapshots")
		return
	}
	if !haveBaseline || !haveFinal {
		t.Logf(
			"runtime debug summary partially unavailable: baseline=%t final=%t",
			haveBaseline,
			haveFinal,
		)
		return
	}

	logRuntimeDebugDelta(t, baseline, final)
}

func logSnapshotSeries(t *testing.T, result *AnalysisResult) {
	t.Helper()

	if len(result.Snapshots) == 0 {
		t.Log("no snapshots captured")
		return
	}

	t.Log("per-snapshot retained object counts:")
	for _, snap := range result.Snapshots {
		t.Logf(
			"  %s: ClientRpc=%d ChannelStream=%d Promise=%d Generator=%d OnNext=%d",
			snap.Label,
			snap.Counts.ClientRpc,
			snap.Counts.ChannelStream,
			snap.Counts.Promise,
			snap.Counts.Generator,
			snap.Counts.OnNext,
		)
		for _, entry := range snap.TopRetained {
			t.Logf("    %s/%s: %d", entry.Service, entry.Method, entry.Count)
		}
	}
}

// TestDriveScenario tests for memory leaks in the quickstart/drive flow.
func TestDriveScenario(t *testing.T) {
	h := testHarness
	s := h.NewPageSession(t)

	wasm.WaitForApp(t, s.Page())
	snaps := NewSnapshotSet(snapshotDir(t))
	baselineRuntime, haveBaselineRuntime := captureRuntimeDebugSummary(t, s)

	// Baseline snapshot on landing page.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "baseline"); err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}

	// Navigate to drive quickstart without reloading the WASM app.
	wasm.NavigateHash(t, h, s.Page(), "#/quickstart/drive")
	time.Sleep(10 * time.Second)

	// Action snapshot after drive is loaded.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "action"); err != nil {
		t.Fatalf("action snapshot: %v", err)
	}

	// Navigate away to trigger cleanup without a full page reload.
	wasm.NavigateHash(t, h, s.Page(), "")
	time.Sleep(5 * time.Second)

	// Cleanup snapshot.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "cleanup"); err != nil {
		t.Fatalf("cleanup snapshot: %v", err)
	}
	finalRuntime, haveFinalRuntime := captureRuntimeDebugSummary(t, s)
	maybeLogRuntimeDebugDelta(
		t,
		baselineRuntime,
		haveBaselineRuntime,
		finalRuntime,
		haveFinalRuntime,
	)

	// Analyze and assert.
	result, err := RunAnalysis(snaps)
	if err != nil {
		t.Fatalf("analysis: %v", err)
	}
	AssertDeltas(t, result, DefaultThresholds())
}

// TestWatchCleanupScenario tests that watch RPC streams clean up on navigation.
func TestWatchCleanupScenario(t *testing.T) {
	h := testHarness
	s := h.NewPageSession(t)

	wasm.WaitForApp(t, s.Page())
	snaps := NewSnapshotSet(snapshotDir(t))
	baselineRuntime, haveBaselineRuntime := captureRuntimeDebugSummary(t, s)

	// Baseline on landing.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "baseline"); err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}

	// Navigate to session dashboard (has multiple watch RPCs) without reload.
	wasm.NavigateHash(t, h, s.Page(), "#/u/1")
	time.Sleep(8 * time.Second)

	// Navigate away without a full page reload.
	wasm.NavigateHash(t, h, s.Page(), "")
	time.Sleep(5 * time.Second)

	// Cleanup snapshot.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "cleanup"); err != nil {
		t.Fatalf("cleanup snapshot: %v", err)
	}
	finalRuntime, haveFinalRuntime := captureRuntimeDebugSummary(t, s)
	maybeLogRuntimeDebugDelta(
		t,
		baselineRuntime,
		haveBaselineRuntime,
		finalRuntime,
		haveFinalRuntime,
	)

	result, err := RunAnalysis(snaps)
	if err != nil {
		t.Fatalf("analysis: %v", err)
	}
	AssertDeltas(t, result, DefaultThresholds())
}

// TestIdleBaselineScenario tests for background leaks on an idle landing page.
func TestIdleBaselineScenario(t *testing.T) {
	h := testHarness
	s := h.NewPageSession(t)

	wasm.WaitForApp(t, s.Page())
	snaps := NewSnapshotSet(snapshotDir(t))
	baselineRuntime, haveBaselineRuntime := captureRuntimeDebugSummary(t, s)

	// Immediate baseline.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "baseline"); err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}

	// Idle for 30 seconds.
	time.Sleep(30 * time.Second)

	// Post-idle snapshot.
	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "cleanup"); err != nil {
		t.Fatalf("idle snapshot: %v", err)
	}
	finalRuntime, haveFinalRuntime := captureRuntimeDebugSummary(t, s)
	maybeLogRuntimeDebugDelta(
		t,
		baselineRuntime,
		haveBaselineRuntime,
		finalRuntime,
		haveFinalRuntime,
	)

	result, err := RunAnalysis(snaps)
	if err != nil {
		t.Fatalf("analysis: %v", err)
	}

	// Tighter thresholds for idle: near-zero growth expected.
	idle := Thresholds{
		ClientRpc:     2,
		ChannelStream: 2,
		Promise:       50,
		Generator:     20,
		OnNext:        20,
	}
	AssertDeltas(t, result, idle)
}

// TestDriveIdleGrowthScenario captures a time series after drive is fully loaded
// to determine whether retained RPC/watch objects continue increasing while idle.
func TestDriveIdleGrowthScenario(t *testing.T) {
	h := testHarness
	s := h.NewPageSession(t)

	wasm.WaitForApp(t, s.Page())
	wasm.NavigateHash(t, h, s.Page(), "#/quickstart/drive")
	wasm.WaitForDriveReady(t, h, s.Page())

	snaps := NewSnapshotSet(snapshotDir(t))

	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "t1"); err != nil {
		t.Fatalf("t1 snapshot: %v", err)
	}

	time.Sleep(15 * time.Second)

	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "t2"); err != nil {
		t.Fatalf("t2 snapshot: %v", err)
	}

	time.Sleep(15 * time.Second)

	if err := snaps.CaptureSnapshot(s.BrowserContext(), s.Page(), "t3"); err != nil {
		t.Fatalf("t3 snapshot: %v", err)
	}

	result, err := RunAnalysis(snaps)
	if err != nil {
		t.Fatalf("analysis: %v", err)
	}

	logSnapshotSeries(t, result)

	idle := Thresholds{
		ClientRpc:     2,
		ChannelStream: 2,
		Promise:       50,
		Generator:     20,
		OnNext:        20,
	}
	AssertDeltas(t, result, idle)
}
