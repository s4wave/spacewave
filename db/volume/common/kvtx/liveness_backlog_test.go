package kvtx

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
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
// and the longest and 99th percentile commit. It runs only when
// SPACEWAVE_BACKLOG is set, since it takes about a minute of wall time.
func TestLivenessBacklog(t *testing.T) {
	if os.Getenv("SPACEWAVE_BACKLOG") == "" {
		t.Skip("set SPACEWAVE_BACKLOG to measure the collector backlog")
	}

	// Open a Volume over SQLite on disk and the real clock, whose collector
	// nets each record at once, and store the fixed children and the first
	// path.
	ctx := t.Context()
	store, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "volume.db"), "kv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := &store_kvtx.Config{CollectorFrees: true}
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
	c := v.newCollector(collectorStore{v: v}, true, time.Nanosecond)
	var freed atomic.Uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			res, err := c.Collect(ctx)
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
	issued := 0
	for _, rate := range []int{25, 50, 100, 200, 400, 800, 1600} {
		var waits []time.Duration
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
			for _, name := range []string{"leaf ", "A", "B", "C"} {
				delete(tr, name+strconv.Itoa(issued-1))
			}
			prev = next
		}
		took := time.Since(start)
		backlog := float64(4*uint64(issued)-freed.Load()) / 4 //nolint:gosec // a test size.
		drain := time.Now()
		for freed.Load() < 4*uint64(issued) { //nolint:gosec // a test size.
			if time.Since(drain) > 30*time.Second {
				t.Fatalf("collector did not drain %d commits in 30s", issued)
			}
			time.Sleep(time.Millisecond)
		}
		slices.Sort(waits)
		t.Logf("rate %d/s: %d commits, %.0f/s achieved, backlog %.1f commits at the end, drained in %v, longest commit %v, p99 %v",
			rate, len(waits), float64(len(waits))/took.Seconds(), backlog, time.Since(drain).Round(time.Millisecond),
			waits[len(waits)-1].Round(time.Microsecond), waits[len(waits)*99/100].Round(time.Microsecond))
	}
}
