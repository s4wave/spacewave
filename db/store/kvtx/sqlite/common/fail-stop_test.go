//go:build !js

package common_test

import (
	"context"
	"errors"
	"os"
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

// TestFailedWALFlushStops holds a commit's WAL flush, fails it, and checks the
// store then refuses every read, commit, flush and checkpoint. COMMIT shows the
// write in the WAL before the flush, so after the failure a read would find
// the failed write, which the drive may not hold, and take it for stored, and a
// checkpoint would copy it into the database file. Close must not checkpoint
// either, so the database file alone still lacks the failed write.
func TestFailedWALFlushStops(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "fail.db")
	db, err := store_kvtx_sqlite.Open(ctx, path, "kv")
	if err != nil {
		t.Fatal(err)
	}

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
	set := func(key string) (kvtx.Tx, error) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return nil, err
		}
		if err := tx.Set(ctx, []byte(key), []byte("v-"+key)); err != nil {
			tx.Discard()
			return nil, err
		}
		return tx, nil
	}
	put := func(key string) error {
		tx, err := set(key)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := put("kept"); err != nil {
		t.Fatal(err)
	}

	// Hold the flush of commit b, and open a write transaction behind it
	// that commits only after the flush fails.
	armed.Store(true)
	done := make(chan error, 1)
	go func() { done <- put("b") }()
	<-entered
	pre, err := set("pre")
	if err != nil {
		t.Fatal(err)
	}

	// Fail the flush. The commit reports it, and the store refuses
	// everything after it without flushing again.
	release <- errWALFault
	if err := <-done; !errors.Is(err, errWALFault) || !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit with a failed flush returned %v", err)
	}
	calls := flushes.Load()
	if err := pre.Commit(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit of a transaction opened before a failed flush returned %v", err)
	}
	if _, err := db.NewTransaction(ctx, false); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("read after a failed flush returned %v", err)
	}
	if err := put("after"); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit after a failed flush returned %v", err)
	}
	if err := db.Sync(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("Sync after a failed flush returned %v", err)
	}
	if got := flushes.Load(); got != calls {
		t.Fatalf("failed store flushed %d more times", got-calls)
	}

	// The checkpointer stops at its next pass instead of copying the WAL.
	common.WakeCheckpoint(db)
	select {
	case <-common.CheckpointsDone(db):
	case <-time.After(10 * time.Second):
		t.Fatal("checkpointer still runs after a failed flush")
	}

	// Close, then read copies of the files. The database file alone must
	// lack the failed write and the refused one, since no checkpoint may
	// copy the WAL after the failure. With its WAL it must still lack the
	// refused write, which COMMIT never wrote.
	_ = db.Close()
	assertCopy(t, path, false, "", "b", "pre")
	assertCopy(t, path, true, "kept", "pre")
}

// assertCopy copies the database at path, with its WAL when withWAL is set,
// opens the copy and fails t unless it holds present, when set, and none of
// absent.
func assertCopy(t *testing.T, path string, withWAL bool, present string, absent ...string) {
	t.Helper()
	ctx := context.Background()
	copyPath := filepath.Join(t.TempDir(), "copy.db")
	files := map[string]string{path: copyPath}
	if withWAL {
		files[path+"-wal"] = copyPath + "-wal"
	}
	for from, to := range files {
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cp, err := store_kvtx_sqlite.Open(ctx, copyPath, "kv")
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	rtx, err := cp.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	if present != "" {
		if _, found, err := rtx.Get(ctx, []byte(present)); err != nil || !found {
			t.Fatalf("copy of the database (WAL %v) lacks %q: %v", withWAL, present, err)
		}
	}
	for _, key := range absent {
		if _, found, err := rtx.Get(ctx, []byte(key)); err != nil {
			t.Fatal(err)
		} else if found {
			t.Fatalf("copy of the database (WAL %v) holds %q after a failed flush and Close", withWAL, key)
		}
	}
}
