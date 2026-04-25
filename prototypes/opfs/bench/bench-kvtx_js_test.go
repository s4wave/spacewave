//go:build js

package bench

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	opfs_store "github.com/s4wave/spacewave/prototypes/opfs/store"
)

func BenchmarkOPFSKVWrite(b *testing.B) {
	for _, size := range benchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			s, cleanup := setupKVStore(b)
			defer cleanup()

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kv/", b.N)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tx, err := s.NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err)
				}
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					tx.Discard()
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkOPFSKVWriteSingleTx(b *testing.B) {
	for _, size := range benchSizes {
		size := size
		b.Run(sizeLabel(size), func(b *testing.B) {
			s, cleanup := setupKVStore(b)
			defer cleanup()

			ctx := context.Background()
			data := bytes.Repeat([]byte{0x61}, size)
			keys := buildKeys("bench/kvs/", b.N)

			// Use a raw write tx (no txcache).
			tx, err := s.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := tx.Set(ctx, keys[i], data); err != nil {
					tx.Discard()
					b.Fatal(err)
				}
			}
			b.StopTimer()

			if err := tx.Commit(ctx); err != nil {
				tx.Discard()
				b.Fatal(err)
			}
		})
	}
}

func setupKVStore(b *testing.B) (*opfs_store.Store, func()) {
	b.Helper()
	name := "bench-kv-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	s, err := opfs_store.Open(name)
	if err != nil {
		b.Fatal(err)
	}
	return s, func() {
		s.Close()
	}
}

func buildKeys(prefix string, n int) [][]byte {
	keys := make([][]byte, n)
	for i := range n {
		key := make([]byte, 0, len(prefix)+12)
		key = append(key, prefix...)
		key = strconv.AppendInt(key, int64(i), 10)
		keys[i] = key
	}
	return keys
}
