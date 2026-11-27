package block_store_fillcache

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// slowObjects adds a fixed latency and a transfer time at bytesPerSec to each
// range read of an object store, as a bucket across a network has.
type slowObjects struct {
	block_store_s3.ObjectStore
	latency     time.Duration
	bytesPerSec int
}

func (s *slowObjects) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	//gostyle:timer external
	case <-time.After(s.latency + time.Duration(length)*time.Second/time.Duration(s.bytesPerSec)):
	}
	return s.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

// harnessEnv returns the integer value of environment variable name, or def.
func harnessEnv(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return n
	}
	return def
}

// TestWarmResidentHarness measures a restart's warm-up at the size its
// environment selects: a bucket of SPACEWAVE_WARM_BLOCKS blocks of 1 to 16 KiB
// in packs of 2,000, a resident list of a random SPACEWAVE_WARM_LISTED_PCT
// percent of them, and SPACEWAVE_WARM_LATENCY_MS of latency and
// SPACEWAVE_WARM_MBPS megabytes a second per range read, priced as Backblaze
// B2.
// It reports the warm-up's time and requests, and a foreground reader's
// latency over unlisted blocks with and without the warm-up running. It runs
// only with SPACEWAVE_WARM_BLOCKS set, on the build host.
func TestWarmResidentHarness(t *testing.T) {
	// Read the harness's size from the environment.
	total := harnessEnv("SPACEWAVE_WARM_BLOCKS", 0)
	if total == 0 {
		t.Skip("set SPACEWAVE_WARM_BLOCKS to run the warm-up harness")
	}
	pct := harnessEnv("SPACEWAVE_WARM_LISTED_PCT", 30)
	latency := time.Duration(harnessEnv("SPACEWAVE_WARM_LATENCY_MS", 20)) * time.Millisecond
	bytesPerSec := harnessEnv("SPACEWAVE_WARM_MBPS", 25) * 1e6
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.WarnLevel)

	// Write the blocks into packs through a store that indexes their extents.
	_, client := s3test.New(t)
	index := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	opts := &block_store_s3.PackStoreOptions{
		KeepPacks:       true,
		Index:           index,
		SoleWriter:      true,
		RefreshInterval: time.Hour,
		Pricing:         block_store_s3.PricingForEndpoint("s3.us-west-004.backblazeb2.com"),
	}
	writer := block_store_s3.NewPackStore(le, client, s3test.BucketName, "p/", opts)
	rng := rand.New(rand.NewPCG(1, 2))
	refs := make([]*block.BlockRef, 0, total)
	sizes := make(map[*block.BlockRef]int, total)
	var batch []*block.PutBatchEntry
	var stored int64
	flush := func() {
		if _, err := writer.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
		batch = batch[:0]
	}
	for range total {
		data := make([]byte, 1<<10+rng.IntN(15<<10))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
		sizes[ref] = len(data)
		stored += int64(len(data))
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data})
		if len(batch) == 2000 {
			flush()
		}
	}
	if len(batch) != 0 {
		flush()
	}
	writer.Close()
	t.Logf("bucket: %d blocks, %d MiB", total, stored>>20)

	// List a random share of the blocks. The rest are the foreground's.
	rng.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })
	listedRefs, foreground := refs[:total*pct/100], refs[total*pct/100:]
	var listedBytes uint64
	for _, ref := range listedRefs {
		listedBytes += uint64(sizes[ref])
	}
	keys := make([]string, 0, len(listedRefs))
	for _, ref := range listedRefs {
		key, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, string(key))
	}

	// openRestart opens a restarted store over the same index and an empty
	// cache holding the list.
	openRestart := func(name string) (*block_store_s3.PackStore, *Cache) {
		store := block_store_s3.NewPackStore(le, &slowObjects{ObjectStore: client, latency: latency, bytesPerSec: bytesPerSec}, s3test.BucketName, "p/", opts)
		t.Cleanup(store.Close)
		if err := store.Preload(ctx); err != nil {
			t.Fatal(err)
		}
		c := openTest(t, filepath.Join(t.TempDir(), name), 1<<40)
		if err := c.SaveResident(ctx, "store", slices.Values(keys)); err != nil {
			t.Fatal(err)
		}
		return store, c
	}

	// foregroundReads reads up to n unlisted blocks one at a time from start,
	// as a view's misses do, until stop closes, and returns their latencies.
	foregroundReads := func(store *block_store_s3.PackStore, start, n int, stop <-chan struct{}) []time.Duration {
		var lat []time.Duration
		for i := start; i < min(start+n, len(foreground)); i++ {
			select {
			case <-stop:
				return lat
			default:
			}
			began := time.Now()
			if _, found, err := store.GetBlock(ctx, foreground[i]); err != nil || !found {
				t.Fatalf("foreground read = %v, %v", found, err)
			}
			lat = append(lat, time.Since(began))
		}
		return lat
	}

	// Warm one restart while the foreground reads beside it.
	store, c := openRestart("warm")
	stop := make(chan struct{})
	var during []time.Duration
	var wg sync.WaitGroup
	wg.Go(func() { during = foregroundReads(store, 0, len(foreground), stop) })
	began := time.Now()
	counts, err := c.WarmResident(ctx, "store", store.ReadBatch)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	warm := time.Since(began)
	close(stop)
	wg.Wait()

	// Every listed block is now in the cache, and no fill was dropped.
	reads := store.ReadCounts()
	var left int
	if _, err := c.MissingResident(ctx, "store", func(refs []*block.BlockRef) error {
		left += len(refs)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cacheCounts := c.Counts()
	t.Logf("warm-up: %+v in %v, %d range reads for %d blocks (%.1f a read), %d MiB read for %d MiB listed, %d left missing, %d fills dropped",
		counts, warm.Round(time.Millisecond), reads.BatchReads, reads.BatchBlocks,
		float64(reads.BatchBlocks)/float64(max(reads.BatchReads, 1)), reads.BatchBytes>>20, listedBytes>>20, left, cacheCounts.Dropped)
	if counts.Fetched != len(listedRefs) || left != 0 || cacheCounts.Dropped != 0 {
		t.Fatalf("warm-up fetched %d of %d, left %d, dropped %d", counts.Fetched, len(listedRefs), left, cacheCounts.Dropped)
	}

	// Read as many foreground blocks on a restart with no warm-up.
	idle, _ := openRestart("idle")
	alone := foregroundReads(idle, len(during), len(during), nil)
	p := func(lat []time.Duration, q float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		s := slices.Clone(lat)
		slices.Sort(s)
		return s[int(q*float64(len(s)-1))]
	}
	t.Logf("foreground during the warm-up: %d reads, p50 %v, p99 %v", len(during), p(during, 0.5), p(during, 0.99))
	t.Logf("foreground alone:              %d reads, p50 %v, p99 %v", len(alone), p(alone, 0.5), p(alone, 0.99))
	t.Logf("one read per listed block would take %d requests, %v at 2 in flight",
		len(listedRefs), (time.Duration(len(listedRefs)/2) * latency).Round(time.Millisecond))
}
