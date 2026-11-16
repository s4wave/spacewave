package kvtx

import (
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
// sizes, while it nets and frees one journal record of 100,000 up pairs,
// while it frees one block naming 100,000 children, and while, under
// HoldUntilFree, a reopened collector reads a tail near its index bound and
// keeps every ripe block it names, and logs the longest and the 99th
// percentile wait. It runs only when KVTX_MEASURE_COLLECT is set, on
// the build host.
func TestCommitWaitDuringCollection(t *testing.T) {
	if os.Getenv("KVTX_MEASURE_COLLECT") == "" {
		t.Skip("set KVTX_MEASURE_COLLECT to measure")
	}
	for _, leaves := range []int{10_000, 100_000} {
		t.Run(strconv.Itoa(leaves), func(t *testing.T) {
			measureCommitWait(t, leaves)
		})
	}
	t.Run("one record of 100000 ups", func(t *testing.T) {
		measureLargeRecordWait(t, 100_000)
	})
	t.Run("one block of 100000 children", func(t *testing.T) {
		measureWideBlockWait(t, 100_000)
	})
	t.Run("a cold tail near the index bound", func(t *testing.T) {
		measureTailWait(t, 20_000, 500_000)
	})
}

// openWaitVolume opens an s4db Volume that frees, with the grace period
// grace.
func openWaitVolume(t *testing.T, grace string) *Volume {
	t.Helper()
	v, _ := openWaitVolumeAt(t, filepath.Join(t.TempDir(), "wait.s4db"), grace, false)
	return v
}

// openWaitVolumeAt opens the s4db Volume at path that frees, with the grace
// period grace, under HoldUntilFree when hold is set, and returns it with a
// function that closes it, which the test's cleanup also calls.
func openWaitVolumeAt(t *testing.T, path, grace string, hold bool) (*Volume, func()) {
	t.Helper()
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	conf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: grace, CollectorHoldUntilFree: hold}
	v, err := NewVolume(t.Context(), "test/wait", store_kvkey.NewDefaultKVKey(), db, conf, false, false, nil, nil)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			_ = v.Close()
			_ = db.Close()
		})
	}
	t.Cleanup(closeAll)
	return v, closeAll
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

// measureLargeRecordWait stores leaves blocks under mids of 1000, then in one
// batch a parent naming each mid's leaves and a top naming the parents under
// a root, releasing the mids, so the batch's one journal record holds an up
// pair for each leaf. It releases the root and logs a writer's waits while
// the collector passes over the young record, then nets it and frees the tree
// a grace period later.
func measureLargeRecordWait(t *testing.T, leaves int) {
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

	// Pass over the young record, then net it and free the tree once it ages,
	// while a writer commits.
	var freed uint64
	elapsed, waits := measureWaits(t, v, tr, func() {
		collectToEnd(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		freed = collectToEnd(t, v)
	})
	t.Logf("one record of %d ups: freed %d in %v; %s", leaves, freed, elapsed, waits)
}

// measureWideBlockWait stores leaves blocks under mids of 1000, then one
// block naming them all under a root, releasing the mids, releases the root,
// and logs a writer's waits while the collector passes over the block's young
// record, then nets it and frees the block and its children a grace period
// later.
func measureWideBlockWait(t *testing.T, leaves int) {
	// Store the leaves, then the block naming them all.
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

// measureTailWait measures a writer's waits behind the free's tail reads
// under HoldUntilFree. It stores ripe leaves, each released from a root of
// its own, and a block W listing as many other leaves, released too, then
// nets the releases with a collector that frees nothing, so those blocks
// ripen below the cursor. It then appends records of 2,048 again entries
// naming the leaves, W and the mids that held W's leaves, entries in all, as
// a writer that holds none of them. The Volume is closed and reopened, so the
// collector starts with no index and the store with cold caches, and the
// writer's waits are logged over two passes: the first reads the whole tail
// across slices and keeps every ripe block, the second keeps them again.
func measureTailWait(t *testing.T, leaves, entries int) {
	// Store the leaves under roots of their own, and W over other leaves.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "wait.s4db")
	v, closeVolume := openWaitVolumeAt(t, path, "", true)
	clk := &skewClock{}
	v.clock = clk.Now
	tr := testTree{}
	names := make([]string, leaves)
	for i := range names {
		names[i] = "leaf " + strconv.Itoa(i)
		tr.add(t, names[i])
	}
	var released []block.NamedRoot
	for i := 0; i < leaves; i += 1000 {
		batch := names[i:min(leaves, i+1000)]
		roots := make([]block.NamedRoot, len(batch))
		for j, name := range batch {
			roots[j] = block.NamedRoot{Name: name, Ref: tr[name].Ref}
			released = append(released, block.NamedRoot{Name: name, Previous: tr[name].Ref})
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(batch...), roots); err != nil {
			t.Fatal(err)
		}
	}
	mids := storeLeaves(t, v, tr, "wleaf ", leaves)
	wnames := make([]string, leaves)
	for i := range wnames {
		wnames[i] = "wleaf " + strconv.Itoa(i)
	}
	wide := tr.add(t, "W", wnames...)
	moves := []block.NamedRoot{{Name: "W", Ref: wide}}
	for _, mid := range mids {
		moves = append(moves, block.NamedRoot{Name: mid, Previous: tr[mid].Ref})
	}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("W"), moves); err != nil {
		t.Fatal(err)
	}

	// Release the leaves and W, then name them past the releases.
	for i := 0; i < len(released); i += 1000 {
		if err := v.SetBucketRoots(ctx, "b", nil, released[i:min(len(released), i+1000)]); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "W", Previous: wide}}); err != nil {
		t.Fatal(err)
	}
	// Net the releases with a collector that frees nothing, so every block
	// they take to zero ripens below the records that follow.
	clk.Add(block_gc.DefaultGrace + time.Second)
	netter := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{HoldUntilFree: true, Now: v.now})
	for {
		res, err := netter.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !res.More {
			break
		}
	}

	// Name the leaves, W and the mids in again lists, entries in all.
	named := make([][]byte, 0, leaves+1+len(mids))
	for _, name := range append(append(slices.Clone(names), "W"), mids...) {
		named = append(named, block_gc.BlockKey(tr[name].Ref))
	}
	keys := make([][]byte, 0, 2048)
	for i := range entries {
		if keys = append(keys, named[i%len(named)]); len(keys) == 2048 || i == entries-1 {
			appendAgain(t, v, keys...)
			keys = keys[:0]
		}
	}
	skew := clk.skew.Load()
	closeVolume()

	// Reopen, then measure two passes: the free keeps every ripe block for
	// the unnetted records.
	v, _ = openWaitVolumeAt(t, path, "", true)
	clk = &skewClock{}
	clk.skew.Store(skew)
	v.clock = clk.Now
	var sum block_gc.CollectResult
	elapsed, waits := measureWaits(t, v, tr, func() {
		for range 2 {
			for {
				res, err := v.Collect(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				sum.Freed += res.Freed
				sum.Kept += res.Kept
				sum.TailReading += res.TailReading
				sum.TailSaturated += res.TailSaturated
				if !res.More {
					break
				}
			}
		}
	})
	if sum.Freed != 0 || sum.TailReading == 0 || sum.TailSaturated != 0 {
		t.Fatalf("freed %d, %d slices reading the tail, %d saturated: want none freed, the tail read in slices, unsaturated", sum.Freed, sum.TailReading, sum.TailSaturated)
	}
	t.Logf("tail of %d entries over %d kept blocks: two passes in %v, %d slices reading the tail, %d kept; %s", entries, len(named), elapsed, sum.TailReading, sum.Kept, waits)
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
	// Commit a root move every millisecond until fn returns.
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
	return elapsed, fmt.Sprintf("%d commits waited max %v, p99 %v, median %v",
		len(waits), waits[len(waits)-1], waits[len(waits)*99/100], waits[len(waits)/2])
}
