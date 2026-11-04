//go:build darwin || linux || windows

package s4db

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkCheckpointBehindWriters measures how fast a checkpoint writes while
// many write transactions queue for the writer lock, each holding it for a
// cold read's time. Its shape follows a loaded server: about 128 transactions
// waiting, random keys, and a checkpoint that rewrites about one leaf per key.
func BenchmarkCheckpointBehindWriters(b *testing.B) {
	const (
		keys    = 500_000
		writers = 128
		perTx   = 16
		hold    = 5 * time.Millisecond
		runFor  = 20 * time.Second
	)

	// Load the index and checkpoint it, so the run rewrites existing leaves.
	ctx := context.Background()
	opts := Options{CheckpointMin: 1 << 20, CheckpointMax: 1 << 20, SingleProcess: true}
	db, err := Open(filepath.Join(b.TempDir(), "writers.s4wave"), opts)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	key := func(i uint64) []byte {
		return binary.BigEndian.AppendUint64(nil, i*0x9e3779b97f4a7c15)
	}
	val := make([]byte, 100)
	for start := 0; start < keys; start += 1024 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for i := start; i < start+1024; i++ {
			if err := tx.Set(ctx, key(uint64(i)), val); err != nil {
				b.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			b.Fatal(err)
		}
		tx.Discard()
	}

	// Run the writers for a fixed time.
	for b.Loop() {
		before := db.Stats()
		start := time.Now()
		var commits atomic.Uint64
		var mtx sync.Mutex
		var lat []time.Duration
		var wg sync.WaitGroup
		for w := range writers {
			wg.Go(func() {
				rng := rand.New(rand.NewPCG(uint64(w), 7))
				var own []time.Duration
				for time.Since(start) < runFor {
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
					time.Sleep(hold)
					if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
						b.Error(err)
						return
					}
					tx.Discard()
					own = append(own, time.Since(began))
					commits.Add(1)
				}
				mtx.Lock()
				lat = append(lat, own...)
				mtx.Unlock()
			})
		}
		wg.Wait()

		// Report the checkpoint's rate and what the writers saw.
		secs := time.Since(start).Seconds()
		after := db.Stats()
		slices.Sort(lat)
		b.ReportMetric(float64(after.CheckpointBytes-before.CheckpointBytes)/secs/1e6, "ckptMB/s")
		b.ReportMetric(float64(after.Checkpoints-before.Checkpoints), "ckpts")
		b.ReportMetric(float64(commits.Load())/secs, "commits/s")
		b.ReportMetric(float64(after.OverlayBytes)/1e6, "endOverlayMB")
		if len(lat) != 0 {
			b.ReportMetric(float64(lat[len(lat)/2].Milliseconds()), "p50ms")
			b.ReportMetric(float64(lat[len(lat)*99/100].Milliseconds()), "p99ms")
			b.ReportMetric(float64(lat[len(lat)-1].Milliseconds()), "maxms")
		}
	}
}
