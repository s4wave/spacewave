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
// either, so the database file alone still lacks the failed write, and no
// Store may open the file again in the process.
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

	// A Store opened on the file in this process would read the failed
	// write: the page cache may hold its WAL frames, and the pool left open
	// keeps the WAL index that maps them. The open must be refused.
	if reopened, err := store_kvtx_sqlite.Open(ctx, path, "kv"); !errors.Is(err, kvtx.ErrFlushFailed) {
		if err == nil {
			_ = reopened.Close()
		}
		t.Fatalf("reopen in the process after a failed flush returned %v", err)
	}
}

// failOnce routes the WAL flushes of db through a wrap whose next flush,
// once armed, fails with errWALFault.
func failOnce(db any) *atomic.Bool {
	armed := new(atomic.Bool)
	common.WrapWALFlush(db, func(flush func() error) error {
		if armed.Swap(false) {
			return errWALFault
		}
		return flush()
	})
	return armed
}

// putKey commits key on db in ctx's commit mode.
func putKey(ctx context.Context, db kvtx.Store, key string) error {
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

// TestFailedFlushStopsOtherStore fails a WAL flush in Store a while Store b
// is open on the same file. b must refuse from then on: its reads would find
// a's failed write in the shared WAL and take it for stored, its commits
// would land after the frames the flush lost, and its own flush of the file
// could succeed without them, since Linux reports a writeback error once per
// open file and may mark the lost pages clean. Each case refuses one path
// first, since the first refusal records the failure in b for the rest. An
// ordered commit flushes nothing after its COMMIT, so only the refusal
// before COMMIT keeps its write out of the WAL.
func TestFailedFlushStopsOtherStore(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		// first refuses the case's path on b, given pre, a write transaction
		// b opened before the failure, or nil.
		first func(b kvtx.Store, pre kvtx.Tx) error
		// pre opens pre for first.
		pre bool
	}{
		{name: "read", first: func(b kvtx.Store, _ kvtx.Tx) error {
			_, err := b.NewTransaction(ctx, false)
			return err
		}},
		{name: "write", first: func(b kvtx.Store, _ kvtx.Tx) error {
			_, err := b.NewTransaction(ctx, true)
			return err
		}},
		{name: "ordered commit", pre: true, first: func(_ kvtx.Store, pre kvtx.Tx) error {
			return pre.Commit(kvtx.WithOrderedCommitContext(ctx))
		}},
		{name: "flush", first: func(b kvtx.Store, _ kvtx.Tx) error {
			return b.(interface{ Sync(context.Context) error }).Sync(ctx)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Open a and b on one file, and leave on b an ordered commit to
			// flush and the case's write transaction open.
			path := filepath.Join(t.TempDir(), "shared.db")
			a, err := store_kvtx_sqlite.Open(ctx, path, "kv")
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			b, err := store_kvtx_sqlite.Open(ctx, path, "kv")
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			armed := failOnce(a)
			ordered := kvtx.WithOrderedCommitContext(ctx)
			if err := putKey(ordered, b, "ordered"); err != nil {
				t.Fatal(err)
			}
			if err := putKey(ordered, a, "lost"); err != nil {
				t.Fatal(err)
			}
			var pre kvtx.Tx
			if c.pre {
				if pre, err = b.NewTransaction(ctx, true); err != nil {
					t.Fatal(err)
				}
				defer pre.Discard()
				if err := pre.Set(ctx, []byte("pre"), []byte("v-pre")); err != nil {
					t.Fatal(err)
				}
			}

			// Fail a's flush of its ordered commit, which b's open write
			// transaction leaves a no lock to commit again, then require b to
			// refuse the case's path first and every other after it.
			armed.Store(true)
			if err := a.Sync(ctx); !errors.Is(err, errWALFault) {
				t.Fatalf("failed flush returned %v", err)
			}
			if err := c.first(b, pre); !errors.Is(err, kvtx.ErrFlushFailed) {
				t.Fatalf("%s on b after a's failed flush returned %v", c.name, err)
			}
			for _, write := range []bool{false, true} {
				if _, err := b.NewTransaction(ctx, write); !errors.Is(err, kvtx.ErrFlushFailed) {
					t.Fatalf("transaction (write %v) on b after a's failed flush returned %v", write, err)
				}
			}
			if err := b.Sync(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
				t.Fatalf("Sync on b after a's failed flush returned %v", err)
			}
		})
	}
}

// TestFailedMarkFollowsFile fails a WAL flush and checks the failed mark
// belongs to the file, not its path: a hard link to the file is refused, and
// a new file at the path opens.
func TestFailedMarkFollowsFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "moved.db")
	a, err := store_kvtx_sqlite.Open(ctx, path, "kv")
	if err != nil {
		t.Fatal(err)
	}
	armed := failOnce(a)
	armed.Store(true)
	if err := putKey(ctx, a, "lost"); !errors.Is(err, errWALFault) {
		t.Fatalf("commit with a failed flush returned %v", err)
	}
	_ = a.Close()

	// A hard link names the failed file.
	link := filepath.Join(dir, "link.db")
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if linked, err := store_kvtx_sqlite.Open(ctx, link, "kv"); !errors.Is(err, kvtx.ErrFlushFailed) {
		if err == nil {
			_ = linked.Close()
		}
		t.Fatalf("open of a hard link to a failed file returned %v", err)
	}

	// Move the failed file and its WAL aside: a new file at the path is not
	// the failed one.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Rename(path+suffix, filepath.Join(dir, "aside.db"+suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	fresh, err := store_kvtx_sqlite.Open(ctx, path, "kv")
	if err != nil {
		t.Fatalf("open of a new file at a failed file's path returned %v", err)
	}
	defer fresh.Close()
	if err := putKey(ctx, fresh, "new"); err != nil {
		t.Fatalf("commit on a new file at a failed file's path returned %v", err)
	}
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
