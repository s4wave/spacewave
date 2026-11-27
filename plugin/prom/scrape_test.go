package prom_plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	io_prometheus_client "github.com/aperturerobotics/go-prometheus-client-lite/client_model/go"
	"github.com/aperturerobotics/go-prometheus-client-lite/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
	"github.com/sirupsen/logrus"
)

// TestScrapeRunThrough scrapes a live exporter into a target, commits, and
// reads the result back through the status and the PromQL service: a good
// scrape, a failed one that marks its series stale, a reopened store that
// resumes the history, and a device target that gathers a registry.
func TestScrapeRunThrough(t *testing.T) {
	// Share the context and logger.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Serve an exporter that can be made to fail. Its job label clashes
	// with the target's label.
	var failing atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# TYPE node_load1 gauge\nnode_load1{job=\"inner\"} 0.5\nnode_disk_bytes{dev=\"sda\"} 1024\n"))
	}))
	defer srv.Close()

	// Create the target and its query service on a real World engine.
	wtb := world_testbed.MustDefault(t, ctx)
	eng := wtb.Engine
	key := "prom/target/node"
	cfg := &s4wave_prom.ScrapeConfig{Url: srv.URL}

	// Create the target.
	store, err := CreateStore(ctx, eng, key, labels.FromStrings("job", "node", "instance", "forge"))
	if err != nil {
		t.Fatal(err)
	}

	// Serve PromQL over it, with helpers to query and commit.
	svc := &queryService{engine: newQueryEngine(), q: NewQueryable(eng, worldTargets{eng: eng})}
	query := func(expr string, at time.Time) *s4wave_prom.QueryResponse {
		t.Helper()
		resp, err := svc.Query(ctx, &s4wave_prom.QueryRequest{Expr: expr, TimeMs: at.UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	commit := func(stores ...*Store) {
		t.Helper()
		if _, err := CommitStores(ctx, eng, stores...); err != nil {
			t.Fatal(err)
		}
	}

	// A good scrape stores the page, the scrape series and an up status.
	t0 := time.Now().Add(-time.Minute).Truncate(time.Second)
	sc := newScraper(le, store, cfg, "peer-a", time.Minute, httpSource(srv.URL))
	sc.scrape(ctx, t0)
	commit(store)
	_, status := readTarget(ctx, t, eng, key)
	if !status.GetUp() || status.GetSeries() != 2 || status.GetDevicePeerId() != "peer-a" || status.GetLastScrapeMs() != t0.UnixMilli() {
		t.Fatalf("status after good scrape: %v", status)
	}
	if v := singleValue(t, query(`up{job="node",instance="forge"}`, t0)); v != 1 {
		t.Fatalf("up = %v", v)
	}
	if v := singleValue(t, query(`node_load1{exported_job="inner"}`, t0)); v != 0.5 {
		t.Fatalf("node_load1 = %v", v)
	}

	// A failed scrape records the error, sets up to 0, and ends the series.
	failing.Store(true)
	t1 := t0.Add(15 * time.Second)
	sc.scrape(ctx, t1)
	commit(store)
	_, status = readTarget(ctx, t, eng, key)
	if status.GetUp() || status.GetLastError() == "" {
		t.Fatalf("status after failed scrape: %v", status)
	}
	if v := singleValue(t, query("up", t1)); v != 0 {
		t.Fatalf("up after failure = %v", v)
	}
	if n := len(query("node_disk_bytes", t1).GetSeries()); n != 0 {
		t.Fatalf("stale series still answered: %d", n)
	}

	// A reopened store resumes the history after the exporter recovers.
	failing.Store(false)
	reopened, err := OpenStore(ctx, eng, key)
	if err != nil {
		t.Fatal(err)
	}

	// Scrape through it from another Device, which takes the target over.
	t2 := t1.Add(15 * time.Second)
	newScraper(le, reopened, cfg, "peer-b", time.Minute, httpSource(srv.URL)).scrape(ctx, t2)
	commit(reopened)
	meta, status := readTarget(ctx, t, eng, key)
	if status.GetDevicePeerId() != "peer-b" || status.Stale(meta, t2.Add(time.Minute)) || !status.Stale(meta, t2.Add(2*time.Minute)) {
		t.Fatalf("status after takeover: %v", status)
	}

	// The first Device stands down at its next commit, writing nothing.
	sc.scrape(ctx, t2.Add(time.Second))
	commit(store)
	select {
	case <-store.Lost():
	default:
		t.Fatal("first Device did not stand down")
	}
	if _, status = readTarget(ctx, t, eng, key); status.GetDevicePeerId() != "peer-b" {
		t.Fatalf("status after stand-down: %v", status)
	}

	// Read up over the whole run.
	resp, err := svc.Query(ctx, &s4wave_prom.QueryRequest{
		Expr:    "up",
		StartMs: t0.UnixMilli(),
		EndMs:   t2.UnixMilli(),
		StepMs:  15_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Expect up, down, up.
	var got []float64
	for _, smp := range resp.GetSeries()[0].GetSamples() {
		got = append(got, smp.GetValue())
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("up over the run = %v", got)
	}

	// A device target gathers a registry with the Go runtime collector and a
	// histogram, plus a summary, which only exporters still produce.
	reg := prometheus.NewRegistry()
	hist := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "op_seconds", Buckets: []float64{0.1, 1}})
	reg.MustRegister(prometheus.NewGoCollector(), hist)
	hist.Observe(0.5)
	hist.Observe(2)

	// Build the summary family by hand.
	summary := &io_prometheus_client.MetricFamily{
		Name: new("rpc_seconds"),
		Type: io_prometheus_client.MetricType_SUMMARY.Enum(),
		Metric: []*io_prometheus_client.Metric{{Summary: &io_prometheus_client.Summary{
			SampleCount: new(uint64(1)),
			SampleSum:   new(3.0),
			Quantile:    []*io_prometheus_client.Quantile{{Quantile: new(0.5), Value: new(3.0)}},
		}}},
	}

	// Create the device target and scrape the registry into it.
	device, err := CreateStore(ctx, eng, "prom/target/device", labels.FromStrings("job", "spacewave"))
	if err != nil {
		t.Fatal(err)
	}
	gather := func(context.Context) ([]*io_prometheus_client.MetricFamily, error) {
		families, err := reg.Gather()
		return append(families, summary), err
	}
	deviceCfg := &s4wave_prom.ScrapeConfig{Url: s4wave_prom.DeviceMetricsURL}
	newScraper(le, device, deviceCfg, "peer-a", time.Minute, gatherSource(gather)).scrape(ctx, t2)
	commit(device)

	// Its series answer as the text format would carry them.
	if v := singleValue(t, query(`go_goroutines{job="spacewave"}`, t2)); v < 1 {
		t.Fatalf("go_goroutines = %v", v)
	}
	for expr, want := range map[string]float64{
		`op_seconds_bucket{le="1"}`:    1,
		`op_seconds_bucket{le="+Inf"}`: 2,
		`op_seconds_count`:             2,
		`rpc_seconds{quantile="0.5"}`:  3,
		`rpc_seconds_sum`:              3,
		`up{job="spacewave"}`:          1,
	} {
		if v := singleValue(t, query(expr, t2)); v != want {
			t.Fatalf("%s = %v, want %v", expr, v, want)
		}
	}

	// Label values span both targets.
	jobs, err := svc.LabelValues(ctx, &s4wave_prom.LabelValuesRequest{Name: "job"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(jobs.GetValues(), []string{"node", "spacewave"}) {
		t.Fatalf("job values = %v", jobs.GetValues())
	}

	// Several selectors union their matches.
	names, err := svc.LabelValues(ctx, &s4wave_prom.LabelValuesRequest{
		Name:  "__name__",
		Match: []string{`node_load1`, `{__name__=~"op_seconds_.*",le!="+Inf"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"node_load1", "op_seconds_bucket", "op_seconds_count", "op_seconds_sum"}; !slices.Equal(names.GetValues(), want) {
		t.Fatalf("metric names = %v, want %v", names.GetValues(), want)
	}

	// Label names follow the selector, and a bad selector fails.
	labelNames, err := svc.LabelNames(ctx, &s4wave_prom.LabelNamesRequest{Match: []string{`node_load1`}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"__name__", "exported_job", "instance", "job"}; !slices.Equal(labelNames.GetValues(), want) {
		t.Fatalf("node_load1 label names = %v, want %v", labelNames.GetValues(), want)
	}
	if _, err := svc.LabelNames(ctx, &s4wave_prom.LabelNamesRequest{Match: []string{`up{`}}); err == nil {
		t.Fatal("bad selector accepted")
	}

	// A new Dashboard starts with the starter panels of the Space's jobs.
	dashKey := "prom/dashboard/hosts"
	err = world.ExecTransaction(ctx, eng, true, func(ctx context.Context, tx world.WorldState) error {
		jobs, err := s4wave_prom.TargetJobs(ctx, tx)
		if err != nil {
			return err
		}
		return s4wave_prom.CreateDashboard(ctx, tx, dashKey, s4wave_prom.StarterDashboard(jobs))
	})
	if err != nil {
		t.Fatal(err)
	}

	// Serve its edits, with a helper that reads its panel titles.
	dashSvc := &dashboardService{eng: eng, objectKey: dashKey}
	titles := func() []string {
		// Read the Dashboard.
		t.Helper()
		var dash *s4wave_prom.Dashboard
		err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, tx world.WorldState) error {
			var err error
			dash, err = s4wave_prom.ReadDashboard(ctx, tx, dashKey)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		// List its titles in order.
		var out []string
		for _, p := range dash.GetPanels() {
			out = append(out, p.GetTitle())
		}
		return out
	}
	want := []string{"CPU busy", "Memory used", "Filesystem used", "Network received", "Heap in use", "Goroutines"}
	if got := titles(); !slices.Equal(got, want) {
		t.Fatalf("starter panels = %v, want %v", got, want)
	}

	// Add, move and remove panels. Editing a removed panel fails.
	added, err := dashSvc.PutPanel(ctx, &s4wave_prom.PutPanelRequest{Panel: &s4wave_prom.Panel{Title: "Load", Expr: "node_load1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dashSvc.MovePanel(ctx, &s4wave_prom.MovePanelRequest{Id: added.GetId()}); err != nil {
		t.Fatal(err)
	}
	if got := titles(); got[0] != "Load" || len(got) != 7 {
		t.Fatalf("panels after move = %v", got)
	}
	if _, err := dashSvc.RemovePanel(ctx, &s4wave_prom.RemovePanelRequest{Id: added.GetId()}); err != nil {
		t.Fatal(err)
	}
	_, err = dashSvc.PutPanel(ctx, &s4wave_prom.PutPanelRequest{Panel: &s4wave_prom.Panel{Id: added.GetId(), Title: "Load"}})
	if !errors.Is(err, s4wave_prom.ErrPanelRemoved) {
		t.Fatalf("edit of removed panel: %v", err)
	}
	if got := titles(); !slices.Equal(got, want) {
		t.Fatalf("panels after remove = %v, want %v", got, want)
	}
}

// readTarget reads the target's meta and status in a read transaction.
func readTarget(ctx context.Context, t *testing.T, eng world.Engine, key string) (*s4wave_prom.TargetMeta, *s4wave_prom.TargetStatus) {
	// Read both in one transaction.
	t.Helper()
	var meta *s4wave_prom.TargetMeta
	var status *s4wave_prom.TargetStatus
	err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, tx world.WorldState) error {
		var err error
		meta, status, err = s4wave_prom.ReadTarget(ctx, tx, key)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return meta, status
}

// singleValue returns the value of a one-series instant result.
func singleValue(t *testing.T, resp *s4wave_prom.QueryResponse) float64 {
	t.Helper()
	if len(resp.GetSeries()) != 1 {
		t.Fatalf("want one series, got %v", resp.GetSeries())
	}
	return resp.GetSeries()[0].GetSamples()[0].GetValue()
}
