//go:build darwin || linux

package s4db

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"path/filepath"
	"testing"
)

// benchKeys is the number of keys the benchmarks load.
const benchKeys = 200000

// loadBench fills a database with benchKeys small values in ordered commits
// of 128 keys and returns it with its keys.
func loadBench(b *testing.B) (*DB, [][]byte) {
	// Open a fresh file.
	b.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(b.TempDir(), "bench.s4wave"), Options{})
	if err != nil {
		b.Fatal(err)
	}

	// Commit random keys with 150-byte values.
	rng := rand.New(rand.NewPCG(1, 2))
	keys := make([][]byte, benchKeys)
	val := make([]byte, 150)
	for i := 0; i < benchKeys; i += 128 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			b.Fatal(err)
		}
		for j := i; j < min(i+128, benchKeys); j++ {
			keys[j] = binary.BigEndian.AppendUint64(nil, rng.Uint64())
			if err := tx.Set(ctx, keys[j], val); err != nil {
				b.Fatal(err)
			}
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			b.Fatal(err)
		}
	}
	return db, keys
}

// BenchmarkGet measures point reads, each in its own read transaction, from
// parallel goroutines.
func BenchmarkGet(b *testing.B) {
	// Load and warm the database.
	db, keys := loadBench(b)
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
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewPCG(rand.Uint64(), 0))
		for pb.Next() {
			get(keys[rng.IntN(len(keys))])
		}
	})
}

// BenchmarkFill measures ordered commits of 128 small values.
func BenchmarkFill(b *testing.B) {
	for b.Loop() {
		db, _ := loadBench(b)
		_ = db.Close()
	}
}
