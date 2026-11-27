//go:build !js

package common_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// errWALFault is the WAL flush failure TestFailedWALFlushStops injects.
var errWALFault = errors.New("injected WAL flush failure")

// TestFailedWALFlushStops holds a durable commit's WAL flush, cancels the
// commit, then fails the flush. COMMIT shows the write in the WAL before the
// flush, so the commit must wait for the flush despite the cancellation, and
// once the flush fails the store must refuse every read, commit and flush
// without calling the flush again: a read would find the failed write, which
// the drive may not hold, and take it for stored.
func TestFailedWALFlushStops(t *testing.T) {
	ctx := context.Background()
	db, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "fail.db"), "kv")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Route the WAL flushes through a gate that can hold one and fail it.
	var armed atomic.Bool
	var flushes atomic.Int64
	entered, release := make(chan struct{}), make(chan error)
	common.WrapWALFlush(db, func(flush func() error) error {
		flushes.Add(1)
		if armed.Swap(false) {
			entered <- struct{}{}
			if err := <-release; err != nil {
				return err
			}
		}
		return flush()
	})
	put := func(ctx context.Context, key string) error {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, []byte(key), []byte("v-"+key)); err != nil {
			tx.Discard()
			return err
		}
		return tx.Commit(ctx)
	}
	if err := put(ctx, "kept"); err != nil {
		t.Fatal(err)
	}

	// Hold the flush of commit b and cancel b. It must not return before the
	// flush ends.
	armed.Store(true)
	bctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- put(bctx, "b") }()
	<-entered
	cancel()
	select {
	case err := <-done:
		t.Fatalf("cancelled commit returned %v before its flush ended", err)
	case <-time.After(100 * time.Millisecond):
	}

	// Fail the flush. The commit reports it, and the store refuses
	// everything after it without flushing again.
	release <- errWALFault
	if err := <-done; !errors.Is(err, errWALFault) || !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit with a failed flush returned %v", err)
	}
	calls := flushes.Load()
	if _, err := db.NewTransaction(ctx, false); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("read after a failed flush returned %v", err)
	}
	if err := put(ctx, "after"); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit after a failed flush returned %v", err)
	}
	if err := db.Sync(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("Sync after a failed flush returned %v", err)
	}
	if got := flushes.Load(); got != calls {
		t.Fatalf("failed store flushed %d more times", got-calls)
	}
}
