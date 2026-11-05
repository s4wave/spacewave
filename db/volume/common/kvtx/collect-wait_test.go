package kvtx

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// TestCommitWaitDuringCollection measures how long a writer's commit waits
// while the collector frees a large garbage tree on an s4db Volume, at two
// sizes, and logs the longest and the 99th percentile wait. It runs only when
// KVTX_MEASURE_COLLECT is set, on the build host.
func TestCommitWaitDuringCollection(t *testing.T) {
	if os.Getenv("KVTX_MEASURE_COLLECT") == "" {
		t.Skip("set KVTX_MEASURE_COLLECT to measure")
	}
	for _, leaves := range []int{10_000, 100_000} {
		t.Run(strconv.Itoa(leaves), func(t *testing.T) {
			measureCommitWait(t, leaves)
		})
	}
}

// measureCommitWait builds main -> top -> mids -> leaves, releases main,
// collects to the end while a writer moves a root every millisecond, and
// logs the writer's waits.
func measureCommitWait(t *testing.T, leaves int) {
	// Open an s4db Volume that frees at once.
	ctx := t.Context()
	db, err := s4db.Open(filepath.Join(t.TempDir(), "wait.s4db"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	v, err := NewVolume(ctx, "test/wait", store_kvkey.NewDefaultKVKey(), db, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })

	// Store the tree, a mid per 1000 leaves, a batch per mid.
	tr := testTree{}
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
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(names, mid)...), []block.NamedRoot{{Name: mid, Ref: tr[mid].Ref}}); err != nil {
			t.Fatal(err)
		}
	}
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

	// Commit a root move every millisecond while collecting to the end.
	var mtx sync.Mutex
	var waits []time.Duration
	done := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		held := tr.add(t, "held")
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
			next := tr.add(t, "w "+strconv.Itoa(i))
			start := time.Now()
			err := v.SetBucketRoots(ctx, "w", tr.entries("w "+strconv.Itoa(i)), []block.NamedRoot{{Name: "w", Ref: next, Previous: prev}})
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
	var freed uint64
	for {
		res, err := v.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		freed += res.Freed
		if !res.More {
			break
		}
	}
	elapsed := time.Since(start)
	close(done)
	<-writerDone

	// Report the waits.
	mtx.Lock()
	defer mtx.Unlock()
	slices.Sort(waits)
	if len(waits) == 0 {
		t.Fatalf("no commit landed during the collection of %d blocks in %v", freed, elapsed)
	}
	p99 := waits[len(waits)*99/100]
	t.Logf("leaves %d: freed %d in %v; %d commits waited max %v, p99 %v, median %v",
		leaves, freed, elapsed, len(waits), waits[len(waits)-1], p99, waits[len(waits)/2])
}
