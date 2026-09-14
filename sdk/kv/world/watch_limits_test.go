package s4wave_kv_world_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_client "github.com/s4wave/spacewave/db/kvtx/rpc/client"
	kvtx_rpc_server "github.com/s4wave/spacewave/db/kvtx/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// openBoundedWorldBackedStore opens the concrete WorldBackedStore for an
// existing kv/store object so the test can reach WatchPrefixBounded.
func openBoundedWorldBackedStore(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
) (*s4wave_kv_world.WorldBackedStore, func()) {
	t.Helper()
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("MustGetObject(%s): %v", objectKey, err)
	}
	var store *s4wave_kv_world.WorldBackedStore
	if err := obj.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		var err error
		store, err = s4wave_kv_world.NewWorldBackedStore(ctx, logrus.NewEntry(logrus.New()), root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		t.Fatalf("AccessWorldState(%s): %v", objectKey, err)
	}
	return store, store.Close
}

// expectWatchEntries requires one received snapshot to equal the wanted pairs in order.
func expectWatchEntries(t *testing.T, entries []kvtx.WatchEntry, want []kvtx.WatchEntry) {
	t.Helper()
	if len(entries) != len(want) {
		t.Fatalf("snapshot has %d entries, want %d: %v", len(entries), len(want), entries)
	}
	for i := range want {
		if string(entries[i].Key) != string(want[i].Key) || string(entries[i].Value) != string(want[i].Value) {
			t.Fatalf("snapshot[%d] = %q=%q, want %q=%q", i, entries[i].Key, entries[i].Value, want[i].Key, want[i].Value)
		}
	}
}

// commitKvSet commits one set into the store and discards the transaction.
func commitKvSet(t *testing.T, ctx context.Context, store kvtx.Store, key, value string) {
	t.Helper()
	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, []byte(key), []byte(value)); err != nil {
		writeTx.Discard()
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Discard()
		t.Fatal(err)
	}
	writeTx.Discard()
}

// TestWatchPrefixBoundedRejectsOverLimitSnapshotWithoutCallback proves an
// initial snapshot past either limit returns ErrWatchLimit with no callback.
func TestWatchPrefixBoundedRejectsOverLimitSnapshotWithoutCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/limit-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)
	store, cleanup := openBoundedWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Commit three records so the initial snapshot is over every limit.
	commitKvSet(t, ctx, store, "a", "one")
	commitKvSet(t, ctx, store, "b", "two")
	commitKvSet(t, ctx, store, "c", "three")

	// Record-count limit below the snapshot size must fail with no callback.
	calls := 0
	err = store.WatchPrefixBounded(ctx, []byte(""), kvtx.WatchLimits{MaxRecords: 2}, func(entries []kvtx.WatchEntry) error {
		calls++
		return nil
	})
	if !errors.Is(err, kvtx.ErrWatchLimit) {
		t.Fatalf("MaxRecords limit error = %v, want ErrWatchLimit", err)
	}
	if calls != 0 {
		t.Fatalf("callback ran %d times for a rejected snapshot", calls)
	}

	// Byte limit below the total key and value bytes must fail the same way.
	err = store.WatchPrefixBounded(ctx, []byte(""), kvtx.WatchLimits{MaxBytes: 4}, func(entries []kvtx.WatchEntry) error {
		calls++
		return nil
	})
	if !errors.Is(err, kvtx.ErrWatchLimit) {
		t.Fatalf("MaxBytes limit error = %v, want ErrWatchLimit", err)
	}
	if calls != 0 {
		t.Fatalf("callback ran %d times for a rejected snapshot", calls)
	}
}

// TestWatchPrefixBoundedExactBoundarySnapshotOrdered proves a snapshot exactly
// at both limits is delivered once in stable sorted key order and one byte
// under the bound is rejected without a callback.
func TestWatchPrefixBoundedExactBoundarySnapshotOrdered(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/boundary-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)
	store, cleanup := openBoundedWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	commitKvSet(t, ctx, store, "a", "one")
	commitKvSet(t, ctx, store, "b", "two")
	commitKvSet(t, ctx, store, "c", "three")

	// One byte under the total key plus value byte count must be rejected
	// without any callback.
	calls := 0
	err = store.WatchPrefixBounded(ctx, []byte(""), kvtx.WatchLimits{MaxRecords: 3, MaxBytes: 13}, func(entries []kvtx.WatchEntry) error {
		calls++
		return nil
	})
	if !errors.Is(err, kvtx.ErrWatchLimit) {
		t.Fatalf("13-byte limit error = %v, want ErrWatchLimit", err)
	}
	if calls != 0 {
		t.Fatalf("callback ran %d times for a rejected snapshot", calls)
	}

	// Exactly three records and 14 total key plus value bytes is delivered once
	// in sorted order. The callback returns ErrWatchLimit so the synchronous
	// watch ends after the single snapshot.
	var got []kvtx.WatchEntry
	err = store.WatchPrefixBounded(ctx, []byte(""), kvtx.WatchLimits{MaxRecords: 3, MaxBytes: 14}, func(entries []kvtx.WatchEntry) error {
		got = entries
		return kvtx.ErrWatchLimit
	})
	if !errors.Is(err, kvtx.ErrWatchLimit) {
		t.Fatalf("exact boundary watch error = %v, want ErrWatchLimit from the callback", err)
	}
	expectWatchEntries(t, got, []kvtx.WatchEntry{
		{Key: []byte("a"), Value: []byte("one")},
		{Key: []byte("b"), Value: []byte("two")},
		{Key: []byte("c"), Value: []byte("three")},
	})
}

// TestBoundedWatchLimitEndsOnlyThatWatch proves a committed growth past the
// bound ends only the bounded watch while a separate prefix watch keeps
// receiving valid snapshots.
func TestBoundedWatchLimitEndsOnlyThatWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/growth-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)
	store, cleanup := openBoundedWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Start a bounded watch over the whole store with a one-record bound.
	boundedErr := make(chan error, 1)
	boundedDone := make(chan struct{})
	boundedCtx, cancelBounded := context.WithCancel(ctx)
	defer cancelBounded()
	go func() {
		defer close(boundedDone)
		boundedErr <- store.WatchPrefixBounded(boundedCtx, []byte(""), kvtx.WatchLimits{MaxRecords: 1}, func(entries []kvtx.WatchEntry) error {
			return nil
		})
	}()

	// Start a separate unbounded watch over the g/ prefix.
	unboundedErr := make(chan error, 1)
	unboundedDone := make(chan struct{})
	unboundedCtx, cancelUnbounded := context.WithCancel(ctx)
	defer cancelUnbounded()
	unboundedSnapshots := make(chan []kvtx.WatchEntry, 4)
	go func() {
		defer close(unboundedDone)
		unboundedErr <- store.WatchPrefix(unboundedCtx, []byte("g/"), func(entries []kvtx.WatchEntry) error {
			select {
			case unboundedSnapshots <- entries:
				return nil
			case <-unboundedCtx.Done():
				return unboundedCtx.Err()
			}
		})
	}()

	// Establish both initial snapshots before any write: the bounded watch sees
	// the empty store within its bound and the unbounded watch sees an empty
	// g/ prefix snapshot.
	select {
	case s := <-unboundedSnapshots:
		expectWatchEntries(t, s, nil)
	case <-ctx.Done():
		t.Fatal("unbounded watch did not deliver its initial snapshot")
	}
	select {
	case <-boundedDone:
		t.Fatal("bounded watch ended before any write")
	case <-time.After(50 * time.Millisecond):
	}

	// Commit the first record: the unbounded watch delivers its snapshot.
	commitKvSet(t, ctx, store, "g/first", "one")
	select {
	case s := <-unboundedSnapshots:
		expectWatchEntries(t, s, []kvtx.WatchEntry{{Key: []byte("g/first"), Value: []byte("one")}})
	case <-ctx.Done():
		t.Fatal("unbounded watch did not deliver the initial snapshot")
	}

	// Commit a second record: the bounded watch must end with ErrWatchLimit
	// while the unbounded watch delivers the changed snapshot.
	commitKvSet(t, ctx, store, "g/second", "two")
	select {
	case s := <-unboundedSnapshots:
		expectWatchEntries(t, s, []kvtx.WatchEntry{
			{Key: []byte("g/first"), Value: []byte("one")},
			{Key: []byte("g/second"), Value: []byte("two")},
		})
	case <-ctx.Done():
		t.Fatal("unbounded watch did not deliver the changed snapshot")
	}
	select {
	case err := <-boundedErr:
		if !errors.Is(err, kvtx.ErrWatchLimit) {
			t.Fatalf("bounded watch error = %v, want ErrWatchLimit", err)
		}
	case <-ctx.Done():
		t.Fatal("bounded watch did not end after exceeding its limit")
	}

	// The unbounded watch must still be running; commit a third record.
	commitKvSet(t, ctx, store, "g/third", "three")
	select {
	case s := <-unboundedSnapshots:
		expectWatchEntries(t, s, []kvtx.WatchEntry{
			{Key: []byte("g/first"), Value: []byte("one")},
			{Key: []byte("g/second"), Value: []byte("two")},
			{Key: []byte("g/third"), Value: []byte("three")},
		})
	case <-ctx.Done():
		t.Fatal("unbounded watch stopped after the bounded watch failed")
	}
	cancelUnbounded()
	<-unboundedDone
	if err := <-unboundedErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unbounded watch ended with %v", err)
	}
}

// TestBoundedWatchOverRPCMapsLimitError proves the RPC server maps
// ErrWatchLimit into a final response with LimitExceeded set and the client
// Store maps that response back to ErrWatchLimit.
func TestBoundedWatchOverRPCMapsLimitError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	objectKey := "kv/rpc-limit-store"
	createKvStoreObject(t, ctx, tb.WorldState, objectKey, true)
	store, cleanup := openBoundedWorldBackedStore(t, ctx, tb.WorldState, objectKey)
	defer cleanup()

	// Commit two records so a one-record bound exceeds the initial snapshot.
	commitKvSet(t, ctx, store, "r/first", "one")
	commitKvSet(t, ctx, store, "r/second", "two")

	// Serve the store over an in-memory RPC pipe.
	mux := srpc.NewMux()
	if err := kvtx_rpc.SRPCRegisterKvtx(mux, kvtx_rpc_server.NewStore(store)); err != nil {
		t.Fatal(err)
	}
	client := kvtx_rpc_client.NewStore(kvtx_rpc.NewSRPCKvtxClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))

	// A bounded client watch over the over-limit snapshot returns the sentinel.
	calls := 0
	err = client.WatchPrefixBounded(ctx, []byte(""), kvtx.WatchLimits{MaxRecords: 1}, func(entries []kvtx.WatchEntry) error {
		calls++
		return nil
	})
	if !errors.Is(err, kvtx.ErrWatchLimit) {
		t.Fatalf("bounded RPC watch error = %v, want ErrWatchLimit", err)
	}
	if calls != 0 {
		t.Fatalf("callback ran %d times for a rejected snapshot", calls)
	}

	// The unbounded client path stays compatible.
	readTx, err := client.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	found, err := readTx.Exists(ctx, []byte("r/first"))
	if err != nil || !found {
		t.Fatalf("unbounded RPC path broken: found=%v err=%v", found, err)
	}
}
