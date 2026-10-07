//go:build darwin || linux || windows

package s4db_test

import (
	"encoding/binary"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
)

// BenchmarkIndexCacheMissBatch reads random batches from an index larger than
// the default 128 MiB decoded cache. Setup and checkpointing are not timed.
func BenchmarkIndexCacheMissBatch(b *testing.B) {
	// Build a 172 MiB leaf index through ordinary write transactions.
	ctx := b.Context()
	path := filepath.Join(b.TempDir(), "index.s4wave")
	db, err := s4db.Open(path, s4db.Options{InlineMax: 1024, RelocateBudget: -1})
	if err != nil {
		b.Fatal(err)
	}
	keys := make([][]byte, 220000)
	value := make([]byte, 768)
	for start := 0; start < len(keys); start += 4096 {
		// Commit one ordered key range without a flush per range.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for i := start; i < min(start+4096, len(keys)); i++ {
			keys[i] = make([]byte, 32)
			binary.BigEndian.PutUint64(keys[i], uint64(i))
			if err := tx.Set(ctx, keys[i], value); err != nil {
				b.Fatal(err)
			}
		}
		if err := kvtx.CommitOrdered(ctx, tx); err != nil {
			b.Fatal(err)
		}
		tx.Discard()
	}
	if err := db.Close(); err != nil {
		b.Fatal(err)
	}

	// Reopen the checkpoint so every read exercises the on-disk index.
	db, err = s4db.Open(path, s4db.Options{RelocateBudget: -1})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Error(err)
		}
	})
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Discard()

	// Generate a repeatable random batch that revisits the whole index.
	rng := rand.New(rand.NewPCG(1, 2))
	batch := make([][]byte, 65536)
	for i := range batch {
		batch[i] = keys[rng.IntN(len(keys))]
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		values, found, err := kvtx.GetBatch(ctx, tx, batch)
		if err != nil {
			b.Fatal(err)
		}
		for i := range values {
			if !found[i] || len(values[i]) != len(value) {
				b.Fatal("batch lost an indexed value")
			}
		}
	}
	b.ReportMetric(float64(len(batch)), "keys/op")
	b.ReportMetric(float64(44000*4096)/(1<<20), "leaf-MiB")
}
