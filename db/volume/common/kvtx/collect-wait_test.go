package kvtx

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// TestCommitWaitDuringCollection measures how long a writer's commit waits
// while the collector frees a large garbage tree on an s4db Volume, at two
// sizes, while it nets and frees one commit of 100,000 ups, and while it frees
// one block referencing 100,000 children, and logs the longest and the 99th
// percentile wait. It runs only when KVTX_MEASURE_COLLECT is set, on the build
// host.
func TestCommitWaitDuringCollection(t *testing.T) {
	if os.Getenv("KVTX_MEASURE_COLLECT") == "" {
		t.Skip("set KVTX_MEASURE_COLLECT to measure")
	}
	for _, leaves := range []int{10_000, 100_000} {
		t.Run(strconv.Itoa(leaves), func(t *testing.T) {
			measureCommitWait(t, leaves)
		})
	}
	t.Run("one commit of 100000 ups", func(t *testing.T) {
		measureLargeCommitWait(t, 100_000)
	})
	t.Run("one block of 100000 children", func(t *testing.T) {
		measureWideBlockWait(t, 100_000)
	})
	t.Run("checkpoint run-through", func(t *testing.T) {
		measureCheckpointRunThrough(t, 30*time.Second)
	})
	t.Run("obsolete sweep of 1000000 records", func(t *testing.T) {
		measureObsoleteSweepWait(t, 100_000, 1_000_000)
	})
}

// openWaitVolume opens an s4db Volume that frees, with the grace period
// grace.
func openWaitVolume(t *testing.T, grace string) *Volume {
	t.Helper()
	db, err := s4db.Open(filepath.Join(t.TempDir(), "wait.s4db"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	conf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: grace}
	v, err := NewVolume(t.Context(), "test/wait", store_kvkey.NewDefaultKVKey(), newOrderedSeekGuard(t, db), conf, false, false, nil, nil)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = v.Close()
		_ = db.Close()
	})
	return v
}

// storeLeaves stores leaves blocks named prefix and their index, a mid per
// 1000 under a root of its own, a batch per mid, and returns the mids' names.
func storeLeaves(t *testing.T, v *Volume, tr testTree, prefix string, leaves int) []string {
	t.Helper()
	var mids []string
	for m := 0; m*1000 < leaves; m++ {
		names := make([]string, 0, 1000)
		for i := m * 1000; i < min(leaves, (m+1)*1000); i++ {
			names = append(names, prefix+strconv.Itoa(i))
			tr.add(t, names[len(names)-1])
		}
		mid := "mid " + prefix + strconv.Itoa(m)
		tr.add(t, mid, names...)
		mids = append(mids, mid)
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries(append(names, mid)...), []block.NamedRoot{{Name: mid, Ref: tr[mid].Ref}}); err != nil {
			t.Fatal(err)
		}
	}
	return mids
}

// measureCommitWait builds main -> top -> mids -> leaves, releases main,
// collects to the end while a writer moves a root every millisecond, and
// logs the writer's waits.
func measureCommitWait(t *testing.T, leaves int) {
	// Store the tree under main.
	ctx := t.Context()
	v := openWaitVolume(t, "1ns")
	tr := testTree{}
	mids := storeLeaves(t, v, tr, "leaf ", leaves)
	top := tr.add(t, "top", mids...)
	moves := []block.NamedRoot{{Name: "main", Ref: top}}
	for _, mid := range mids {
		moves = append(moves, block.NamedRoot{Name: mid, Previous: tr[mid].Ref})
	}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("top"), moves); err != nil {
		t.Fatal(err)
	}

	// Release the tree and let its records age.
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: top}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)

	// Collect to the end while a writer commits.
	var freed uint64
	elapsed, waits := measureWaits(t, v, tr, func() {
		freed += collectToEnd(t, v)
	})
	t.Logf("leaves %d: freed %d in %v; %s", leaves, freed, elapsed, waits)
}

// measureLargeCommitWait stores leaves blocks under mids of 1000, then in one
// batch a parent referencing each mid's leaves and a top referencing the
// parents under a root, releasing the mids, so the batch's one commit logs an
// up for each leaf. It releases the root and logs a writer's waits while the
// collector passes over the young commit, then nets it and frees the tree a
// grace period later.
func measureLargeCommitWait(t *testing.T, leaves int) {
	// Store the leaves, then the parents and top in one batch.
	ctx := t.Context()
	v := openWaitVolume(t, "")
	clk := &skewClock{}
	v.clock = clk.Now
	tr := testTree{}
	mids := storeLeaves(t, v, tr, "leaf ", leaves)
	batch := make([]string, 0, len(mids)+1)
	moves := make([]block.NamedRoot, 0, len(mids)+1)
	for m, mid := range mids {
		names := make([]string, 0, 1000)
		for i := m * 1000; i < min(leaves, (m+1)*1000); i++ {
			names = append(names, "leaf "+strconv.Itoa(i))
		}
		parent := "parent " + strconv.Itoa(m)
		tr.add(t, parent, names...)
		batch = append(batch, parent)
		moves = append(moves, block.NamedRoot{Name: mid, Previous: tr[mid].Ref})
	}
	top := tr.add(t, "top", batch...)
	moves = append(moves, block.NamedRoot{Name: "top", Ref: top})
	if err := v.SetBucketRoots(ctx, "b", tr.entries(append(batch, "top")...), moves); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "top", Previous: top}}); err != nil {
		t.Fatal(err)
	}

	// Pass over the young commit, then net it and free the tree once it ages,
	// while a writer commits.
	var freed uint64
	elapsed, waits := measureWaits(t, v, tr, func() {
		collectToEnd(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		freed = collectToEnd(t, v)
	})
	t.Logf("one commit of %d ups: freed %d in %v; %s", leaves, freed, elapsed, waits)
}

// measureWideBlockWait stores leaves blocks under mids of 1000, then one block
// referencing them all under a root, releasing the mids, releases the root, and
// logs a writer's waits while the collector passes over the block's young
// record, then nets it and frees the block and its children a grace period
// later.
func measureWideBlockWait(t *testing.T, leaves int) {
	// Store the leaves, then the block referencing them all.
	ctx := t.Context()
	v := openWaitVolume(t, "")
	clk := &skewClock{}
	v.clock = clk.Now
	tr := testTree{}
	mids := storeLeaves(t, v, tr, "leaf ", leaves)
	names := make([]string, 0, leaves)
	for i := range leaves {
		names = append(names, "leaf "+strconv.Itoa(i))
	}
	wide := tr.add(t, "wide", names...)
	moves := []block.NamedRoot{{Name: "wide", Ref: wide}}
	for _, mid := range mids {
		moves = append(moves, block.NamedRoot{Name: mid, Previous: tr[mid].Ref})
	}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("wide"), moves); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "wide", Previous: wide}}); err != nil {
		t.Fatal(err)
	}

	// Pass over the young record, then net it and free the block once it ages,
	// while a writer commits.
	var freed uint64
	elapsed, waits := measureWaits(t, v, tr, func() {
		collectToEnd(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		freed = collectToEnd(t, v)
	})
	t.Logf("one block of %d children: freed %d in %v; %s", leaves, freed, elapsed, waits)
}

// measureCheckpointRunThrough commits World-shaped commits at 12 a second for
// d, each a parent referencing 9 new leaves, 250 stored leaves and the parent
// before it in its chain, so about 260 named keys, under the root main. A
// checkpoint every 32 commits, 0.375 a second, starts a new chain, which leaves
// the chain before it dead. A collector frees while the commits continue, with
// a 2 s grace period. It logs the commits' waits, the frees, and the up log's
// backlog, UpSeq - UpCursor, sampled each second, then collects to the end and
// audits.
func measureCheckpointRunThrough(t *testing.T, d time.Duration) {
	// Store the shared leaves under a root of their own.
	ctx := t.Context()
	v := openWaitVolume(t, "2s")
	tr := testTree{}
	var shared []string
	for i := range 250 {
		shared = append(shared, "shared "+strconv.Itoa(i))
		tr.add(t, shared[i])
	}
	tr.add(t, "base", shared...)
	if err := v.SetBucketRoots(ctx, "b", tr.entries(append(shared, "base")...), []block.NamedRoot{{Name: "base", Ref: tr["base"].Ref}}); err != nil {
		t.Fatal(err)
	}

	// Collect in the background until the commits stop.
	var freed atomic.Uint64
	stop := make(chan struct{})
	collected := make(chan struct{})
	go func() {
		defer close(collected)
		for {
			res, err := v.Collect(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			freed.Add(res.Freed)
			if res.More {
				continue
			}
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	// Commit at 12 a second, sampling the backlog each second.
	backlog := func() uint64 {
		var gap uint64
		err := v.readRecords(ctx, func(rec *block_gc.Records) error {
			state, err := rec.GetState(ctx)
			gap = state.GetUpSeq() - state.GetUpCursor()
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return gap
	}
	var waits []time.Duration
	var gaps []uint64
	var prev, link *block.BlockRef
	tick := time.NewTicker(time.Second / 12)
	defer tick.Stop()
	start := time.Now()
	for i := 0; time.Since(start) < d; i++ {
		<-tick.C
		if i%12 == 0 {
			gaps = append(gaps, backlog())
		}
		if i%32 == 0 {
			link = nil
		}
		names := make([]string, 0, 10)
		for j := range 9 {
			names = append(names, "leaf "+strconv.Itoa(i)+" "+strconv.Itoa(j))
			tr.add(t, names[j])
		}
		children := append(slices.Clone(names), shared...)
		parent := "parent " + strconv.Itoa(i)
		if link != nil {
			children = append(children, "parent "+strconv.Itoa(i-1))
		}
		link = tr.add(t, parent, children...)
		at := time.Now()
		err := v.SetBucketRoots(ctx, "b", tr.entries(append(names, parent)...), []block.NamedRoot{{Name: "main", Ref: link, Previous: prev}})
		waits = append(waits, time.Since(at))
		if err != nil {
			t.Fatal(err)
		}
		prev = link
	}
	elapsed := time.Since(start)
	during := freed.Load()
	close(stop)
	<-collected

	// Free what remains once it ages, and audit.
	time.Sleep(2*time.Second + 100*time.Millisecond)
	after := collectToEnd(t, v)
	slices.Sort(waits)
	t.Logf("%d commits in %v waited max %v, p99 %v, median %v; freed %d during the commits and %d after; backlog each second %v, at the end %d",
		len(waits), elapsed, waits[len(waits)-1], waits[len(waits)*99/100], waits[len(waits)/2], during, after, gaps, backlog())
	if during == 0 {
		t.Fatal("nothing freed while the commits continued")
	}
	requireAudit(t, v, 0)
}

// s4dbWaits summarizes the s4db store's longest wait on background work,
// its flushes and its runs per level.
func s4dbWaits(t *testing.T, v *Volume) string {
	t.Helper()
	st, err := v.kvtxStore.(*orderedSeekGuard).Store.(*s4db.DB).Stats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("background wait max %v, flushes %d, runs %v, memtable %d", st.BackgroundWaitMax, st.Flushes, st.Runs, st.MemtableBytes)
}

// measureObsoleteSweepWait stores leaves blocks under mids of 1000 and
// records obsolete records of an old graph kind, each keyed like a format 6
// edge between two blocks, then logs a writer's waits while the obsolete
// sweep deletes them, obsoleteSweepKeys per transaction, and the mean
// transaction's time.
func measureObsoleteSweepWait(t *testing.T, leaves, records int) {
	// Store the live tree, then the obsolete records, 10,000 per transaction.
	ctx := t.Context()
	v := openWaitVolume(t, "")
	tr := testTree{}
	storeLeaves(t, v, tr, "leaf ", leaves)
	prefix := append(bytes.Clone(livenessPrefix), 'g')
	for at := 0; at < records; at += 10_000 {
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			vtx, err := p.store.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			for i := at; i < min(records, at+10_000); i++ {
				from := sha256.Sum256([]byte("from " + strconv.Itoa(i)))
				to := sha256.Sum256([]byte("to " + strconv.Itoa(i)))
				if err := vtx.Set(ctx, slices.Concat(prefix, from[:], to[:]), nil); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Time the writer alone, then sweep them as the upgrade does, in the
	// background, while the writer commits.
	_, alone := measureWaits(t, v, tr, func() { time.Sleep(time.Second) })
	before := s4dbWaits(t, v)
	var err error
	elapsed, waits := measureWaits(t, v, tr, func() {
		err = v.sweepObsoletePrefix(block.WithBackground(ctx), prefix)
	})
	t.Logf("s4db before the sweep: %s; after: %s", before, s4dbWaits(t, v))
	if err != nil {
		t.Fatal(err)
	}
	if n := countPrefix(t, v.kvtxStore, prefix); n != 0 {
		t.Fatalf("%d obsolete records remain", n)
	}
	txns := (records + obsoleteSweepKeys - 1) / obsoleteSweepKeys
	t.Logf("obsolete sweep of %d records over %d leaves: %d transactions in %v, mean %v; %s; alone, %s",
		records, leaves, txns, elapsed, elapsed/time.Duration(txns), waits, alone)
}

// skewClock reads the wall clock moved forward by a skew, so a measurement
// ages records past the grace period while the collector's pace still times
// its slices on the wall clock.
type skewClock struct {
	// skew is how far the clock reads ahead of the wall clock.
	skew atomic.Int64
}

// Now returns the wall clock's time plus the skew.
func (c *skewClock) Now() time.Time {
	return time.Now().Add(time.Duration(c.skew.Load()))
}

// Add moves the clock forward by d.
func (c *skewClock) Add(d time.Duration) {
	c.skew.Add(int64(d))
}

// collectToEnd runs collection passes until one reports no more work, and
// returns the blocks freed.
func collectToEnd(t *testing.T, v *Volume) uint64 {
	t.Helper()
	var freed uint64
	for {
		res, err := v.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		freed += res.Freed
		if !res.More {
			return freed
		}
	}
}

// measureWaits runs fn while a writer moves a root every millisecond, and
// returns fn's duration and a summary of the writer's waits.
func measureWaits(t *testing.T, v *Volume, tr testTree, fn func()) (time.Duration, string) {
	// Commit a root move every millisecond until fn returns, then release the root.
	t.Helper()
	ctx := t.Context()
	var mtx sync.Mutex
	var waits []time.Duration
	done := make(chan struct{})
	writerDone := make(chan struct{})
	held := tr.add(t, "held")
	writes := testTree{}
	go func() {
		defer close(writerDone)
		if err := v.SetBucketRoots(ctx, "w", tr.entries("held"), []block.NamedRoot{{Name: "w", Ref: held}}); err != nil {
			t.Error(err)
			return
		}
		prev := held
		for i := 0; ; i++ {
			select {
			case <-done:
				// Release the root, so the next measurement starts clear.
				if err := v.SetBucketRoots(ctx, "w", nil, []block.NamedRoot{{Name: "w", Previous: prev}}); err != nil {
					t.Error(err)
				}
				return
			case <-time.After(time.Millisecond):
			}
			name := "w " + strconv.Itoa(i)
			next := writes.add(t, name)
			start := time.Now()
			err := v.SetBucketRoots(ctx, "w", writes.entries(name), []block.NamedRoot{{Name: "w", Ref: next, Previous: prev}})
			wait := time.Since(start)
			if err != nil {
				t.Error(err)
				return
			}
			prev = next
			mtx.Lock()
			waits = append(waits, wait)
			mtx.Unlock()
		}
	}()
	start := time.Now()
	fn()
	elapsed := time.Since(start)
	close(done)
	<-writerDone

	// Summarize the waits.
	mtx.Lock()
	defer mtx.Unlock()
	slices.Sort(waits)
	if len(waits) == 0 {
		t.Fatalf("no commit landed in %v", elapsed)
	}
	over, _ := slices.BinarySearch(waits, 5*time.Millisecond)
	return elapsed, fmt.Sprintf("%d commits waited max %v, p99 %v, median %v, %d over 5ms",
		len(waits), waits[len(waits)-1], waits[len(waits)*99/100], waits[len(waits)/2], len(waits)-over)
}
