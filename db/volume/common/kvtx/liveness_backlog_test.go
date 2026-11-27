package kvtx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// backlogFanout is the fan-out of each page a backlog commit rewrites, near
// the largest World child list.
const backlogFanout = 180

// TestLivenessBacklog measures whether the collector keeps up with commits
// at a sustained rate. Each commit rewrites a path of three pages of
// backlogFanout children, each sharing all but one child with the page it
// replaces, and moves the root onto it, so the collector nets three new pages
// and frees three old pages and one old leaf per commit. The rate steps up;
// each step logs the commits issued, the backlog of frees left at its end in
// commits, the time the collector took to drain it with the writer stopped,
// the longest and 99th percentile commit, and how much of the longest commit
// a collector transaction overlapped. It runs only when SPACEWAVE_BACKLOG is
// set, to 1 for every rate or to a comma-separated list of rates.
func TestLivenessBacklog(t *testing.T) {
	if os.Getenv("SPACEWAVE_BACKLOG") == "" {
		t.Skip("set SPACEWAVE_BACKLOG to measure the collector backlog")
	}

	// SPACEWAVE_BACKLOG_FREES=0 runs the collector with frees off, as
	// deployed builds do, to measure the holds of the leases and the net
	// alone.
	frees := os.Getenv("SPACEWAVE_BACKLOG_FREES") != "0"

	// Open a Volume over SQLite on disk and the real clock, whose collector
	// nets each record at once, and store the fixed children and the first
	// path.
	ctx := t.Context()
	store, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "volume.db"), "kv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := &store_kvtx.Config{CollectorFrees: frees}
	v, err := NewVolume(ctx, "test/backlog", store_kvkey.NewDefaultKVKey(), store, cfg, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	v.clock = time.Now
	tr := testTree{}
	fixed := make([]string, backlogFanout-1)
	for i := range fixed {
		fixed[i] = "fixed " + strconv.Itoa(i)
		tr.add(t, fixed[i])
	}
	path := func(i int) (*block.BlockRef, []string) {
		n := strconv.Itoa(i)
		names := []string{"leaf " + n, "A" + n, "B" + n, "C" + n}
		tr.add(t, names[0])
		tr.add(t, names[1], append(slices.Clone(fixed), names[0])...)
		tr.add(t, names[2], append(slices.Clone(fixed), names[1])...)
		return tr.add(t, names[3], append(slices.Clone(fixed), names[2])...), names
	}
	prev, names := path(0)
	if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(fixed), names...)...), []block.NamedRoot{{Name: "main", Ref: prev}}); err != nil {
		t.Fatal(err)
	}

	// Run the collector beside the writer, as the Volume's own does, yielding
	// to a waiting writer.
	spans := &spanStore{Store: collectorStore{v: v}}
	c := block_gc.NewCollector(spans, block_gc.CollectorOptions{
		Frees: frees,
		Grace: time.Nanosecond,
		Now:   v.now,
		Phase: func(p block_gc.Phase) func() {
			spans.phase.Store(int32(p) + 1)
			return func() { spans.phase.Store(0) }
		},
		WritersWaiting: func() bool {
			q, ok := v.kvtxStore.(kvtx.WriterQueueStore)
			return ok && q.WritersWaiting()
		},
	})
	var freed atomic.Uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			res, err := c.Collect(backgroundPass(ctx))
			if err != nil {
				if ctx.Err() == nil {
					t.Error(err)
				}
				return
			}
			freed.Add(res.Freed)
			if !res.More {
				time.Sleep(200 * time.Microsecond)
			}
		}
	}()
	t.Cleanup(func() { <-done })

	// Step the rate, commit on schedule, then drain.
	rates := []int{25, 50, 100, 200, 400, 800, 1600}
	if env := os.Getenv("SPACEWAVE_BACKLOG"); env != "1" {
		rates = rates[:0]
		for _, f := range strings.Split(env, ",") {
			rate, err := strconv.Atoi(f)
			if err != nil {
				t.Fatal(err)
			}
			rates = append(rates, rate)
		}
	}
	issued := 0
	for _, rate := range rates {
		var waits []time.Duration
		var slow span
		start, step := time.Now(), 2*time.Second
		for k := 0; time.Since(start) < step; k++ {
			if d := time.Until(start.Add(time.Duration(k) * time.Second / time.Duration(rate))); d > 0 {
				time.Sleep(d)
			}
			issued++
			next, names := path(issued)
			began := time.Now()
			if err := v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: "main", Ref: next, Previous: prev}}); err != nil {
				t.Fatal(err)
			}
			waits = append(waits, time.Since(began))
			if w := waits[len(waits)-1]; w > slow.end.Sub(slow.start) {
				slow = span{start: began, end: began.Add(w)}
			}
			for _, name := range []string{"leaf ", "A", "B", "C"} {
				delete(tr, name+strconv.Itoa(issued-1))
			}
			prev = next
		}
		took := time.Since(start)
		backlog := float64(4*uint64(issued)-freed.Load()) / 4 //nolint:gosec // a test size.
		drain := time.Now()
		for frees && freed.Load() < 4*uint64(issued) { //nolint:gosec // a test size.
			if time.Since(drain) > 30*time.Second {
				t.Fatalf("collector did not drain %d commits in 30s", issued)
			}
			time.Sleep(time.Millisecond)
		}
		slices.Sort(waits)
		over, longest, held, by := spans.overlap(slow)
		t.Logf("rate %d/s: %d commits, %.0f/s achieved, backlog %.1f commits at the end, drained in %v, longest commit %v at %s, p99 %v; collector transactions overlapped it %v, the longest of them %v; longest collector transaction %v, in %s",
			rate, len(waits), float64(len(waits))/took.Seconds(), backlog, time.Since(drain).Round(time.Millisecond),
			waits[len(waits)-1].Round(time.Microsecond), slow.start.Format("15:04:05.000"), waits[len(waits)*99/100].Round(time.Microsecond),
			over.Round(time.Microsecond), longest.Round(time.Microsecond), held.Round(time.Microsecond), by)
	}
}

// span is an interval of wall time.
type span struct {
	// start and end bound the span.
	start, end time.Time
	// by names the collector functions that ran the transaction.
	by string
	// work is when the transaction's body returned; the rest is its commit.
	work time.Time
}

// spanStore runs a collector's transactions and records when each ran.
type spanStore struct {
	block_gc.Store
	// mtx guards spans.
	mtx sync.Mutex
	// spans are the transactions run, in order.
	spans []span
	// phase is the collector's phase plus one, zero outside the net and the
	// sweep.
	phase atomic.Int32
}

// Update runs the transaction and records its span.
func (s *spanStore) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	by := []string{[]string{"leases", "net", "sweep"}[s.phase.Load()]}
	pcs := make([]uintptr, 16)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for f, more := frames.Next(); more; f, more = frames.Next() {
		if name, ok := strings.CutPrefix(f.Function, "github.com/s4wave/spacewave/db/block/gc."); ok {
			by = append(by, name)
		}
	}
	asked := time.Now()
	var start, work time.Time
	err := s.Store.Update(ctx, func(rec *block_gc.Records, blocks block.StoreOps) error {
		start = time.Now()
		err := fn(rec, blocks)
		work = time.Now()
		return err
	})
	if start.IsZero() {
		start, work = asked, asked
	}
	by = append(by, fmt.Sprintf("waited %v for the turn", start.Sub(asked).Round(time.Microsecond)))
	s.mtx.Lock()
	s.spans = append(s.spans, span{start: start, end: time.Now(), by: strings.Join(by, " < "), work: work})
	s.mtx.Unlock()
	return err
}

// overlap returns how much of w the recorded transactions covered, the
// longest of those transactions, and the longest transaction recorded with
// the collector functions that ran it, and forgets them all.
func (s *spanStore) overlap(w span) (over, longest, held time.Duration, by string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, sp := range s.spans {
		d := sp.end.Sub(sp.start)
		if d > held {
			held, by = d, sp.by+fmt.Sprintf(", body %v, commit %v", sp.work.Sub(sp.start).Round(time.Microsecond), sp.end.Sub(sp.work).Round(time.Microsecond))
		}
		if lo, hi := maxTime(sp.start, w.start), minTime(sp.end, w.end); hi.After(lo) {
			over += hi.Sub(lo)
			longest = max(longest, d)
		}
	}
	s.spans = s.spans[:0]
	return over, longest, held, by
}

// maxTime returns the later of a and b.
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// minTime returns the earlier of a and b.
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
