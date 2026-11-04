//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// BenchmarkCheckpointReplayedOverlay measures how a reopened file drains an
// overlay far past the checkpoint limit, the shape a process restarted after
// a long burst of commits replays, while paced writers commit for a fixed
// time. It compares whole checkpoints with slices of the default size, and
// reports the replay time, the time to the first published checkpoint, the
// overlay and log left at the end, the peak heap and resident memory, and the
// writers' commit latency. S4DB_REPLAY_BASE_MB sets the size of the values a
// checkpointed tree holds before the overlay, none by default, and
// S4DB_REPLAY_MB the size of the values the overlay sets at random keys over
// the tree's and its own, 32 MB by default. S4DB_REPLAY_SECONDS sets how long
// the writers run after the replay, 15 by default.
// S4DB_REPLAY_IMAGE names a file that keeps the replayed image between runs,
// so each mode can run in its own process.
func BenchmarkCheckpointReplayedOverlay(b *testing.B) {
	const (
		valSize = 200
		writers = 8
		perTx   = 16
		pace    = 20 * time.Millisecond
	)
	base := int64(benchEnv(b, "S4DB_REPLAY_BASE_MB", 0)) << 20
	overlay := int64(benchEnv(b, "S4DB_REPLAY_MB", 32)) << 20
	runFor := time.Duration(benchEnv(b, "S4DB_REPLAY_SECONDS", 15)) * time.Second
	baseKeys := uint64(base / valSize)         // #nosec G115 -- sizes are not negative.
	keys := baseKeys + uint64(overlay/valSize) // #nosec G115 -- sizes are not negative.
	key := func(i uint64) []byte {
		return binary.BigEndian.AppendUint64(nil, i*0x9e3779b97f4a7c15)
	}
	val := bytes.Repeat([]byte{7}, valSize)

	// Write the tree, then the overlay with checkpoints held off, so a reopen
	// replays it.
	ctx := context.Background()
	img := os.Getenv("S4DB_REPLAY_IMAGE")
	if img == "" {
		img = filepath.Join(b.TempDir(), "replay.s4wave")
	}
	if _, err := os.Stat(img); err != nil {
		fillKeys(b, img, Options{SingleProcess: true}, baseKeys, func(i uint64) []byte { return key(i) }, val)
		held := 2 * overlay
		rng := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- benchmark data.
		fillKeys(b, img, Options{CheckpointMin: held, CheckpointMax: held, SingleProcess: true},
			uint64(overlay/valSize), func(uint64) []byte { return key(rng.Uint64N(keys)) }, val) // #nosec G115 -- overlay is positive.
	}

	// Drain it whole and in slices.
	for _, slice := range []int64{-1, 0} {
		name := "whole"
		if slice == 0 {
			name = "sliced"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				// Reopen a fresh copy of the file, and count resident memory
				// from here.
				b.StopTimer()
				path := filepath.Join(b.TempDir(), "drain-"+strconv.Itoa(b.N)+".s4wave")
				copyFile(b, img, path)
				runtime.GC()
				_ = os.WriteFile("/proc/self/clear_refs", []byte("5"), 0)
				b.StartTimer()
				start := time.Now()
				db, err := Open(path, Options{CheckpointSliceBytes: slice, SingleProcess: true})
				if err != nil {
					b.Fatal(err)
				}
				opened := time.Since(start)
				ckpt0 := db.cur.Load().ckpt

				// Sample the heap, the overlay and the log, and watch for the
				// first checkpoint.
				done := make(chan struct{})
				var peak uint64
				var first time.Duration
				var sampler sync.WaitGroup
				sampler.Go(func() {
					var ms runtime.MemStats
					var logged time.Time
					for {
						runtime.ReadMemStats(&ms)
						peak = max(peak, ms.HeapInuse)
						st := db.cur.Load()
						if first == 0 && st.ckpt != ckpt0 {
							first = time.Since(start)
						}
						if time.Since(logged) >= 5*time.Second {
							logged = time.Now()
							b.Logf("%s t=%.0fs overlayMB=%.0f logMB=%.0f ckpts=%d heapMB=%.0f",
								name, time.Since(start).Seconds(), float64(st.overlayBytes)/1e6,
								float64(st.logBytes)/1e6, db.Stats().Checkpoints, float64(ms.HeapInuse)/1e6)
						}
						select {
						case <-done:
							return
						case <-time.After(5 * time.Millisecond):
						}
					}
				})

				// Commit from paced writers for a fixed time.
				var mtx sync.Mutex
				var lat []time.Duration
				var wg sync.WaitGroup
				for w := range writers {
					wg.Go(func() {
						rng := rand.New(rand.NewPCG(uint64(w), 9)) // #nosec G404 -- benchmark data.
						var own []time.Duration
						tick := time.NewTicker(pace)
						defer tick.Stop()
						for time.Since(start) < opened+runFor {
							began := time.Now()
							tx, err := db.NewTransaction(ctx, true)
							if err != nil {
								b.Error(err)
								return
							}
							for range perTx {
								if err := tx.Set(ctx, key(rng.Uint64N(keys)), val); err != nil {
									b.Error(err)
									return
								}
							}
							if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
								b.Error(err)
								return
							}
							tx.Discard()
							own = append(own, time.Since(began))
							<-tick.C
						}
						mtx.Lock()
						lat = append(lat, own...)
						mtx.Unlock()
					})
				}
				wg.Wait()
				close(done)
				sampler.Wait()

				// Report the checkpoints and what the writers saw.
				st := db.Stats()
				slices.Sort(lat)
				b.ReportMetric(opened.Seconds()*1e3, "openms")
				b.ReportMetric(first.Seconds()*1e3, "firstCkptms")
				b.ReportMetric(float64(st.OverlayBytes)/1e6, "endOverlayMB")
				b.ReportMetric(float64(st.LogBytes)/1e6, "endLogMB")
				b.ReportMetric(float64(st.Checkpoints), "ckpts")
				b.ReportMetric(float64(peak)/1e6, "peakHeapMB")
				b.ReportMetric(float64(peakRSS())/1e6, "peakRSSMB")
				b.ReportMetric(float64(len(lat)), "commits")
				b.ReportMetric(float64(lat[len(lat)*99/100].Milliseconds()), "p99ms")
				b.ReportMetric(float64(lat[len(lat)-1].Milliseconds()), "maxms")
				b.StopTimer()
				if err := db.Close(); err != nil {
					b.Fatal(err)
				}
				_ = os.Remove(path)
				b.StartTimer()
			}
		})
	}
}

// fillKeys opens the file at path with opts, sets n keys that key names to
// val in ordered commits of 1024, and closes it. A file opened with default
// options is compacted first, so its keys end in a checkpointed tree.
func fillKeys(b *testing.B, path string, opts Options, n uint64, key func(uint64) []byte, val []byte) {
	// Set the keys in batches.
	ctx := context.Background()
	db, err := Open(path, opts)
	if err != nil {
		b.Fatal(err)
	}
	for start := uint64(0); start < n; start += 1024 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for i := start; i < min(start+1024, n); i++ {
			if err := tx.Set(ctx, key(i), val); err != nil {
				b.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			b.Fatal(err)
		}
		tx.Discard()
	}

	// Checkpoint a tree written with default limits, and close.
	if opts.CheckpointMax == 0 {
		if err := db.Compact(ctx); err != nil {
			b.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		b.Fatal(err)
	}
}

// benchEnv returns the integer in the environment variable name, or def
// when it is unset.
func benchEnv(b *testing.B, name string, def int) int {
	s := os.Getenv(name)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		b.Fatalf("%s: %v", name, err)
	}
	return n
}

// copyFile streams the file at src to dst.
func copyFile(b *testing.B, src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		b.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		b.Fatal(err)
	}
	if err := out.Close(); err != nil {
		b.Fatal(err)
	}
}

// peakRSS returns the process's peak resident memory since the last reset
// through /proc/self/clear_refs, or 0 where /proc is missing.
func peakRSS() int64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for line := range bytes.Lines(b) {
		if v, ok := bytes.CutPrefix(line, []byte("VmHWM:")); ok {
			kb, _ := strconv.ParseInt(string(bytes.Fields(v)[0]), 10, 64)
			return kb << 10
		}
	}
	return 0
}
