package block_store_writeback

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	"github.com/sirupsen/logrus"
)

// evictTimingEnv names the environment variable that runs
// TestEvictSQLiteTiming when set to its block count.
const evictTimingEnv = "SPACEWAVE_EVICT_TIMING"

// evictTimingWritersEnv names the environment variable that sets the
// foreground writers of TestEvictSQLiteTiming, one when unset.
const evictTimingWritersEnv = "SPACEWAVE_EVICT_TIMING_WRITERS"

// apartValues hides the Volume's EditValues, so eviction drops values and
// changes the queue in two writes, as before the joint write.
type apartValues struct {
	block.StoreOps
}

// TestEvictSQLiteTiming times eviction passes over a SQLite Volume on disk
// while a foreground writer commits small blocks: one write per batch sized by
// its writer hold, against two writes per fixed batch of 1024. It runs only
// with evictTimingEnv set, and logs the numbers.
func TestEvictSQLiteTiming(t *testing.T) {
	if os.Getenv(evictTimingEnv) == "" {
		t.Skip("set " + evictTimingEnv + " to run")
	}
	for _, joint := range []bool{false, true} {
		name := "apart-1024"
		if joint {
			name = "joint-sized"
		}
		t.Run(name, func(t *testing.T) { timeEvictSQLite(t, joint) })
	}
}

// timeEvictSQLite runs one timed pass, as TestEvictSQLiteTiming describes.
func timeEvictSQLite(t *testing.T, joint bool) {
	// Open a SQLite Volume and a placed store over it.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_sqlite.NewSqlite(ctx, le, volume_sqlite.NewConfig(filepath.Join(t.TempDir(), "evict.db"), "hydra"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	values := block.StoreOps(vol)
	if !joint {
		values = apartValues{vol}
	}
	s, err := NewStore(ctx, ownedStore{vol}, values, markers, testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !joint {
		s.evictBatchSize = 1024
	}

	// Write the blocks of 3 KiB in batches and upload them.
	var n int
	if _, err := fmt.Sscan(os.Getenv(evictTimingEnv), &n); err != nil || n <= 0 {
		t.Fatalf("%s=%q: want a block count", evictTimingEnv, os.Getenv(evictTimingEnv))
	}
	for done := 0; done < n; {
		var entries []*block.PutBatchEntry
		for range min(1000, n-done) {
			data := make([]byte, 3<<10)
			_, _ = rand.Read(data)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		}
		if _, err := s.PutBlockBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
		done += len(entries)
	}
	drain(t, s, newUntrackedBlockStore())

	// Commit small foreground blocks, one per transaction, timing each, from
	// as many writers as evictTimingWritersEnv names, one by default.
	writers := 1
	if v := os.Getenv(evictTimingWritersEnv); v != "" {
		if _, err := fmt.Sscan(v, &writers); err != nil {
			t.Fatal(err)
		}
	}
	fgCtx, fgCancel := context.WithCancel(ctx)
	var mtx sync.Mutex
	var waits []time.Duration
	var fg sync.WaitGroup
	for range writers {
		fg.Go(func() {
			for fgCtx.Err() == nil {
				data := make([]byte, 512)
				_, _ = rand.Read(data)
				start := time.Now()
				if _, _, err := vol.PutBlock(fgCtx, data, leafOpts); err != nil {
					return
				}
				mtx.Lock()
				waits = append(waits, time.Since(start))
				mtx.Unlock()
				time.Sleep(2 * time.Millisecond)
			}
		})
	}

	// Evict every block, logging each batch's hold.
	var holds []time.Duration
	evictHoldHook = func(_ int, hold time.Duration) { holds = append(holds, hold) }
	defer func() { evictHoldHook = nil }()
	start := time.Now()
	result, err := s.Evict(ctx, nil, 1<<40, nil, nil)
	elapsed := time.Since(start)
	fgCancel()
	fg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != n {
		t.Fatalf("evicted %d of %d", result.Evicted, n)
	}

	// Report the rate, the holds and the foreground waits.
	mib := float64(result.EvictedBytes) / (1 << 20)
	t.Logf("evicted %d blocks, %.1f MiB in %v: %.1f MiB/s over %d batches (%.1f ms a batch), final batch size %d",
		result.Evicted, mib, elapsed.Round(time.Millisecond), mib/elapsed.Seconds(), result.Batches,
		float64(elapsed.Milliseconds())/float64(result.Batches), s.evictBatchSize)
	if len(holds) != 0 {
		slices.Sort(holds)
		t.Logf("joint writer hold: p50 %v p90 %v max %v", holds[len(holds)/2], holds[len(holds)*9/10], holds[len(holds)-1])
	}
	if len(waits) != 0 {
		slices.Sort(waits)
		t.Logf("foreground put: %d puts, p50 %v p99 %v max %v", len(waits), waits[len(waits)/2], waits[len(waits)*99/100], waits[len(waits)-1])
	}
}
