package kvtx_hidalgo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aperturerobotics/cayley/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/kv/kvtest"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// Use real transactional storage for adapter contracts. The spy supplies only
// the optional optimization and records its boundaries; no transaction or
// iterator semantics are substituted.
type batchTestStore struct {
	kvtx.Store
	batches [][]kvtx.WriteBatchEntry
	commits int
	failure error
}

func (s *batchTestStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &batchTestTx{Tx: tx, store: s}, nil
}

type batchTestTx struct {
	kvtx.Tx
	store *batchTestStore
}

func (t *batchTestTx) ApplyWriteBatch(ctx context.Context, entries []kvtx.WriteBatchEntry) error {
	snapshot := make([]kvtx.WriteBatchEntry, len(entries))
	for i, e := range entries {
		snapshot[i] = kvtx.WriteBatchEntry{Key: bytes.Clone(e.Key), Value: bytes.Clone(e.Value), Delete: e.Delete}
	}
	t.store.batches = append(t.store.batches, snapshot)
	if t.store.failure != nil {
		return t.store.failure
	}
	for _, e := range entries {
		var err error
		if e.Delete {
			err = t.Delete(ctx, e.Key)
		} else {
			err = t.Set(ctx, e.Key, e.Value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *batchTestTx) Commit(ctx context.Context) error { t.store.commits++; return t.Tx.Commit(ctx) }

func newBatchTestTx(t *testing.T) (*bufferedTx, *batchTestStore) {
	t.Helper()
	store := &batchTestStore{Store: store_kvtx_inmem.NewStore()}
	tx, err := NewKV(store).Tx(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Close() })
	return tx.(*bufferedTx), store
}

func TestBufferedKVContract(t *testing.T) {
	// Run the same upstream adapter contract suite as the unbuffered backend.
	kvtest.RunTestLocal(t, func(string) (kv.KV, error) {
		return flat.Upgrade(NewKV(&batchTestStore{Store: store_kvtx_inmem.NewStore()})), nil
	}, nil)
}

func TestBufferedWritesOwnInputAndReadTheirWrites(t *testing.T) {
	ctx := t.Context()
	tx, store := newBatchTestTx(t)
	key, value := []byte("beta"), []byte("old")
	if err := tx.Put(ctx, key, value); err != nil {
		t.Fatal(err)
	}
	clear(key)
	clear(value)
	if err := tx.Put(ctx, []byte("beta"), []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Put(ctx, []byte("empty"), nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Put(ctx, []byte("gone"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Del(ctx, []byte("gone")); err != nil {
		t.Fatal(err)
	}
	if len(store.batches) != 0 {
		t.Fatal("writes escaped before a flush boundary")
	}
	got, err := tx.Get(ctx, []byte("beta"))
	if err != nil || string(got) != "new" {
		t.Fatalf("Get: %q %v", got, err)
	}
	clear(got)
	vals, err := tx.GetBatch(ctx, []flat.Key{[]byte("beta"), []byte("gone"), []byte("empty"), []byte("absent"), nil, []byte("beta")})
	if err != nil || string(vals[0]) != "new" || vals[1] != nil || vals[2] == nil || len(vals[2]) != 0 || vals[3] != nil || vals[4] != nil || string(vals[5]) != "new" {
		t.Fatalf("GetBatch: %#v %v", vals, err)
	}
	clear(vals[0])
	if string(vals[5]) != "new" {
		t.Fatal("GetBatch aliases duplicate results")
	}
	// A scan is a visibility fence even before Commit. Later reads must also
	// find values delegated to the underlying transaction, not only the overlay.
	it := tx.Scan(ctx)
	defer it.Close()
	var keys []string
	for it.Next(ctx) {
		keys = append(keys, string(it.Key()))
	}
	if it.Err() != nil || fmt.Sprint(keys) != "[beta empty]" {
		t.Fatalf("scan: %v %v", keys, it.Err())
	}
	vals, err = tx.GetBatch(ctx, []flat.Key{[]byte("beta"), []byte("empty"), []byte("gone")})
	if err != nil || string(vals[0]) != "new" || vals[1] == nil || vals[2] != nil {
		t.Fatalf("post-flush batch: %#v %v", vals, err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 3 {
		t.Fatalf("flush batches: %#v", store.batches)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if store.commits != 1 {
		t.Fatalf("commits=%d", store.commits)
	}
	if err := tx.Put(ctx, []byte("later"), nil); !errors.Is(err, kvtx.ErrDiscarded) {
		t.Fatalf("put after commit: %v", err)
	}
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	v, ok, err := read.Get(ctx, []byte("beta"))
	if err != nil || !ok || string(v) != "new" {
		t.Fatalf("reopen: %q %v %v", v, ok, err)
	}
}

func TestBufferedWriteBoundsAndOversizedValue(t *testing.T) {
	ctx := t.Context()
	tx, store := newBatchTestTx(t)
	for i := range writeBatchMaxEntries*2 + 1 {
		if err := tx.Put(ctx, []byte(fmt.Sprintf("key-%04d", i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if len(tx.pending) > writeBatchMaxEntries || tx.bytes > writeBatchMaxBytes {
			t.Fatal("entry or byte bound exceeded")
		}
	}
	if len(store.batches) != 2 || len(store.batches[0]) != writeBatchMaxEntries || len(store.batches[1]) != writeBatchMaxEntries {
		t.Fatal("entry flush boundary differs")
	}
	// Replacement accounting does not accumulate overwritten value bytes.
	for range 20 {
		if err := tx.Put(ctx, []byte("replace"), bytes.Repeat([]byte{'x'}, 10<<10)); err != nil {
			t.Fatal(err)
		}
	}
	if tx.bytes > 12<<10 {
		t.Fatalf("replacement charge=%d", tx.bytes)
	}
	for i := range 5 {
		if err := tx.Put(ctx, []byte(fmt.Sprintf("large-%d", i)), bytes.Repeat([]byte{'y'}, 100<<10)); err != nil {
			t.Fatal(err)
		}
		if tx.bytes > writeBatchMaxBytes {
			t.Fatal("byte bound exceeded")
		}
	}
	huge := bytes.Repeat([]byte{'h'}, writeBatchMaxBytes+1)
	if err := tx.Put(ctx, []byte("oversize"), huge); err != nil {
		t.Fatal(err)
	}
	clear(huge)
	if len(tx.pending) != 0 || tx.bytes != 0 {
		t.Fatal("oversized input retained")
	}
	got, err := tx.Get(ctx, []byte("oversize"))
	if err != nil || len(got) != len(huge) || got[0] != 'h' {
		t.Fatalf("oversize: %v", err)
	}
	for _, b := range store.batches {
		charge := 0
		for _, e := range b {
			charge += batchEntryBytes(e)
		}
		if len(b) > writeBatchMaxEntries || (charge > writeBatchMaxBytes && len(b) != 1) {
			t.Fatalf("unbounded flush: %d entries %d bytes", len(b), charge)
		}
	}
}

func TestBufferedFailureAndDiscard(t *testing.T) {
	ctx := t.Context()
	t.Run("flush error is terminal", func(t *testing.T) {
		tx, store := newBatchTestTx(t)
		failure := errors.New("batch storage failure")
		if err := tx.Put(ctx, []byte("key"), []byte("value")); err != nil {
			t.Fatal(err)
		}
		store.failure = failure
		if err := tx.Commit(ctx); !errors.Is(err, failure) {
			t.Fatalf("Commit: %v", err)
		}
		store.failure = nil
		if err := tx.Commit(ctx); !errors.Is(err, failure) {
			t.Fatalf("retry: %v", err)
		}
		if err := tx.Put(ctx, []byte("tail"), nil); !errors.Is(err, failure) {
			t.Fatalf("tail: %v", err)
		}
		it := tx.Scan(ctx)
		defer it.Close()
		if it.Next(ctx) || !errors.Is(it.Err(), failure) {
			t.Fatalf("scan: %v", it.Err())
		}
		if store.commits != 0 || len(store.batches) != 1 {
			t.Fatal("error retried or committed")
		}
	})
	t.Run("close drops pending", func(t *testing.T) {
		tx, store := newBatchTestTx(t)
		if err := tx.Put(ctx, []byte("key"), []byte("value")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Close(); err != nil {
			t.Fatal(err)
		}
		if len(store.batches) != 0 || len(tx.pending) != 0 || tx.bytes != 0 {
			t.Fatal("close flushed or retained data")
		}
		if err := tx.Commit(ctx); !errors.Is(err, kvtx.ErrDiscarded) {
			t.Fatalf("closed Commit: %v", err)
		}
	})
	t.Run("cancellation does not flush", func(t *testing.T) {
		tx, store := newBatchTestTx(t)
		if err := tx.Put(ctx, []byte("key"), nil); err != nil {
			t.Fatal(err)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if err := tx.Commit(canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
		if len(store.batches) != 0 || store.commits != 0 {
			t.Fatal("canceled operation touched backend")
		}
		_ = tx.Close()
	})
}

func TestOnlyCapableWritableTransactionsBuffer(t *testing.T) {
	ctx := t.Context()
	scalar := store_kvtx_inmem.NewStore()
	for _, store := range []kvtx.Store{scalar, &batchTestStore{Store: scalar}} {
		for _, write := range []bool{false, true} {
			tx, err := NewKV(store).Tx(ctx, write)
			if err != nil {
				t.Fatal(err)
			}
			_, buffered := tx.(*bufferedTx)
			_, capable := store.(*batchTestStore)
			if buffered != (capable && write) {
				t.Fatalf("capability %T write=%v buffered=%v", store, write, buffered)
			}
			_ = tx.Close()
		}
	}
}

func TestBufferedLazyScanAndResetSeePrecedingWrites(t *testing.T) {
	ctx := t.Context()
	tx, _ := newBatchTestTx(t)
	it := tx.Scan(ctx)
	defer it.Close()
	// The scalar adapter creates the underlying iterator lazily at first Next,
	// not at Scan. Buffering must preserve that observable boundary.
	if err := tx.Put(ctx, []byte("a"), []byte("first")); err != nil {
		t.Fatal(err)
	}
	if !it.Next(ctx) || string(it.Key()) != "a" || string(it.Val()) != "first" {
		t.Fatalf("first Next missed queued write: key=%q value=%q err=%v", it.Key(), it.Val(), it.Err())
	}
	if err := tx.Put(ctx, []byte("b"), []byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Del(ctx, []byte("a")); err != nil {
		t.Fatal(err)
	}
	it.Reset()
	if !it.Next(ctx) || string(it.Key()) != "b" || string(it.Val()) != "second" {
		t.Fatalf("Reset missed preceding writes: key=%q value=%q err=%v", it.Key(), it.Val(), it.Err())
	}
	if it.Next(ctx) || it.Err() != nil {
		t.Fatalf("unexpected trailing result: %q %v", it.Key(), it.Err())
	}
}

func TestBufferedLazyScanKeepsFlushErrors(t *testing.T) {
	ctx := t.Context()
	tx, store := newBatchTestTx(t)
	it := tx.Scan(ctx)
	defer it.Close()
	if err := tx.Put(ctx, []byte("key"), nil); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("failed while starting scan")
	store.failure = failure
	if it.Next(ctx) || !errors.Is(it.Err(), failure) {
		t.Fatalf("first Next: %v", it.Err())
	}
	store.failure = nil
	it.Reset()
	if it.Next(ctx) || !errors.Is(it.Err(), failure) {
		t.Fatalf("reset hid failed flush: %v", it.Err())
	}
	if len(store.batches) != 1 || store.commits != 0 {
		t.Fatal("failed batch retried or committed")
	}
}
