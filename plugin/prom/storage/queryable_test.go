package promstorage

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// testTargets are the target labels of the several-target fixtures.
var testTargets = []labels.Labels{
	labels.FromStrings("job", "api", "instance", "a:9100"),
	labels.FromStrings("job", "api", "instance", "b:9100"),
	labels.FromStrings("job", "db", "instance", "c:9100"),
}

// testSeries are the series every test target scrapes.
var testSeries = []labels.Labels{
	labels.FromStrings("__name__", "http_requests_total", "method", "GET"),
	labels.FromStrings("__name__", "http_requests_total", "method", "POST"),
	labels.FromStrings("__name__", "http_requests_total", "method", "PUT"),
	labels.FromStrings("__name__", "temperature", "room", "kitchen"),
}

// TestQueryableMatchesReference checks selector, rate, and aggregation
// queries over several targets against the in-memory reference.
func TestQueryableMatchesReference(t *testing.T) {
	// Scrape three targets for 40 minutes and commit in 5 windows of 8 minutes.
	ctx := t.Context()
	f := newFixture(t, testTargets...)
	const scrapes = 160
	for from := 0; from < scrapes; from += 32 {
		for _, key := range f.keys {
			f.appendSamples(key, testSeries, from, from+32)
		}
		stats := f.commit(ctx)
		if stats.Segments != len(f.keys) {
			t.Fatalf("segments = %d, want one per target", stats.Segments)
		}
	}
	end := testBaseTime + (scrapes-1)*testStep
	start := end - 30*60*1000
	queries := []string{
		`http_requests_total`,
		`http_requests_total{job="api"}`,
		`http_requests_total{method=~"GET|POST", job!="db"}`,
		`{__name__=~"temp.*", instance="b:9100"}`,
		`http_requests_total{method!~"P.*"}`,
		`rate(http_requests_total[1m])`,
		`rate(http_requests_total[5m])`,
		`increase(http_requests_total{method="POST"}[10m])`,
		`irate(http_requests_total[2m])`,
		`sum(rate(http_requests_total[2m]))`,
		`sum by (job) (rate(http_requests_total[2m]))`,
		`sum without (method) (http_requests_total)`,
		`avg(temperature)`,
		`max by (instance) (temperature)`,
		`count by (job) (http_requests_total)`,
		`topk(2, rate(http_requests_total[3m]))`,
		`avg_over_time(temperature[4m])`,
		`temperature > 20`,
		`sum(rate(http_requests_total[2m])) / count(temperature)`,
		`http_requests_total offset 5m`,
	}

	// Run each query as an instant and a range query against both storages.
	q := f.queryable()
	for _, qs := range queries {
		got := instantQuery(ctx, t, q, qs, end)
		want := instantQuery(ctx, t, f.ref, qs, end)
		requireSameResult(t, qs, got, want)
		if got.Value.String() == "" || got.Value.String() == "{}" {
			t.Fatalf("%s: empty result, the query tests nothing", qs)
		}
		gotRange := rangeQuery(ctx, t, q, qs, start, end, 30*time.Second)
		wantRange := rangeQuery(ctx, t, f.ref, qs, start, end, 30*time.Second)
		requireSameResult(t, qs+" (range)", gotRange, wantRange)
	}
}

// TestQueryableLabelMatching checks matcher types and label lookups.
func TestQueryableLabelMatching(t *testing.T) {
	// Commit one window for two targets.
	ctx := t.Context()
	f := newFixture(t, testTargets[0], testTargets[2])
	for _, key := range f.keys {
		f.appendSamples(key, testSeries, 0, 8)
	}
	f.commit(ctx)
	end := testBaseTime + 7*testStep
	q := f.queryable()

	// Count the series each selector returns.
	cases := []struct {
		selector string
		want     int
	}{
		{`http_requests_total`, 6},
		{`http_requests_total{job="api"}`, 3},
		{`http_requests_total{job="db", method="PUT"}`, 1},
		{`http_requests_total{job!="api"}`, 3},
		{`http_requests_total{method=~"GET|PUT"}`, 4},
		{`http_requests_total{method!~"GET|PUT"}`, 2},
		{`{room="kitchen"}`, 2},
		{`{__name__=~".+", job="db"}`, 4},
		{`http_requests_total{room=""}`, 6},
		{`http_requests_total{room!=""}`, 0},
		{`http_requests_total{instance="nope"}`, 0},
		{`nothing`, 0},
	}
	for _, c := range cases {
		res := instantQuery(ctx, t, q, c.selector, end)
		if res.Err != nil {
			t.Fatalf("%s: %v", c.selector, res.Err)
		}
		vec, err := res.Vector()
		if err != nil {
			t.Fatalf("%s: %v", c.selector, err)
		}
		if len(vec) != c.want {
			t.Fatalf("%s: %d series, want %d", c.selector, len(vec), c.want)
		}
	}

	// Look up label names the way a panel editor does.
	querier, err := q.Querier(testBaseTime, end)
	if err != nil {
		t.Fatal(err)
	}
	defer querier.Close()
	names, _, err := querier.LabelNames(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"__name__", "instance", "job", "method", "room"}
	if !equalStrings(names, wantNames) {
		t.Fatalf("label names = %v, want %v", names, wantNames)
	}

	// Look up label values under a selector.
	methods, _, err := querier.LabelValues(ctx, "method", nil, labels.MustNewMatcher(labels.MatchEqual, "job", "db"))
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(methods, []string{"GET", "POST", "PUT"}) {
		t.Fatalf("method values = %v", methods)
	}
}

// TestQueryableMissingTarget checks that a listed target without an object is
// skipped instead of failing the query.
func TestQueryableMissingTarget(t *testing.T) {
	// Commit one target and list it beside a target that does not exist.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	f.appendSamples(f.keys[0], testSeries, 0, 4)
	f.commit(ctx)
	q := NewQueryable(f.eng, StaticTargets{"prom/target/missing", f.keys[0]})

	// The query answers from the target that exists.
	res := instantQuery(ctx, t, q, `temperature`, testBaseTime+3*testStep)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	vec, _ := res.Vector()
	if len(vec) != 1 {
		t.Fatalf("got %d series, want 1", len(vec))
	}
}

// equalStrings reports whether a and b hold the same strings in order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newTestEngine returns a PromQL engine with Prometheus defaults.
func newTestEngine() *promql.Engine {
	return promql.NewEngine(promql.EngineOpts{
		MaxSamples:    10_000_000,
		Timeout:       30 * time.Second,
		LookbackDelta: 5 * time.Minute,
	})
}

// instantQuery runs qs at time ts, in milliseconds, against q.
func instantQuery(ctx context.Context, t testing.TB, q storage.Queryable, qs string, ts int64) *promql.Result {
	t.Helper()
	query, err := newTestEngine().NewInstantQuery(ctx, q, nil, qs, time.UnixMilli(ts))
	if err != nil {
		t.Fatalf("parse %q: %v", qs, err)
	}
	// Close would return the result's point slices to the engine's pool,
	// where a later query overwrites them, so the result stays unclosed.
	return query.Exec(ctx)
}

// rangeQuery runs qs from start to end, in milliseconds, at step against q.
func rangeQuery(ctx context.Context, t testing.TB, q storage.Queryable, qs string, start, end int64, step time.Duration) *promql.Result {
	t.Helper()
	query, err := newTestEngine().NewRangeQuery(ctx, q, nil, qs, time.UnixMilli(start), time.UnixMilli(end), step)
	if err != nil {
		t.Fatalf("parse %q: %v", qs, err)
	}
	// Close would return the result's point slices to the engine's pool,
	// where a later query overwrites them, so the result stays unclosed.
	return query.Exec(ctx)
}

// requireSameResult fails t unless got and want hold the same value.
func requireSameResult(t *testing.T, qs string, got, want *promql.Result) {
	// Fail on an error from either side.
	t.Helper()
	if got.Err != nil || want.Err != nil {
		t.Fatalf("%s: got error %v, reference error %v", qs, got.Err, want.Err)
	}

	// Compare the printed values line by line.
	gotLines := strings.Split(got.Value.String(), "\n")
	wantLines := strings.Split(want.Value.String(), "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("%s: line %d differs from reference (%d vs %d lines)\n got: %s\nwant: %s", qs, i, len(gotLines), len(wantLines), g, w)
		}
	}
}

// referenceQueryable is the in-memory reference: a storage.Queryable over
// samples held in Go slices, built with the fork's own list series.
type referenceQueryable struct {
	// series holds the samples by encoded label set.
	series map[string]*referenceSeries
}

// referenceSeries is one series of the reference.
type referenceSeries struct {
	// lset is the series' full label set.
	lset labels.Labels
	// samples are the series' samples in time order.
	samples []chunks.Sample
}

// newReferenceQueryable constructs an empty reference.
func newReferenceQueryable() *referenceQueryable {
	return &referenceQueryable{series: make(map[string]*referenceSeries)}
}

// add records a sample of the series lset.
func (r *referenceQueryable) add(lset labels.Labels, t int64, v float64) {
	key := string(lset.Bytes(nil))
	s := r.series[key]
	if s == nil {
		s = &referenceSeries{lset: lset}
		r.series[key] = s
	}
	s.samples = append(s.samples, referenceSample{t: t, f: v})
}

// Querier returns a querier over all recorded samples. The engine and the
// series iterators apply the time range.
func (r *referenceQueryable) Querier(int64, int64) (storage.Querier, error) {
	return r, nil
}

// Select returns the series matching matchers, sorted by labels.
func (r *referenceQueryable) Select(_ context.Context, _ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	var out []storage.Series
	for _, s := range r.series {
		match := true
		for _, m := range matchers {
			if !m.Matches(s.lset.Get(m.Name)) {
				match = false
				break
			}
		}
		if match {
			out = append(out, storage.NewListSeries(s.lset, s.samples))
		}
	}
	slices.SortFunc(out, func(a, b storage.Series) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	return NewSeriesSet(out)
}

// LabelValues is unused by the reference comparison.
func (r *referenceQueryable) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames is unused by the reference comparison.
func (r *referenceQueryable) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close releases nothing.
func (r *referenceQueryable) Close() error { return nil }

// _ is a type assertion
var (
	_ storage.Queryable = (*referenceQueryable)(nil)
	_ storage.Querier   = (*referenceQueryable)(nil)
)
