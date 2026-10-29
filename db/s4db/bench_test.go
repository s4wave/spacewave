//go:build darwin || linux || windows

package s4db

import (
	"context"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// benchKeys is the number of keys the benchmarks load.
const benchKeys = 200000

// loadBench fills a database with benchKeys small values in ordered commits
// of 128 keys and returns it with its keys.
func loadBench(b *testing.B) (*DB, [][]byte) {
	db, keys, _ := fill(b, benchKeys, Options{}, randKey)
	return db, keys
}

// randKey returns a random eight-byte key.
func randKey(rng *rand.Rand) []byte {
	return binary.BigEndian.AppendUint64(nil, rng.Uint64())
}

// blockKey returns a key shaped like a block store key: a store prefix and
// a SHA-256 block reference, whose first ten bytes every block key shares.
func blockKey(rng *rand.Rand) []byte {
	k := []byte{'h', '/', 'b', '/', 0x0a, 0x24, 0x08, 0x01, 0x12, 0x20}
	for range 4 {
		k = binary.BigEndian.AppendUint64(k, rng.Uint64())
	}
	return k
}

// fill opens a fresh database with opts, commits n keys from key with
// 150-byte values in ordered commits of 128 keys, and returns it with its
// keys and the slowest commit.
func fill(b *testing.B, n int, opts Options, key func(*rand.Rand) []byte) (*DB, [][]byte, time.Duration) {
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
			keys[j] = key(rng)
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
// parallel goroutines, with the index cached, with a cache too small for it,
// where most reads decode a page, and with block store keys.
func BenchmarkGet(b *testing.B) {
	runs := []struct {
		name string
		opts Options
		key  func(*rand.Rand) []byte
	}{
		{"cached", Options{}, randKey},
		{"misses", Options{CacheBytes: 2 << 20}, randKey},
		{"blocks", Options{}, blockKey},
	}
	for _, run := range runs {
		b.Run(run.name, func(b *testing.B) {
			// Load and warm the database.
			db, keys, _ := fill(b, benchKeys, run.opts, run.key)
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
				db, _, slowest := fill(b, 1_000_000, run.opts, randKey)
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

// BenchmarkChurnSpace keeps about 200k keys live while replacing a twentieth
// of them per round, as a block store under garbage collection does, and
// reports how far the file length runs ahead of the pages in use. Most keys
// are small records held in the index, so each checkpoint rewrites a large
// part of it.
func BenchmarkChurnSpace(b *testing.B) {
	for b.Loop() {
		// Open with small checkpoints, so the rounds checkpoint often.
		ctx := context.Background()
		path := filepath.Join(b.TempDir(), "churn.s4wave")
		db, err := Open(path, Options{CheckpointMin: 4 << 20, CheckpointMax: 4 << 20})
		if err != nil {
			b.Fatal(err)
		}
		rng := rand.New(rand.NewPCG(5, 6))
		buf := make([]byte, 48<<10)
		var keys [][]byte
		commit := func(n int, fn func(tx *Tx)) {
			// Apply fn n times in one transaction.
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for range n {
				fn(tx.(*Tx))
			}

			// Commit it.
			if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
				b.Fatal(err)
			}
		}
		set := func(tx *Tx) {
			// Most keys are small records, the rest blocks of 2 to 48 KiB.
			n := 64 + rng.IntN(256)
			if rng.IntN(100) >= 92 {
				n = 2<<10 + rng.IntN(46<<10)
			}
			k := binary.BigEndian.AppendUint64(nil, rng.Uint64())
			if err := tx.Set(ctx, k, buf[:n]); err != nil {
				b.Fatal(err)
			}
			keys = append(keys, k)
		}

		// Fill, then replace 10k random keys per round with new ones.
		for i := 0; i < 200_000; i += 256 {
			commit(256, set)
		}
		for range 60 {
			for i := 0; i < 10_000; i += 250 {
				commit(250, func(tx *Tx) {
					// Delete a random live key.
					j := rng.IntN(len(keys))
					if err := tx.Delete(ctx, keys[j]); err != nil {
						b.Fatal(err)
					}
					keys[j] = keys[len(keys)-1]
					keys = keys[:len(keys)-1]

					// Add a new one in its place.
					set(tx)
				})
			}
		}

		// Report the length against the pages in use.
		if err := db.Sync(ctx); err != nil {
			b.Fatal(err)
		}
		u, err := db.Usage(ctx)
		if err != nil {
			b.Fatal(err)
		}
		used := u.ValueChunkBytes + u.IndexChunkBytes + u.LogBytes
		b.ReportMetric(float64(u.FileBytes)/(1<<20), "file-MiB")
		b.ReportMetric(float64(used)/(1<<20), "used-MiB")
		b.ReportMetric(float64(u.FileBytes)/float64(used), "file/used")
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSweepSpace fills about 300k keys, deletes half of them at once as
// a block store's garbage collection sweep does, then keeps replacing keys at
// a steady rate and idles. It reports the file length against the pages in use
// at the end of the churn and after the idle time, and the commit latency
// during the churn.
func BenchmarkSweepSpace(b *testing.B) {
	for b.Loop() {
		// Open with small checkpoints, so the churn checkpoints often.
		ctx := context.Background()
		path := filepath.Join(b.TempDir(), "sweep.s4wave")
		db, err := Open(path, Options{CheckpointMin: 4 << 20, CheckpointMax: 4 << 20})
		if err != nil {
			b.Fatal(err)
		}
		rng := rand.New(rand.NewPCG(7, 8))
		buf := make([]byte, 48<<10)
		var keys [][]byte
		commit := func(n int, fn func(tx *Tx)) time.Duration {
			// Apply fn n times in one transaction.
			start := time.Now()
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for range n {
				fn(tx.(*Tx))
			}

			// Commit it.
			if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
				b.Fatal(err)
			}
			return time.Since(start)
		}
		set := func(tx *Tx) {
			// Most keys are small records, the rest blocks of 2 to 48 KiB.
			n := 64 + rng.IntN(256)
			if rng.IntN(100) >= 92 {
				n = 2<<10 + rng.IntN(46<<10)
			}
			k := binary.BigEndian.AppendUint64(nil, rng.Uint64())
			if err := tx.Set(ctx, k, buf[:n]); err != nil {
				b.Fatal(err)
			}
			keys = append(keys, k)
		}
		del := func(tx *Tx) {
			// Delete a random live key.
			j := rng.IntN(len(keys))
			if err := tx.Delete(ctx, keys[j]); err != nil {
				b.Fatal(err)
			}
			keys[j] = keys[len(keys)-1]
			keys = keys[:len(keys)-1]
		}
		report := func(name string) {
			// Report the length against the pages in use.
			if err := db.Sync(ctx); err != nil {
				b.Fatal(err)
			}
			u, err := db.Usage(ctx)
			if err != nil {
				b.Fatal(err)
			}
			used := u.ValueChunkBytes + u.IndexChunkBytes + u.LogBytes
			b.ReportMetric(float64(u.FileBytes)/(1<<20), name+"-file-MiB")
			b.ReportMetric(float64(u.FileBytes)/float64(used), name+"-file/used")
		}

		// Fill, then sweep half the keys.
		for i := 0; i < 300_000; i += 256 {
			commit(256, set)
		}
		for i := 0; i < 150_000; i += 1000 {
			commit(1000, del)
		}
		report("sweep")

		// Replace 50 keys every 2 ms for about 8 s, timing each commit.
		var lat []time.Duration
		for range 4000 {
			lat = append(lat, commit(50, func(tx *Tx) {
				del(tx)
				set(tx)
			}))
			time.Sleep(2 * time.Millisecond)
		}
		report("churn")
		slices.Sort(lat)
		b.ReportMetric(float64(lat[len(lat)/2].Microseconds()), "commit-p50-us")
		b.ReportMetric(float64(lat[len(lat)*99/100].Microseconds()), "commit-p99-us")
		b.ReportMetric(float64(lat[len(lat)-1].Microseconds()), "commit-max-us")

		// Idle, reporting as compaction proceeds.
		for i := range 6 {
			time.Sleep(10 * time.Second)
			if i != 5 {
				report("idle-" + strconv.Itoa(10*(i+1)))
			}
		}
		report("idle")
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
