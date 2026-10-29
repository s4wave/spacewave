//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

// checkpointKeys returns a repeatable million random 32-byte keys.
func checkpointKeys() [][]byte {
	rng := rand.New(rand.NewPCG(1, 2))
	keys := make([][]byte, 1_000_000)
	for i := range keys {
		keys[i] = make([]byte, 32)
		for j := range 4 {
			binary.BigEndian.PutUint64(keys[i][j*8:], rng.Uint64())
		}
	}
	return keys
}

// checkpointOptions keeps the update overlay below the automatic checkpoint threshold.
func checkpointOptions() Options {
	return Options{CheckpointMin: 64 << 20, CheckpointMax: 64 << 20}
}

// TestCheckpointWorkingSetFixture creates an explicitly requested disposable fixture.
func TestCheckpointWorkingSetFixture(t *testing.T) {
	// Require an unused path so the fixture cannot replace saved data.
	path := os.Getenv("S4DB_CHECKPOINT_CREATE")
	if path == "" {
		t.Skip("set S4DB_CHECKPOINT_CREATE to create a fresh benchmark fixture")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path, Options{CheckpointMin: 256 << 20, CheckpointMax: 256 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	// Fill through ordinary ordered commits, then checkpoint once on Close.
	ctx := t.Context()
	keys := checkpointKeys()
	val := bytes.Repeat([]byte{1}, 150)
	for start := 0; start < len(keys); start += 128 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range keys[start:min(start+128, len(keys))] {
			if err := tx.Set(ctx, key, val); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			t.Fatal(err)
		}
		tx.Discard()
	}
}

// checkpointHeap writes a collected Go heap at a checkpoint phase when requested.
func checkpointHeap(b *testing.B, phase string) {
	// Leave timing runs on the collector's normal schedule.
	b.Helper()
	dir := os.Getenv("S4DB_CHECKPOINT_PROFILE")
	if dir == "" {
		return
	}

	// Collect twice so the profile includes frees from the preceding collection.
	runtime.GC()
	runtime.GC()

	// Write one collected snapshot of the checkpoint's working set.
	if err := os.MkdirAll(dir, 0755); err != nil {
		b.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, phase+".pb.gz"))
	if err != nil {
		b.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
}

// checkpointCopy copies and flushes a closed fixture without retaining its bytes.
func checkpointCopy(b *testing.B, fixture, path string) {
	// Keep both file lifetimes inside this copy.
	b.Helper()
	src, err := os.Open(fixture)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			b.Error(err)
		}
	}()
	dst, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := dst.Close(); err != nil {
			b.Error(err)
		}
	}()

	// Finish the copy's writes before opening the database.
	if _, err := io.Copy(dst, src); err != nil {
		b.Fatal(err)
	}
	if err := dst.Sync(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkCheckpointWorkingSet measures a full-tree checkpoint and the reads
// around it on a copy of S4DB_CHECKPOINT_FIXTURE. Profiling runs collect at each
// production checkpoint phase; timing runs leave S4DB_CHECKPOINT_PROFILE unset.
func BenchmarkCheckpointWorkingSet(b *testing.B) {
	// Require the same closed fixture for every implementation.
	fixture := os.Getenv("S4DB_CHECKPOINT_FIXTURE")
	if fixture == "" {
		b.Skip("set S4DB_CHECKPOINT_FIXTURE to a checkpoint fixture")
	}
	if os.Getenv("S4DB_CHECKPOINT_PROFILE") != "" {
		runtime.MemProfileRate = 64 << 10
	}

	// Reuse the same key, value, and read sequences in each iteration.
	keys := checkpointKeys()
	ctx := b.Context()
	original := bytes.Repeat([]byte{1}, 150)
	updated := bytes.Repeat([]byte{2}, 150)
	const changed = 262144
	const reads = 200000
	var updateTime, buildTime, saveTime, beforeTime, afterTime time.Duration
	var buildReads int64

	// Delay each index read of the build by S4DB_CHECKPOINT_READ_DELAY, such
	// as 1ms, to model a cold build on a slow drive.
	delay, _ := time.ParseDuration(os.Getenv("S4DB_CHECKPOINT_READ_DELAY"))
	for b.Loop() {
		// Copy the closed fixture without retaining its bytes in the Go heap.
		b.StopTimer()
		path := filepath.Join(b.TempDir(), "checkpoint.s4wave")
		checkpointCopy(b, fixture, path)
		db, err := Open(path, checkpointOptions())
		if err != nil {
			b.Fatal(err)
		}

		// Spread updates across the leaf level in ordered commits.
		start := time.Now()
		for at := 0; at < changed; at += 128 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for _, key := range keys[at : at+128] {
				if err := tx.Set(ctx, key, updated); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
				b.Fatal(err)
			}
			tx.Discard()
		}
		updateTime += time.Since(start)

		// Read the same random sample before and immediately after publication.
		read := func() time.Duration {
			// Hold one snapshot throughout this point-read sample.
			start := time.Now()
			tx, err := db.NewTransaction(ctx, false)
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Discard()

			// Check both updated values and unchanged tree entries.
			rng := rand.New(rand.NewPCG(3, 4))
			for range reads {
				i := rng.IntN(len(keys))
				want := original
				if i < changed {
					want = updated
				}
				got, found, err := tx.Get(ctx, keys[i])
				if err != nil || !found || !bytes.Equal(got, want) {
					b.Fatal("checkpoint changed a value", i, err)
				}
			}
			return time.Since(start)
		}
		beforeTime += read()
		checkpointHeap(b, "before")

		// Run the production checkpoint phases under their normal writer lock.
		if err := db.w.lock(ctx); err != nil {
			b.Fatal(err)
		}
		// Start the build cold behind a slow drive when asked.
		file := db.p.r
		if delay != 0 {
			db.p.cache = newCache(db.opts.CacheBytes)
			db.p.r = delayReader{r: file, delay: delay}
		}
		reads0 := db.p.reads.Load()
		b.StartTimer()
		start = time.Now()
		tree := db.w.newTree(db.cur.Load())
		tree.b.out.reserve = func(n uint64) ([]run, error) {
			return db.w.reserve(tree, n)
		}
		if err := tree.build(); err != nil {
			b.Fatal(err)
		}
		buildTime += time.Since(start)
		buildReads += db.p.reads.Load() - reads0
		db.p.r = file
		checkpointHeap(b, "built")
		start = time.Now()
		if err := db.w.saveHeld(ctx, tree); err != nil {
			b.Fatal(err)
		}
		saveTime += time.Since(start)
		db.w.unlock(nil)
		b.StopTimer()
		checkpointHeap(b, "saved")
		runtime.KeepAlive(tree)
		afterTime += read()
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}

		// Verify the same values after a fresh open of the saved checkpoint.
		db, err = Open(path, checkpointOptions())
		if err != nil {
			b.Fatal(err)
		}
		read()
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}

	// Report write and read phases separately from the checkpoint wall time.
	for _, metric := range []struct {
		// name is the phase's unit.
		name string
		// elapsed is the total time over benchmark iterations.
		elapsed time.Duration
	}{
		{"update-ms", updateTime}, {"build-ms", buildTime},
		{"save-ms", saveTime},
		{"before-read-ms", beforeTime}, {"after-read-ms", afterTime},
	} {
		b.ReportMetric(float64(metric.elapsed.Nanoseconds())/1e6/float64(b.N), metric.name)
	}
	b.ReportMetric(float64(buildReads)/float64(b.N), "build-reads")
}

// delayReader adds a fixed latency to each read, like a drive serving a
// cold page, while letting reads overlap.
type delayReader struct {
	// r reads the file.
	r io.ReaderAt
	// delay is the latency of each read.
	delay time.Duration
}

// ReadAt sleeps for the delay, then reads.
func (d delayReader) ReadAt(b []byte, off int64) (int, error) {
	time.Sleep(d.delay)
	return d.r.ReadAt(b, off)
}
