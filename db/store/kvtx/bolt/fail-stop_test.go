//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/device"
)

// errSyncFault is the sync failure failSyncStorage injects.
var errSyncFault = errors.New("injected sync failure")

// failSyncStorage is a bolt data file on a memory device whose next Sync,
// once armed, fails.
type failSyncStorage struct {
	*device.Handle
	// armed fails the next Sync.
	armed atomic.Bool
}

// Sync fails once armed, and otherwise syncs the handle.
func (s *failSyncStorage) Sync() error {
	if s.armed.Swap(false) {
		return errSyncFault
	}
	return s.Handle.Sync()
}

// openFailSyncStore opens a store on a failSyncStorage.
func openFailSyncStore(t *testing.T) (*Store, *failSyncStorage) {
	t.Helper()
	h, err := device.OpenHandle(t.Context(), device.NewMemory(), "db")
	if err != nil {
		t.Fatal(err)
	}
	fs := &failSyncStorage{Handle: h}
	db, err := bdb.OpenStorage(fs, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(db, []byte("fail"))
	t.Cleanup(func() { _ = s.Close() })
	return s, fs
}

// setKey writes key in a new write transaction, which it returns uncommitted.
func setKey(t *testing.T, s *Store, key string) kvtx.Tx {
	t.Helper()
	tx, err := s.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(t.Context(), []byte(key), []byte(key)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	return tx
}

// TestBoltFailedCommitSyncStops fails the fsync of a full commit, which bolt
// makes itself. The store must record it and refuse every later transaction,
// batched ones included: the next commit's fsync could succeed without the
// lost pages and point its meta page at them.
func TestBoltFailedCommitSyncStops(t *testing.T) {
	ctx := context.Background()
	s, fs := openFailSyncStore(t)
	commitTestKey(t, s, "kept", false)

	fs.armed.Store(true)
	tx := setKey(t, s, "failed")
	if err := tx.Commit(ctx); !errors.Is(err, errSyncFault) || !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit with a failed fsync returned %v", err)
	}
	if _, err := s.NewTransaction(ctx, true); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("write transaction after a failed commit fsync returned %v", err)
	}
	if _, err := NewBatchStore(s, 4).NewTransaction(ctx, true); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("batched write transaction after a failed commit fsync returned %v", err)
	}
}

// TestBoltOpenTxAfterFailedSync opens a write transaction, fails an ordered
// flush, and then commits the transaction. It must commit nothing: its
// commit's fsync could succeed without the pages the failed flush lost.
func TestBoltOpenTxAfterFailedSync(t *testing.T) {
	ctx := context.Background()
	s, fs := openFailSyncStore(t)
	commitTestKey(t, s, "ordered", true)

	// Open the transaction, then fail the flush of the ordered commit.
	// Discard runs before the cleanup's Close, which waits for bolt's writer
	// lock that the transaction holds until it ends.
	tx := setKey(t, s, "late")
	defer tx.Discard()
	fs.armed.Store(true)
	if err := s.Sync(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("failed Sync returned %v", err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit of a transaction opened before a failed flush returned %v", err)
	}
	var found bool
	if err := s.db.View(func(btx *bdb.Tx) error {
		if b := btx.Bucket(s.bucket); b != nil {
			found = b.Get([]byte("late")) != nil
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("a refused commit's write is visible")
	}
}

// reportOnceStorage models Linux reporting a writeback error to one fsync of
// an open file: its armed Sync waits for release and fails, and a Sync that
// starts while that one runs returns success, as the fsync whose error went
// to the other caller does.
type reportOnceStorage struct {
	*device.Handle
	// armed holds the next Sync and fails it.
	armed atomic.Bool
	// running is set while the held Sync runs.
	running atomic.Bool
	// entered receives once the held Sync arrives.
	entered chan struct{}
	// overlapped receives once a Sync starts while the held one runs.
	overlapped chan struct{}
	// release ends the held Sync with its error.
	release chan error
}

// Sync holds and fails the armed Sync, and reports success for a Sync
// overlapping it.
func (s *reportOnceStorage) Sync() error {
	if s.armed.Swap(false) {
		s.running.Store(true)
		defer s.running.Store(false)
		s.entered <- struct{}{}
		return <-s.release
	}
	if s.running.Load() {
		select {
		case s.overlapped <- struct{}{}:
		default:
		}
		return nil
	}
	return s.Handle.Sync()
}

// TestBoltCommitBesideFailedSync runs a full commit while the deadline
// flush's fsync is failing. The commit must not report success: bolt fsyncs
// inside the commit, and on Linux its fsync can succeed beside the failing
// one for the same lost pages, so the commit would mark the ordered commits
// before it durable and its meta page would point at pages the drive lacks.
func TestBoltCommitBesideFailedSync(t *testing.T) {
	ctx := context.Background()
	h, err := device.OpenHandle(t.Context(), device.NewMemory(), "db")
	if err != nil {
		t.Fatal(err)
	}
	rs := &reportOnceStorage{
		Handle:     h,
		entered:    make(chan struct{}),
		overlapped: make(chan struct{}, 1),
		release:    make(chan error),
	}
	db, err := bdb.OpenStorage(rs, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(db, []byte("fail"))
	t.Cleanup(func() { _ = s.Close() })
	commitTestKey(t, s, "ordered", true)

	// Hold the failing flush of the ordered commit, then commit beside it.
	rs.armed.Store(true)
	synced := make(chan error, 1)
	go func() { synced <- s.Sync(ctx) }()
	<-rs.entered
	tx := setKey(t, s, "full")
	defer tx.Discard()
	committed := make(chan error, 1)
	go func() { committed <- tx.Commit(ctx) }()
	select {
	case <-rs.overlapped:
	case <-time.After(200 * time.Millisecond):
	}

	// Fail the flush. Neither the flush nor the commit may report success.
	rs.release <- errSyncFault
	if err := <-synced; !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("failed Sync returned %v", err)
	}
	if err := <-committed; !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit beside a failed Sync returned %v", err)
	}
	if !s.flusher.Pending() {
		t.Fatal("the ordered commit was marked durable after its flush failed")
	}
}

// TestBoltRefusedCommitKeepsStore commits a closed transaction. bolt refuses
// it before touching the file, so the store must not fail stop on it.
func TestBoltRefusedCommitKeepsStore(t *testing.T) {
	s, _ := openFailSyncStore(t)
	btx, err := s.db.Begin(true)
	if err != nil {
		t.Fatal(err)
	}
	if err := btx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.commit(btx, btx.Commit); err == nil || errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit of a closed transaction returned %v", err)
	}
	if err := s.flusher.Failed(); err != nil {
		t.Fatalf("a refused commit failed the store: %v", err)
	}
	commitTestKey(t, s, "after", false)
}

// TestBoltFailedBeginStops fails a write Begin as a failed fsync inside it
// would: bolt fsyncs before it reuses pages from a freelist read from disk.
// The store must record the failure from both the transaction and the batch
// path, since after a failed fsync Linux may mark the unwritten pages clean
// and a later Begin's fsync would succeed without them. A Begin that fails to
// take the writer lock leaves the file as it was, and must fail only itself.
func TestBoltFailedBeginStops(t *testing.T) {
	ctx := context.Background()
	lockErr := fmt.Errorf("bbolt: acquire writer lock: %w", syscall.EDEADLK)
	syncErr := fmt.Errorf("%w: order writes: %w", bdberrors.ErrSyncFailed, errSyncFault)
	for _, batched := range []bool{false, true} {
		s, _ := openFailSyncStore(t)
		begin := beginBolt
		var fault error
		beginBolt = func(db *bdb.DB, write bool) (*bdb.Tx, error) {
			if write && fault != nil {
				return nil, fault
			}
			return begin(db, write)
		}
		t.Cleanup(func() { beginBolt = begin })
		batch := NewBatchStore(s, 4)
		beginTx := func() error {
			if !batched {
				tx, err := s.NewTransaction(ctx, true)
				if err == nil {
					tx.Discard()
				}
				return err
			}
			// A batch keeps its bolt transaction past Discard, so commit
			// it to release bolt's writer lock.
			tx, err := batch.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			tx.Discard()
			return batch.Flush()
		}

		// A lock failure fails the Begin alone.
		fault = lockErr
		if err := beginTx(); !errors.Is(err, syscall.EDEADLK) || errors.Is(err, kvtx.ErrFlushFailed) {
			t.Fatalf("write Begin failing its writer lock returned %v (batched %v)", err, batched)
		}
		fault = nil
		if err := beginTx(); err != nil {
			t.Fatalf("write Begin after a lock failure returned %v (batched %v)", err, batched)
		}

		// A failed fsync fails the store.
		fault = syncErr
		if err := beginTx(); !errors.Is(err, errSyncFault) || !errors.Is(err, kvtx.ErrFlushFailed) {
			t.Fatalf("write Begin with a failed fsync returned %v (batched %v)", err, batched)
		}
		fault = nil
		if _, err := s.NewTransaction(ctx, false); !errors.Is(err, kvtx.ErrFlushFailed) {
			t.Fatalf("read transaction after a failed Begin returned %v (batched %v)", err, batched)
		}
	}
}
