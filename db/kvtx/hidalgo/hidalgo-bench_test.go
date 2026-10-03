package kvtx_hidalgo

import (
	"context"
	"strconv"
	"testing"

	"github.com/aperturerobotics/cayley/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/kv/options"
	badger "github.com/dgraph-io/badger/v4"
	store_kvtx_badger "github.com/s4wave/spacewave/db/store/kvtx/badger"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

func BenchmarkTxScanPrefixEarlyStop(b *testing.B) {
	b.Run("inmem", func(b *testing.B) {
		benchmarkTxScanPrefixEarlyStop(b, NewKV(store_kvtx_inmem.NewStore()))
	})
	b.Run("badger", func(b *testing.B) {
		store, err := store_kvtx_badger.Open(badger.DefaultOptions(b.TempDir()).WithLogger(nil))
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() {
			if err := store.GetDB().Close(); err != nil {
				b.Error(err)
			}
		})
		benchmarkTxScanPrefixEarlyStop(b, NewKV(store))
	})
}

func benchmarkTxScanPrefixEarlyStop(b *testing.B, db *KV) {
	// Open a write transaction for the benchmark prefix.
	ctx := context.Background()
	prefix := kv.Key{[]byte("index"), []byte("subject")}
	tx, err := db.Tx(ctx, true)
	if err != nil {
		b.Fatal(err)
	}

	// Populate and commit the benchmark prefix entries.
	for i := range 16384 {
		key := kv.Key{[]byte("index"), []byte("subject"), []byte(strconv.Itoa(i))}
		if err := tx.Put(ctx, flat.KeyEscape(key), []byte("value")); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		b.Fatal(err)
	}
	if err := tx.Close(); err != nil {
		b.Fatal(err)
	}

	// Measure opening and closing a prefix scan after its first result.
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// Open a read transaction for one measured prefix scan.
		tx, err := db.Tx(ctx, false)
		if err != nil {
			b.Fatal(err)
		}

		// Start the prefix scan and require its first entry.
		it := tx.Scan(ctx, options.WithPrefixKV(prefix))
		if !it.Next(ctx) {
			if err := it.Close(); err != nil {
				b.Fatal(err)
			}
			if err := tx.Close(); err != nil {
				b.Fatal(err)
			}
			b.Fatalf("expected item, err=%v", it.Err())
		}

		// Close the measured iterator and read transaction.
		if err := it.Close(); err != nil {
			if closeErr := tx.Close(); closeErr != nil {
				b.Fatal(closeErr)
			}
			b.Fatal(err)
		}
		if err := tx.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
