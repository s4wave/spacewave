//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	bdb "github.com/aperturerobotics/bbolt"
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
