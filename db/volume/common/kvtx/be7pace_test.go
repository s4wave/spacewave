package kvtx_test

import (
	"context"
	"crypto/rand"
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
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	dbkvtx "github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	"github.com/sirupsen/logrus"
)

// ownedPuts puts batches as a bucket handle without collection does: through
// PrepareOwnedBlockBatch with no owner.
type ownedPuts struct {
	*volume_kvtx.Volume
}

// PutBlockBatch puts entries with no owner.
func (o ownedPuts) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	return o.PrepareOwnedBlockBatch(ctx, "", entries)
}

// statsStore is the SQLite store's counters.
type statsStore interface {
	Stats() common.StoreStats
}

// envInt reads an integer from the environment, or def.
func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// TestBe7Pace measures the wait a foreground writer sees while a build-shaped
// background writer puts diverted batches through be-7's put path: a
// PackedStore diverting to a packahead Log over a SQLite Volume with
// collection off, opened with Hyperline's World options. It runs only with
// BE7PACE set to the background batch hold in milliseconds, 0 for none.
func TestBe7Pace(t *testing.T) {
	holdEnv := os.Getenv("BE7PACE")
	if holdEnv == "" {
		t.Skip("BE7PACE unset")
	}
	holdMs, err := strconv.Atoi(holdEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer volume_kvtx.SetBackgroundBatchHold(time.Duration(holdMs) * time.Millisecond)()
	seedMB := envInt("BE7PACE_SEED_MB", 2048)
	buildBatches := envInt("BE7PACE_BATCHES", 200)
	dir := os.Getenv("BE7PACE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}

	// Open the Volume as Hyperline opens the World's, keeping its store.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(dir, "world.db")
	conf := volume_sqlite.NewConfig(path, "world")
	conf.MmapSize = 64 << 30
	var store volume_sqlite.Store
	vol, err := volume_sqlite.NewSqliteWithOptions(ctx, le, conf, volume_sqlite.Options{
		Wrap: func(_ context.Context, s volume_sqlite.Store) (volume_sqlite.Store, error) {
			store = s
			return s, nil
		},
		WALCheckpointBytes: 512 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	stats := store.(statsStore)

	// Seed stored blocks, as the World's earlier rows, unless a seed is there.
	if st, err := os.Stat(path); err != nil || st.Size() < int64(seedMB)<<20/2 {
		seedStart := time.Now()
		for written := 0; written < seedMB<<20; {
			entries := randomEntries(1024, 4096)
			if _, err := (ownedPuts{vol}).PutBlockBatch(ctx, entries); err != nil {
				t.Fatal(err)
			}
			written += 1024 * 4096
		}
		t.Logf("seeded %d MB in %v", seedMB, time.Since(seedStart).Round(time.Millisecond))
	}

	// Divert the build's values to a packahead Log, as be-7 does.
	log, err := packahead.Open(ctx, filepath.Join(dir, "world-packs"), packahead.Options{Linger: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	packed := block_store_writeback.NewPackedStore(log, ownedPuts{vol}, true)

	// Sample the WAL's size.
	var walPeak atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if st, err := os.Stat(path + "-wal"); err == nil && st.Size() > walPeak.Load() {
				walPeak.Store(st.Size())
			}
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()

	// Commit a foreground write twice a second, timing its wait for the
	// writer turn.
	var waits []time.Duration
	var waitMtx sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			queued := time.Now()
			tx, err := store.NewTransaction(ctx, true)
			if err != nil {
				t.Error(err)
				return
			}
			wait := time.Since(queued)
			for range 64 {
				key := make([]byte, 32)
				_, _ = rand.Read(key)
				value := make([]byte, 8192)
				_, _ = rand.Read(value)
				if err := tx.Set(ctx, append([]byte("fg/"), key...), value); err != nil {
					t.Error(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Error(err)
			}
			waitMtx.Lock()
			waits = append(waits, wait)
			waitMtx.Unlock()
		}
	}()

	// Put the build's batches back to back on a background context.
	before := stats.Stats()
	bctx := dbkvtx.WithBackgroundWriteContext(ctx)
	buildStart := time.Now()
	for range buildBatches {
		if _, err := packed.PutBlockBatch(bctx, randomEntries(4096, 4096)); err != nil {
			t.Fatal(err)
		}
	}
	build := time.Since(buildStart)
	close(stop)
	wg.Wait()
	after := stats.Stats()

	// Report the foreground waits, the build's time and the WAL.
	slices.Sort(waits)
	pct := func(q float64) time.Duration {
		if len(waits) == 0 {
			return 0
		}
		return waits[min(len(waits)-1, int(q*float64(len(waits))))].Round(10 * time.Microsecond)
	}
	fmt.Printf("hold %dms: build %d batches in %v; foreground %d waits p50 %v p99 %v max %v; WAL peak %d MiB; restarts forced %d truncates %d; background turns %d\n",
		holdMs, buildBatches, build.Round(time.Millisecond), len(waits), pct(0.5), pct(0.99), pct(1),
		walPeak.Load()>>20, after.RestartsForced-before.RestartsForced, after.Truncates-before.Truncates,
		after.BackgroundWaits-before.BackgroundWaits)
}

// randomEntries returns n entries of size random bytes each, with their
// references known.
func randomEntries(n, size int) []*block.PutBatchEntry {
	entries := make([]*block.PutBatchEntry, n)
	for i := range entries {
		data := make([]byte, size)
		_, _ = rand.Read(data)
		entries[i] = &block.PutBatchEntry{Data: data, RefsKnown: true}
	}
	return entries
}
