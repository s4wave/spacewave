package prom_plugin

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// TestStoreCommitAndReopen checks that committed samples survive a reopen and
// that the reopened store continues the same target.
func TestStoreCommitAndReopen(t *testing.T) {
	// Commit two windows.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	key := f.keys[0]
	f.appendSamples(key, testSeries, 0, 8)
	f.commit(ctx)
	f.appendSamples(key, testSeries, 8, 16)
	f.commit(ctx)

	// Open the target again as a new writer.
	reopened, err := OpenStore(ctx, f.eng, key)
	if err != nil {
		t.Fatal(err)
	}

	// The reopened store rejects samples at or before the committed time.
	err = reopened.Append(testSeries[0], testBaseTime+15*testStep, 1)
	if !errors.Is(err, storage.ErrOutOfOrderSample) {
		t.Fatalf("append at committed time = %v, want out of order", err)
	}

	// It continues the target with the known series and a new one.
	f.stores[key] = reopened
	f.appendSamples(key, testSeries, 16, 24)
	extra := labels.FromStrings("__name__", "uptime_seconds")
	if err := reopened.Append(extra, testBaseTime+23*testStep, 7); err != nil {
		t.Fatal(err)
	}
	f.commit(ctx)

	// Every sample reads back, as the reference holds it.
	end := testBaseTime + 23*testStep
	q := f.queryable()
	got := instantQuery(ctx, t, q, `http_requests_total`, end)
	want := instantQuery(ctx, t, f.ref, `http_requests_total`, end)
	requireSameResult(t, "http_requests_total", got, want)
	gotRange := rangeQuery(ctx, t, q, `http_requests_total`, testBaseTime, end, time.Minute)
	wantRange := rangeQuery(ctx, t, f.ref, `http_requests_total`, testBaseTime, end, time.Minute)
	requireSameResult(t, "http_requests_total (range)", gotRange, wantRange)

	// The series added after the reopen is queryable with the target labels.
	up := instantQuery(ctx, t, q, `uptime_seconds`, end)
	vec, err := up.Vector()
	if err != nil || len(vec) != 1 || vec[0].F != 7 {
		t.Fatalf("uptime_seconds = %v, %v", vec, err)
	}
	if vec[0].Metric.Get("instance") != "a:9100" {
		t.Fatalf("uptime_seconds lacks the target labels: %s", vec[0].Metric)
	}
}

// TestStoreInstantAndRangeValues checks PromQL answers against values known
// from the sample generator, not from the reference.
func TestStoreInstantAndRangeValues(t *testing.T) {
	// One counter that grows by 3 every 15 seconds.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	f.appendSamples(f.keys[0], testSeries[:1], 0, 40)
	f.commit(ctx)
	q := f.queryable()

	// An instant query returns the latest sample inside the lookback.
	at := testBaseTime + 39*testStep
	vec, err := instantQuery(ctx, t, q, `http_requests_total`, at).Vector()
	if err != nil || len(vec) != 1 || vec[0].F != testValue(0, 39) {
		t.Fatalf("instant = %v, %v", vec, err)
	}

	// rate over a window of a steady counter is the per-second growth.
	vec, err = instantQuery(ctx, t, q, `rate(http_requests_total[2m])`, at).Vector()
	if err != nil || len(vec) != 1 || math.Abs(vec[0].F-0.2) > 1e-9 {
		t.Fatalf("rate = %v, %v", vec, err)
	}

	// A range query returns one point per step.
	res := rangeQuery(ctx, t, q, `http_requests_total`, testBaseTime+30*testStep, at, 15*time.Second)
	mat, err := res.Matrix()
	if err != nil || len(mat) != 1 || len(mat[0].Floats) != 10 {
		t.Fatalf("range = %v, %v", mat, err)
	}
	for i, p := range mat[0].Floats {
		if p.F != testValue(0, 30+i) {
			t.Fatalf("point %d = %v, want %v", i, p.F, testValue(0, 30+i))
		}
	}
}

// TestStoreStaleness checks that a stale marker ends a series for PromQL
// while the earlier samples stay readable.
func TestStoreStaleness(t *testing.T) {
	// Scrape 10 times, then mark the series stale one step later.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	key := f.keys[0]
	f.appendSamples(key, testSeries[:1], 0, 10)
	staleAt := testBaseTime + 10*testStep
	if err := f.stores[key].AppendStale(mergeLabels(labels.EmptyLabels(), testSeries[0]), staleAt); err != nil {
		t.Fatal(err)
	}
	f.commit(ctx)
	q := f.queryable()

	// Before the marker the series is present. At and after it, the series
	// is absent.
	vec, err := instantQuery(ctx, t, q, `http_requests_total`, staleAt-1).Vector()
	if err != nil || len(vec) != 1 {
		t.Fatalf("before marker = %v, %v", vec, err)
	}
	vec, err = instantQuery(ctx, t, q, `http_requests_total`, staleAt+testStep).Vector()
	if err != nil || len(vec) != 0 {
		t.Fatalf("after marker = %v, %v, want no series", vec, err)
	}

	// Select the series straight from the querier.
	querier, err := q.Querier(testBaseTime, staleAt)
	if err != nil {
		t.Fatal(err)
	}
	defer querier.Close()
	set := querier.Select(ctx, true, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total"))
	if !set.Next() {
		t.Fatal("series not found")
	}

	// The marker is stored as a stale NaN, not dropped.
	it := set.At().Iterator(nil)
	var lastT int64
	var lastV float64
	for it.Next() != chunkenc.ValNone {
		lastT, lastV = it.At()
	}
	if lastT != staleAt || !value.IsStaleNaN(lastV) {
		t.Fatalf("last sample = %d %v, want stale marker at %d", lastT, lastV, staleAt)
	}
}

// TestStoreRollback checks that a cancelled window leaves no trace: nothing is
// written, its series are forgotten, and the store appends again.
func TestStoreRollback(t *testing.T) {
	// Commit a window.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	key := f.keys[0]
	st := f.stores[key]
	f.appendSamples(key, testSeries[:2], 0, 4)
	f.commit(ctx)

	// Append a second window with a new series, then cancel it.
	ghost := labels.FromStrings("__name__", "ghost")
	for k := int64(4); k < 8; k++ {
		if err := st.Append(testSeries[0], testBaseTime+k*testStep, 1); err != nil {
			t.Fatal(err)
		}
		if err := st.Append(ghost, testBaseTime+k*testStep, 1); err != nil {
			t.Fatal(err)
		}
	}

	// The uncommitted window is invisible to readers.
	q := f.queryable()
	end := testBaseTime + 7*testStep
	vec, _ := instantQuery(ctx, t, q, `ghost`, end).Vector()
	if len(vec) != 0 {
		t.Fatalf("uncommitted series visible: %v", vec)
	}
	if err := st.Rollback(); err != nil {
		t.Fatal(err)
	}

	// Committing after the rollback writes nothing.
	stats := f.commit(ctx)
	if stats.Segments != 0 || stats.Bytes != 0 {
		t.Fatalf("commit after rollback wrote %+v", stats)
	}
	vec, _ = instantQuery(ctx, t, q, `ghost`, end).Vector()
	if len(vec) != 0 {
		t.Fatalf("rolled back series visible: %v", vec)
	}

	// The same times are free again, so a new series can use them.
	other := labels.FromStrings("__name__", "other")
	if err := st.Append(other, testBaseTime+4*testStep, 5); err != nil {
		t.Fatal(err)
	}
	stats = f.commit(ctx)
	if stats.Segments != 1 || stats.Samples != 1 {
		t.Fatalf("commit wrote %+v, want one segment of one sample", stats)
	}

	// The new series reads back, and the table holds it in the freed number.
	vec, err := instantQuery(ctx, t, q, `other`, end).Vector()
	if err != nil || len(vec) != 1 || vec[0].F != 5 {
		t.Fatalf("other = %v, %v", vec, err)
	}
	reopened, err := OpenStore(ctx, f.eng, key)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.table.Len() != 3 {
		t.Fatalf("series table holds %d series, want 3", reopened.table.Len())
	}
}

// TestStoreCommitRetry checks that a commit that fails keeps its window, and
// the next commit writes it once.
func TestStoreCommitRetry(t *testing.T) {
	// Append a window to the store.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	f.appendSamples(f.keys[0], testSeries, 0, 8)

	// A cancelled context fails the commit.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	stores := []*Store{f.stores[f.keys[0]]}
	if _, err := CommitStores(cancelled, f.eng, stores...); err == nil {
		t.Fatal("commit with a cancelled context succeeded")
	}

	// The failed commit left nothing in the World.
	vec, _ := instantQuery(ctx, t, f.queryable(), `temperature`, testBaseTime+7*testStep).Vector()
	if len(vec) != 0 {
		t.Fatalf("failed commit left samples: %v", vec)
	}

	// The retry writes the window, and a second commit has nothing to write.
	stats := f.commit(ctx)
	if stats.Samples != 8*len(testSeries) {
		t.Fatalf("retry wrote %d samples, want %d", stats.Samples, 8*len(testSeries))
	}
	if stats := f.commit(ctx); stats.Segments != 0 {
		t.Fatalf("second commit wrote %+v", stats)
	}

	// The samples match the reference exactly once.
	got := instantQuery(ctx, t, f.queryable(), `temperature`, testBaseTime+7*testStep)
	want := instantQuery(ctx, t, f.ref, `temperature`, testBaseTime+7*testStep)
	requireSameResult(t, "temperature", got, want)
}

// TestStoreAppendErrors checks the append rules that match Prometheus.
func TestStoreAppendErrors(t *testing.T) {
	// Append a first sample.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	st := f.stores[f.keys[0]]
	lset := testSeries[0]
	if err := st.Append(lset, testBaseTime, 1); err != nil {
		t.Fatal(err)
	}

	// Repeating a sample is ignored. A different value at the same time
	// fails.
	if err := st.Append(lset, testBaseTime, 1); err != nil {
		t.Fatalf("repeat = %v, want nil", err)
	}
	if err := st.Append(lset, testBaseTime, 2); !errors.Is(err, storage.ErrDuplicateSampleForTimestamp) {
		t.Fatalf("duplicate = %v", err)
	}

	// An earlier timestamp than the series' latest fails.
	if err := st.Append(lset, testBaseTime+testStep, 2); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(lset, testBaseTime, 3); !errors.Is(err, storage.ErrOutOfOrderSample) {
		t.Fatalf("out of order = %v", err)
	}

	// A series may not carry a label the target provides.
	bad := labels.FromStrings("__name__", "x", "job", "other")
	if err := st.Append(bad, testBaseTime+2*testStep, 1); !errors.Is(err, ErrReservedLabel) {
		t.Fatalf("reserved label = %v", err)
	}

	// After a commit, a new window may not reach back into committed time.
	f.commit(ctx)
	if err := st.Append(lset, testBaseTime+testStep, 9); !errors.Is(err, storage.ErrOutOfOrderSample) {
		t.Fatalf("append into committed time = %v", err)
	}
	if err := st.Append(testSeries[1], testBaseTime, 9); !errors.Is(err, storage.ErrOutOfOrderSample) {
		t.Fatalf("new series into committed time = %v", err)
	}
}

// TestStoreBytesPerCommitStayFlat checks that the bytes one commit writes
// follow the window, not the history already stored.
func TestStoreBytesPerCommitStayFlat(t *testing.T) {
	// Commit 150 equal windows of 4 samples for each of 4 series.
	ctx := t.Context()
	f := newFixture(t, testTargets[0])
	key := f.keys[0]
	const windows, perWindow = 150, 4
	bytes := make([]uint64, windows)
	for w := range windows {
		f.appendSamples(key, testSeries, w*perWindow, (w+1)*perWindow)
		stats := f.commit(ctx)
		if stats.Segments != 1 || stats.Samples != perWindow*len(testSeries) {
			t.Fatalf("window %d wrote %+v", w, stats)
		}
		bytes[w] = stats.Bytes
	}

	// Compare early commits, after the series table page, to the latest ones.
	early, late := meanBytes(bytes[10:30]), meanBytes(bytes[windows-20:])
	t.Logf("mean bytes per commit: windows 10-29 = %.0f, last 20 = %.0f", early, late)
	if late > early*1.5 {
		t.Fatalf("bytes per commit grew from %.0f to %.0f as history grew", early, late)
	}
}

// meanBytes returns the mean of v.
func meanBytes(v []uint64) float64 {
	var sum float64
	for _, b := range v {
		sum += float64(b)
	}
	return sum / float64(len(v))
}

// TestStoreCompactionAndRetention scrapes seven hours into a target that keeps
// three, committing once a minute. Each closed two-hour bucket ends as one
// segment, segments past the retention and its bucket are gone, and queries
// over the kept range match the reference.
func TestStoreCompactionAndRetention(t *testing.T) {
	// Create a target that keeps three hours.
	ctx := t.Context()
	f := newFixture(t)
	key := "prom/target/kept"
	retention := 3 * time.Hour
	meta := &s4wave_prom.TargetMeta{Labels: NewLabelSet(testTargets[0]), RetentionMs: uint64(retention.Milliseconds())}

	// Write it as the CLI does, then open its writer.
	err := world.ExecTransaction(ctx, f.eng, true, func(ctx context.Context, wtx world.WorldState) error {
		return s4wave_prom.CreateTarget(ctx, wtx, key, meta)
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(ctx, f.eng, key)
	if err != nil {
		t.Fatal(err)
	}
	f.stores[key], f.keys = st, []string{key}

	// Scrape every 15 seconds for seven hours, committing every minute.
	const scrapes = 7 * 60 * 4
	for from := 0; from < scrapes; from += 4 {
		f.appendSamples(key, testSeries, from, from+4)
		f.commit(ctx)
	}
	end := testBaseTime + (scrapes-1)*testStep

	// Read the segments' ranges.
	type span struct{ minTime, maxTime int64 }
	var spans []span
	err = f.queryable().forEachTarget(ctx, func(r *StoreReader) error {
		return r.ForEachSegment(ctx, math.MinInt64, math.MaxInt64, func(seg *Segment) error {
			spans = append(spans, span{seg.GetMinTime(), seg.GetMaxTime()})
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	// No kept segment ends a bucket before the retention, and each closed
	// bucket has one segment.
	open := compactBucket(end)
	perBucket := make(map[int64]int)
	for _, s := range spans {
		if s.maxTime < end-(retention+CompactSpan).Milliseconds() {
			t.Fatalf("segment %v outlived the retention", s)
		}
		perBucket[compactBucket(s.minTime)]++
	}
	for bucket, n := range perBucket {
		if bucket < open && n != 1 {
			t.Fatalf("closed bucket %d holds %d segments", bucket, n)
		}
	}
	if perBucket[open] < 2 || len(perBucket) != 3 {
		t.Fatalf("segments per bucket = %v, want merged closed buckets and an open run", perBucket)
	}

	// The kept range answers as the reference does. The dropped range is
	// empty.
	q := f.queryable()
	start := spans[0].minTime + 10*time.Minute.Milliseconds()
	for _, qs := range []string{`http_requests_total`, `rate(http_requests_total[5m])`, `max_over_time(temperature[10m])`} {
		got := rangeQuery(ctx, t, q, qs, start, end, time.Minute)
		want := rangeQuery(ctx, t, f.ref, qs, start, end, time.Minute)
		requireSameResult(t, qs, got, want)
	}
	if vec, err := instantQuery(ctx, t, q, `http_requests_total`, testBaseTime+time.Hour.Milliseconds()).Vector(); err != nil || len(vec) != 0 {
		t.Fatalf("expired range answered %v, %v", vec, err)
	}
}
