package packsweep

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// scaleBlocksEnv names the variable that sets the blocks of the scale run;
// unset skips it.
const scaleBlocksEnv = "PACK_SWEEP_SCALE_BLOCKS"

// scalePackBlocksEnv names the variable that sets the blocks a pack of the
// scale run holds; unset writes full packs of 4096.
const scalePackBlocksEnv = "PACK_SWEEP_SCALE_PACK_BLOCKS"

// scaleHeapEnv names the variable naming a directory for a heap profile
// after each step of the scale run.
const scaleHeapEnv = "PACK_SWEEP_SCALE_HEAP"

// TestScaleRun runs a cycle over a synthetic store of production shape, full
// packs of 4096 blocks linking older blocks, with the scratch in an s4db
// file, and logs the time, the reads, the scratch size and the heap of each
// step. It writes nothing outside a temporary directory and an in-memory
// bucket.
func TestScaleRun(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv(scaleBlocksEnv))
	if n == 0 {
		t.Skip("set " + scaleBlocksEnv + " to run")
	}
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	rng := rand.New(rand.NewPCG(1, 2))

	// Write the packs: each block links up to two older blocks, and a chain
	// through a growing share of the blocks reaches most of the store from
	// the newest, leaving the old packs mostly dead.
	_, client := s3test.New(t)
	store := block_store_s3.NewPackStore(le, client, s3test.BucketName, "world/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer store.Close()
	refs := make([]*block.BlockRef, 0, n)
	writeStart := time.Now()
	packBlocks := 4096
	if v, _ := strconv.Atoi(os.Getenv(scalePackBlocksEnv)); v > 0 {
		packBlocks = v
	}
	chain := 0
	for len(refs) < n {
		batch := make([]*block.PutBatchEntry, 0, packBlocks)
		for range min(packBlocks, n-len(refs)) {
			i := len(refs) + len(batch)
			data := fmt.Appendf(nil, "block %d %0*d", i, 40+rng.IntN(80), 0)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			var children []*block.BlockRef
			if i > 0 {
				// Link the last chained block, more often for newer
				// blocks, so the old packs are the dead ones.
				if rng.Float64() < 0.3+0.6*float64(i)/float64(n) {
					children = append(children, refsAt(refs, batch, chain))
					chain = i
				}
				for range rng.IntN(3) {
					children = append(children, refsAt(refs, batch, max(0, i-1-rng.IntN(min(i, 200000)))))
				}
			}
			batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true})
		}
		if _, err := store.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
		for _, e := range batch {
			refs = append(refs, e.Ref)
		}
	}
	t.Logf("wrote %d blocks in %s", n, time.Since(writeStart))

	// Open the scratch file as the command does.
	path := filepath.Join(t.TempDir(), "scratch")
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	scratch := kvtx_prefixer.NewPrefixer(db, []byte("sweep/"))
	report := func(step string, start time.Time) {
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		if dir := os.Getenv(scaleHeapEnv); dir != "" {
			f, err := os.Create(filepath.Join(dir, step+".heap"))
			if err != nil {
				t.Fatal(err)
			}
			if err := pprof.WriteHeapProfile(f); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
		}
		size := dirSize(t, filepath.Dir(path))
		t.Logf("%s: %s, scratch %d bytes (%.1f a block), heap in use %d MiB",
			step, time.Since(start).Round(time.Millisecond), size, float64(size)/float64(n), ms.HeapInuse>>20)
	}

	// Snapshot, walk from the newest block, the chain's head, and judge.
	start := time.Now()
	later := func() time.Time { return time.Now().Add(48 * time.Hour) }
	progress, err := Snapshot(ctx, &SnapshotOptions{Store: scratch, Packs: store, Now: later})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot read %d packs, %d bytes", progress.Catalogued, progress.Bytes)
	report("snapshot", start)
	start = time.Now()
	head, err := Walk(ctx, &WalkOptions{Store: scratch, Roots: refs[n-1:]})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("walk reached %d of %d blocks, holes %v", head.Live, n, head.Holes)
	report("walk", start)
	start = time.Now()
	sum, err := Judge(ctx, &JudgeOptions{Store: scratch})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("judged packs %v, bytes %v", sum.Packs, sum.Bytes)
	report("judge", start)
}

// refsAt returns the ref of block i, written or in the batch being built.
func refsAt(refs []*block.BlockRef, batch []*block.PutBatchEntry, i int) *block.BlockRef {
	if i < len(refs) {
		return refs[i]
	}
	return batch[i-len(refs)].Ref
}

// dirSize sums the sizes of the files in dir.
func dirSize(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}
