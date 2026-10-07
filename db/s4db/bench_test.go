//go:build darwin || linux || windows

package s4db

import (
	"context"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchKeys is the number of keys the benchmarks load.
const benchKeys = 200000

// loadBench fills a database with benchKeys small values in ordered commits
// of 128 keys and returns it with its keys.
func loadBench(b *testing.B) (*DB, [][]byte) {
	db, keys, _ := fill(b, benchKeys, Options{})
	return db, keys
}

// fill opens a fresh database with opts, commits n random keys with 150-byte
// values in ordered commits of 128 keys, and returns it with its keys and
// the slowest commit.
func fill(b *testing.B, n int, opts Options) (*DB, [][]byte, time.Duration) {
	// Open a fresh file.
	b.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(b.TempDir(), "bench.s4wave"), opts)
	if err != nil {
		b.Fatal(err)
	}

	// Commit the keys, timing each commit.
	rng := rand.New(rand.NewPCG(1, 2))
	keys := make([][]byte, n)
	val := make([]byte, 150)
	var slowest time.Duration
	for i := 0; i < n; i += 128 {
		start := time.Now()
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for j := i; j < min(i+128, n); j++ {
			keys[j] = binary.BigEndian.AppendUint64(nil, rng.Uint64())
			if err := tx.Set(ctx, keys[j], val); err != nil {
				b.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			b.Fatal(err)
		}
		slowest = max(slowest, time.Since(start))
	}
	return db, keys, slowest
}

// BenchmarkGet measures point reads, each in its own read transaction, from
// parallel goroutines, with the index cached and with a cache too small for
// it, where most reads decode a page.
func BenchmarkGet(b *testing.B) {
	runs := []struct {
		name string
		opts Options
	}{
		{"cached", Options{}},
		{"misses", Options{CacheBytes: 2 << 20}},
	}
	for _, run := range runs {
		b.Run(run.name, func(b *testing.B) {
			// Load and warm the database.
			db, keys, _ := fill(b, benchKeys, run.opts)
			defer db.Close()
			ctx := context.Background()
			get := func(k []byte) {
				tx, err := db.NewTransaction(ctx, false)
				if err != nil {
					b.Fatal(err)
				}
				if _, ok, err := tx.Get(ctx, k); err != nil || !ok {
					b.Fatal("missing key", err)
				}
				tx.Discard()
			}
			for _, k := range keys {
				get(k)
			}

			// Read random keys.
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				rng := rand.New(rand.NewPCG(rand.Uint64(), 0))
				for pb.Next() {
					get(keys[rng.IntN(len(keys))])
				}
			})
		})
	}
}

// BenchmarkFill measures ordered commits of 128 small values.
func BenchmarkFill(b *testing.B) {
	for b.Loop() {
		db, _ := loadBench(b)
		_ = db.Close()
	}
}

// BenchmarkCheckpointFill measures ordered commits of 128 small values into
// a million keys with checkpoints on and off. It reports the slowest commit
// and the drive's flush time: checkpoints build their trees off the writer
// lock, so no commit should wait much longer than a flush.
func BenchmarkCheckpointFill(b *testing.B) {
	runs := []struct {
		name string
		opts Options
	}{
		{"checkpoints", Options{}},
		{"no-checkpoints", Options{CheckpointMin: math.MaxInt64, CheckpointMax: math.MaxInt64}},
	}
	for _, run := range runs {
		b.Run(run.name, func(b *testing.B) {
			for b.Loop() {
				// Fill, then time one flush of the filled file.
				db, _, slowest := fill(b, 1_000_000, run.opts)
				start := time.Now()
				if err := db.s.flushDurable(); err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(time.Since(start).Microseconds())/1000, "flush-ms")
				b.ReportMetric(float64(slowest.Microseconds())/1000, "slowest-commit-ms")

				// Close without timing the final checkpoint.
				b.StopTimer()
				if err := db.Close(); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
		})
	}
}

// BenchmarkReplay measures opening a crash copy whose log holds a full
// 64 MiB overlay, the most a process opening the file replays.
func BenchmarkReplay(b *testing.B) {
	// Open a file that checkpoints at 64 MiB.
	ctx := context.Background()
	dir := b.TempDir()
	path := filepath.Join(dir, "replay.s4wave")
	opts := Options{CheckpointMin: 64 << 20, CheckpointMax: 64 << 20}
	db, err := Open(path, opts)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Commit small values until the overlay is nearly full.
	rng := rand.New(rand.NewPCG(1, 2))
	val := make([]byte, 150)
	for db.cur.Load().overlayBytes < opts.CheckpointMax*15/16 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for range 128 {
			if err := tx.Set(ctx, binary.BigEndian.AppendUint64(nil, rng.Uint64()), val); err != nil {
				b.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			b.Fatal(err)
		}
	}

	// Copy the file as a crash would leave it.
	if err := db.Sync(ctx); err != nil {
		b.Fatal(err)
	}
	snap, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}

	// Open a fresh crash copy each time; closing checkpoints the copy.
	crashed := filepath.Join(dir, "crashed.s4wave")
	var opening time.Duration
	for b.Loop() {
		writeSynced(b, crashed, snap)
		start := time.Now()
		c, err := Open(crashed, opts)
		if err != nil {
			b.Fatal(err)
		}
		opening += time.Since(start)
		if err := c.Close(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(opening.Milliseconds())/float64(b.N), "open-ms")
	b.ReportMetric(float64(db.cur.Load().overlayBytes)/(1<<20), "overlay-MiB")
}

// BenchmarkBlockSpace fills about 922 MiB of block-shaped values, as a
// Volume holds, and reports the size of the space record each checkpoint
// rewrites.
func BenchmarkBlockSpace(b *testing.B) {
	for b.Loop() {
		// Fill 40k keys: seven in ten blocks of 256 B to 256 KiB, the rest
		// small records.
		ctx := context.Background()
		path := filepath.Join(b.TempDir(), "blocks.s4wave")
		db, err := Open(path, Options{})
		if err != nil {
			b.Fatal(err)
		}
		rng := rand.New(rand.NewPCG(3, 4))
		buf := make([]byte, 256<<10)
		for i := 0; i < 40000; i += 128 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for j := i; j < min(i+128, 40000); j++ {
				n := 100 + rng.IntN(200)
				if j%10 < 7 {
					n = []int{256, 256, 256, 256, 4096, 4096, 4096, 32 << 10, 32 << 10, 256 << 10}[rng.IntN(10)]
				}
				if err := tx.Set(ctx, binary.BigEndian.AppendUint64(nil, rng.Uint64()), buf[:n]); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
				b.Fatal(err)
			}
		}

		// Close, which checkpoints, and read the saved space record.
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
		db, err = Open(path, Options{})
		if err != nil {
			b.Fatal(err)
		}
		st := db.cur.Load()
		b.ReportMetric(float64(st.sb.space.n), "space-bytes")
		b.ReportMetric(float64(st.sb.treePages), "tree-pages")
		_, size := usage(b, path)
		b.ReportMetric(float64(size)/(1<<20), "file-MiB")
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// writeSynced writes data to a new file at path and flushes it, so a later
// open does not pay for the copy's flush.
func writeSynced(b *testing.B, path string, data []byte) {
	// Create the file.
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	// Write and flush the data.
	if _, err := f.Write(data); err != nil {
		b.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		b.Fatal(err)
	}
}
