package block_store_fillcache

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// TestPackLogHarness compares the SQLite fill cache with a packahead Log used
// as a cache, at the size its environment names: SPACEWAVE_PACKLOG_BLOCKS
// blocks of SPACEWAVE_PACKLOG_MIN to SPACEWAVE_PACKLOG_MAX bytes. For each
// store it logs the fill cost per block, the open time, the heap the open
// keeps, and the read cost per block from one reader and from 16. It skips
// unless SPACEWAVE_PACKLOG_BLOCKS is set.
func TestPackLogHarness(t *testing.T) {
	n := harnessEnv("SPACEWAVE_PACKLOG_BLOCKS", 0)
	if n == 0 {
		t.Skip("SPACEWAVE_PACKLOG_BLOCKS is unset")
	}
	lo := harnessEnv("SPACEWAVE_PACKLOG_MIN", 1<<10)
	hi := harnessEnv("SPACEWAVE_PACKLOG_MAX", 16<<10)
	reads := harnessEnv("SPACEWAVE_PACKLOG_READS", 200000)
	arms := os.Getenv("SPACEWAVE_PACKLOG_ARMS")
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Make the blocks once; both stores take the same ones.
	rng := rand.New(rand.NewPCG(7, 8))
	refs := make([]*block.BlockRef, n)
	values := make([][]byte, n)
	var total int64
	for i := range n {
		data := make([]byte, lo+rng.IntN(hi-lo+1))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i], values[i] = ref, data
		total += int64(len(data))
	}
	t.Logf("blocks %d, value bytes %d, mean %d", n, total, total/int64(n))

	// The SQLite fill cache, in the layout a new file takes.
	if arms == "" || arms == "sqlite" {
		path := filepath.Join(dir, "cache.db")
		c, err := Open(ctx, le, path, Options{LimitBytes: 1 << 40})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		for i := range n {
			c.enqueue(&block.PutBatchEntry{Ref: refs[i], Data: values[i]})
			if (i+1)%2000 == 0 {
				if err := c.Wait(ctx); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := c.Wait(ctx); err != nil {
			t.Fatal(err)
		}
		fill := time.Since(start)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(path)
		t.Logf("sqlite fill %.1f µs/block, file %d bytes", us(fill, n), info.Size())

		heap0 := liveHeap()
		start = time.Now()
		c, err = Open(ctx, le, path, Options{LimitBytes: 1 << 40})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("sqlite open %v, heap kept %d MiB", time.Since(start), (liveHeap()-heap0)>>20)
		get := func(i int) bool {
			_, found, err := c.blocks.GetBlock(ctx, refs[i])
			return err == nil && found
		}
		readArm(t, "sqlite", n, reads, get)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// A packahead Log, filled in batches of 64 as a writer would queue them.
	if arms == "" || arms == "packlog" {
		path := filepath.Join(dir, "packs")
		keys := make([][]byte, n)
		for i := range n {
			keys[i] = packfile.BlockKey(refs[i].GetHash())
		}
		l, err := packahead.Open(ctx, path, packahead.Options{})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		batch := make([]packahead.Block, 0, 64)
		for i := range n {
			batch = append(batch, packahead.Block{Key: keys[i], Value: values[i]})
			if len(batch) == cap(batch) || i == n-1 {
				if err := l.Append(ctx, batch); err != nil {
					t.Fatal(err)
				}
				batch = batch[:0]
			}
		}
		fill := time.Since(start)
		l.Close()
		t.Logf("packlog fill %.1f µs/block, synced per batch", us(fill, n))

		// Drop the values so the heap reading holds the table alone.
		clear(values)
		heap0 := liveHeap()
		start = time.Now()
		l, err = packahead.Open(ctx, path, packahead.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("packlog open %v, heap kept %d MiB", time.Since(start), (liveHeap()-heap0)>>20)
		get := func(i int) bool {
			_, found, err := l.Get(keys[i])
			return err == nil && found
		}
		readArm(t, "packlog", n, reads, get)
		l.Close()
	}
}

// readArm logs the cost per random read of a store, from one reader and from
// 16 readers at once.
func readArm(t *testing.T, name string, n, reads int, get func(int) bool) {
	for _, readers := range []int{1, 16} {
		var wg sync.WaitGroup
		start := time.Now()
		per := reads / readers
		for r := range readers {
			wg.Go(func() {
				rng := rand.New(rand.NewPCG(uint64(r), 11))
				for range per {
					if !get(rng.IntN(n)) {
						t.Error("a read missed")
						return
					}
				}
			})
		}
		wg.Wait()
		d := time.Since(start)
		t.Logf("%s read, %d readers: %.2f µs/block wall, %.0f k blocks/s", name, readers, us(d, per*readers), float64(per*readers)/d.Seconds()/1000)
	}
}

// us returns d per item in microseconds.
func us(d time.Duration, n int) float64 {
	return float64(d.Nanoseconds()) / float64(n) / 1000
}

// liveHeap returns the heap in use after a collection.
func liveHeap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}
