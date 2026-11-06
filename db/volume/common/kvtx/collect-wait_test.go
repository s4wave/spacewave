package kvtx

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
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
// sizes, and while it indexes, nets and frees one journal record of 100,000
// up pairs, and logs the longest and the 99th percentile wait. It runs only
// when KVTX_MEASURE_COLLECT is set, on the build host.
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
}

// openWaitVolume opens an s4db Volume that frees, with the grace period
// grace.
func openWaitVolume(t *testing.T, grace string) *Volume {
	t.Helper()
	db, err := s4db.Open(filepath.Join(t.TempDir(), "wait.s4db"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	v, err := NewVolume(t.Context(), "test/wait", store_kvkey.NewDefaultKVKey(), db, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: grace}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v
}

// storeLeaves stores leaves blocks, a mid per 1000 under a root of its own,
// a batch per mid, and returns the mids' names.
func storeLeaves(t *testing.T, v *Volume, tr testTree, leaves int) []string {
	t.Helper()
	var mids []string
	for m := 0; m*1000 < leaves; m++ {
		names := make([]string, 0, 1000)
		for i := m * 1000; i < min(leaves, (m+1)*1000); i++ {
			names = append(names, "leaf "+strconv.Itoa(i))
			tr.add(t, names[len(names)-1])
		}
		mid := "mid " + strconv.Itoa(m)
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
	mids := storeLeaves(t, v, tr, leaves)
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
// the collector indexes the record, then nets it and frees the tree a grace
// period later.
func measureLargeRecordWait(t *testing.T, leaves int) {
	// Store the leaves, then the parents and top in one batch.
	ctx := t.Context()
	v := openWaitVolume(t, "")
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v.clock = clk.Now
	tr := testTree{}
	mids := storeLeaves(t, v, tr, leaves)
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

	// Index the young record, then net it and free the tree once it ages,
	// while a writer commits.
	var freed uint64
	elapsed, waits := measureWaits(t, v, tr, func() {
		collectToEnd(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		freed = collectToEnd(t, v)
	})
	t.Logf("one record of %d ups: freed %d in %v; %s", leaves, freed, elapsed, waits)
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
