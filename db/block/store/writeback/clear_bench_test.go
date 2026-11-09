//go:build !js

package block_store_writeback

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// clearBenchQueued is the markers the clear benchmarks queue: their marked
// and queue keys make B-trees of thousands of leaves, past the page cache
// clearBenchPragmas sets, so a clear reads and writes leaves as it does on a
// large store.
const clearBenchQueued = 100_000

// clearBenchPragmas keeps SQLite's page cache to 256 KiB, a small part of the
// marker trees.
var clearBenchPragmas = store_kvtx_sqlite.Pragmas{CacheSize: 64, WithoutRowID: true}

// newClearBenchStore opens a Store over a Volume on a SQLite file with queued
// markers, enabled, and returns it with the queue in write order.
func newClearBenchStore(b *testing.B, queued int) (*Store, []pendingBlock) {
	b.Helper()
	ctx := b.Context()
	kv, err := store_kvtx_sqlite.OpenWithPragmas(ctx, filepath.Join(b.TempDir(), "clear.db"), "kv", clearBenchPragmas)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = kv.Close() })
	vol, err := volume_kvtx.NewVolume(ctx, "test/writeback", store_kvkey.NewDefaultKVKey(), kv, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(release)
	writeTx := func(ctx context.Context) (kvtx.Tx, error) { return markers.NewTransaction(ctx, true) }
	readTx := func(ctx context.Context) (kvtx.Tx, error) { return markers.NewTransaction(ctx, false) }

	// Queue the markers as a tool that copies blocks in does, before the
	// store first opens, in write order a microsecond apart.
	start := time.Now().Add(-time.Hour)
	for from := 0; from < queued; from += 10_000 {
		err := kvtx.RunTransaction(ctx, true, writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			for i := from; i < min(from+10_000, queued); i++ {
				h, err := hash.Sum(hash.HashType_HashType_SHA256, fmt.Appendf(nil, "clear-bench-%d", i))
				if err != nil {
					return err
				}
				m := Mark{Hash: h, Size: 3300, At: start.Add(time.Duration(i) * time.Microsecond)}
				marked, queue := MarkerKeys(m)
				if err := tx.Set(ctx, marked, nil); err != nil {
					return err
				}
				if err := tx.Set(ctx, queue, MarkerQueueValue(m)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	s, err := NewStore(ctx, ownedStore{vol}, vol, markers, testMarkersID, true)
	if err != nil {
		b.Fatal(err)
	}

	// Read the queue in write order, as an upload batch holds it.
	entries := make([]pendingBlock, 0, queued)
	err = kvtx.RunTransaction(ctx, false, readTx, func(ctx context.Context, tx kvtx.Tx) error {
		return scanQueue(ctx, tx, nil, func(item queueItem) error {
			entry, err := item.pending()
			entries = append(entries, entry)
			return err
		})
	})
	if err != nil || len(entries) != queued {
		b.Fatalf("queue reads %d of %d entries: %v", len(entries), queued, err)
	}
	return s, entries
}

// waitCounted waits for the walk that starts the queue's count at the
// store's open, so its write transactions do not run beside the clear's.
func waitCounted(b *testing.B, s *Store) {
	b.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; {
		status, _ := s.GetStatus()
		if !status.Counting {
			return
		}
		if time.Now().After(deadline) {
			b.Fatal("the queue's count did not finish within 10 s")
		}
		time.Sleep(time.Millisecond)
	}
}

// withClearBatch sets the clear bounds for a benchmark and restores the
// defaults after it.
func withClearBatch(b *testing.B, batch ClearBatch) {
	b.Helper()
	SetClearBatch(batch)
	b.Cleanup(func() { SetClearBatch(ClearBatch{}) })
}

// BenchmarkClearPerBlock measures what one marker's clear costs on SQLite at
// a range of transaction sizes: each transaction removes exactly the batch's
// blocks, as a hold of 500 ms admits on this disk. A transaction's fixed cost
// is its begin, the epoch and count reads, the queue head scan and the
// commit; the rest grows with its blocks. It reports the writer's hold a
// block, the commit included, and the blocks a second of the whole clear,
// the prefetch that holds no lock included. Each size clears its own part of
// one queue.
//
//	go test -run '^$' -bench BenchmarkClearPerBlock -benchtime 1x
func BenchmarkClearPerBlock(b *testing.B) {
	const cleared = 16384
	s, entries := newClearBenchStore(b, clearBenchQueued)
	waitCounted(b, s)
	w := &contendedWrites{Store: s.markers}
	s.markers = w
	for _, blocks := range []int{16, 45, 128, 512, 2048, 8192} {
		b.Run(fmt.Sprintf("blocks=%d", blocks), func(b *testing.B) {
			withClearBatch(b, ClearBatch{Hold: clearHoldMax, MaxBlocks: blocks})
			batch := entries[:cleared]
			entries = entries[cleared:]
			w.held.Store(0)
			b.ResetTimer()
			start := time.Now()
			if err := s.clear(b.Context(), batch); err != nil {
				b.Fatal(err)
			}
			elapsed := time.Since(start)
			b.StopTimer()
			b.ReportMetric(float64(time.Duration(w.held.Load()).Microseconds())/cleared, "held-us/block")
			b.ReportMetric(cleared/elapsed.Seconds(), "blocks/s")
		})
	}
}

// contendedWrites stands in for the production store's writer: each write
// transaction on the markers first waits turnWait for its turn, as a
// background writer did behind eviction, checkpoints and player commits, and
// each Get under it costs getCost more, as a disk under load made each
// cleared block cost: a cleared block reads its queue entry and whether its
// eviction entry exists, two reads. It records the longest a transaction held the writer,
// from its turn to its commit.
type contendedWrites struct {
	kvtx.Store
	turnWait, getCost time.Duration
	turns             atomic.Int32
	longest, held     atomic.Int64
}

// NewTransaction waits for the turn, then opens the transaction.
func (w *contendedWrites) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if !write {
		return w.Store.NewTransaction(ctx, write)
	}
	if w.turnWait > 0 {
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-time.After(w.turnWait):
		}
	}
	tx, err := w.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	w.turns.Add(1)
	return &contendedTx{Tx: tx, w: w, start: time.Now()}, nil
}

// contendedTx costs each Get getCost and records its hold.
type contendedTx struct {
	kvtx.Tx
	w     *contendedWrites
	start time.Time
	once  sync.Once
}

// Get reads the key after the disk's cost.
func (t *contendedTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.spin()
	return t.Tx.Get(ctx, key)
}

// Exists reads whether the key exists after the disk's cost.
func (t *contendedTx) Exists(ctx context.Context, key []byte) (bool, error) {
	t.spin()
	return t.Tx.Exists(ctx, key)
}

// spin spends the disk's cost of a read, which a sleep would overshoot at
// this scale.
func (t *contendedTx) spin() {
	if t.w.getCost > 0 {
		for start := time.Now(); time.Since(start) < t.w.getCost; {
		}
	}
}

// Commit commits and records the hold, the commit included.
func (t *contendedTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	t.record()
	return err
}

// Discard discards and records the hold.
func (t *contendedTx) Discard() {
	t.Tx.Discard()
	t.record()
}

// record keeps the longest hold.
func (t *contendedTx) record() {
	t.once.Do(func() {
		held := int64(time.Since(t.start))
		t.w.held.Add(held)
		for {
			prev := t.w.longest.Load()
			if held <= prev || t.w.longest.CompareAndSwap(prev, held) {
				return
			}
		}
	})
}

// BenchmarkClearContended measures the clear's rate against be-7's writer on
// 2026-11-09: each turn waited 0.28 s, and a 20 ms hold cleared about 45
// blocks, so each block cost about 0.44 ms under the writer, two reads of
// 0.22 ms. The holds run
// from that 20 ms to 250 ms; it reports the blocks cleared a second, the
// blocks a turn and the longest hold, the commit included.
//
//	go test -run '^$' -bench BenchmarkClearContended -benchtime 1x
func BenchmarkClearContended(b *testing.B) {
	for _, hold := range []time.Duration{20, 100, 150, 250} {
		hold *= time.Millisecond
		b.Run(fmt.Sprintf("hold=%v", hold), func(b *testing.B) {
			s, entries := newClearBenchStore(b, 20_000)
			waitCounted(b, s)
			withClearBatch(b, ClearBatch{Hold: hold})
			w := &contendedWrites{Store: s.markers, turnWait: 280 * time.Millisecond, getCost: 220 * time.Microsecond}
			s.markers = w

			// Clear about six turns' worth at the hold.
			cleared := min(len(entries), 6*int(hold/(2*w.getCost)))
			b.ResetTimer()
			start := time.Now()
			if err := s.clear(b.Context(), entries[:cleared]); err != nil {
				b.Fatal(err)
			}
			elapsed := time.Since(start)
			b.StopTimer()
			b.ReportMetric(float64(cleared)/elapsed.Seconds(), "blocks/s")
			b.ReportMetric(float64(cleared)/float64(w.turns.Load()), "blocks/turn")
			b.ReportMetric(float64(time.Duration(w.longest.Load()).Microseconds())/1000, "max-hold-ms")
		})
	}
}
