//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	coord_bolt "github.com/s4wave/spacewave/db/coord/bolt"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/kvtx"
)

// SyncDeadline is how long an ordered commit may stay pending before the store
// flushes it on its own.
const SyncDeadline = time.Second

// Store is a bolt database key-value store.
//
// An ordered commit becomes durable at the first of: Sync, a later full
// commit, SyncDeadline after the first pending ordered commit, or Close.
type Store struct {
	// db is the bolt database.
	db *bdb.DB
	// bucket is the bucket holding the store's keys.
	bucket []byte
	// flusher makes ordered commits durable with a bolt flush.
	flusher *kvtx.OrderedFlusher
	// writers orders write transactions ahead of bolt's own writer lock,
	// so a transaction opened with block.WithBackground waits
	// behind every foreground writer that is waiting. Bolt's lock grants
	// waiters in no priority order, so without writers a burst of
	// background writes, such as cache fills or upload marker clears,
	// queues ahead of a player's write.
	writers csync.PrioMutex
}

// writerBackground is a Store.writers priority, as is writerForeground.
const (
	// writerBackground is the priority of write transactions opened with
	// block.WithBackground.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
)

// NewStore constructs a new key-value store from a bolt db.
func NewStore(db *bdb.DB, bucket []byte) *Store {
	return &Store{db: db, bucket: bucket, flusher: kvtx.NewOrderedFlusher(db.Sync, SyncDeadline, kvtx.NewFailStop())}
}

// Open opens a bolt database store. It flushes once, so ordered commits that
// an earlier process left in the page cache are durable before this store's
// counters start.
func Open(path string, mode os.FileMode, options *bdb.Options, bucket []byte) (*Store, error) {
	// Require a bucket name before opening the Bolt database.
	if len(bucket) == 0 {
		return nil, errors.New("bucket len cannot be zero")
	}

	// Open the Bolt database and make earlier ordered commits durable.
	b, err := bdb.Open(path, mode, options)
	if err != nil {
		return nil, err
	}
	if !b.IsReadOnly() {
		if err := b.Sync(); err != nil {
			return nil, errors.Join(err, b.Close())
		}
	}

	return NewStore(b, bucket), nil
}

// GetDB returns the bolt DB.
func (s *Store) GetDB() *bdb.DB {
	return s.db
}

// NewCoordinator builds a coordinator over this store's commit generation and locks.
func (s *Store) NewCoordinator(volumeID string) coord.Coordinator {
	return coord_bolt.NewCoordinator(s.db, coord_inmem.ForVolume(volumeID))
}

// FileSize reports the database file size, or zero when its stat fails.
func (s *Store) FileSize() (uint64, error) {
	// Preserve unavailable file statistics as a zero size.
	fi, err := os.Stat(s.db.Path())
	if err != nil {
		return 0, nil
	}

	// Reject a negative file size before converting to an unsigned count.
	if fi.Size() < 0 {
		return 0, errors.New("bolt file size is negative")
	}
	return uint64(fi.Size()), nil //nolint:gosec // The negative size was rejected above.
}

// beginBatchTransaction opens the batch's shared write transaction without a
// per-transaction writer turn, unless a flush or commit failed. BatchStore
// serializes its use with its mutex.
func (s *Store) beginBatchTransaction() (*bdb.Tx, error) {
	if err := s.flusher.Failed(); err != nil {
		return nil, err
	}
	return s.beginWrite()
}

// beginBolt opens a bolt transaction. A test replaces it to fail a write
// Begin as a failed fsync inside it would, since bolt fsyncs inside Begin only
// a file it opened itself, which a test cannot make fail.
var beginBolt = (*bdb.DB).Begin

// beginWrite opens a bolt write transaction. Bolt fsyncs its file inside
// Begin before it reuses pages from a freelist read from disk, as after an
// open, so a failure of that fsync, which bolt wraps in ErrSyncFailed, fails
// the store for the reason a failed commit does. Begin's other errors, such
// as a failure to take the writer lock in multi-process mode, leave the file
// as it was and fail only the call.
func (s *Store) beginWrite() (*bdb.Tx, error) {
	txn, err := beginBolt(s.db, true)
	if errors.Is(err, bdberrors.ErrSyncFailed) {
		return nil, s.flusher.Fail(err)
	}
	return txn, err
}

// commit runs commitFn, which commits txn, unless a flush or commit failed,
// and otherwise rolls txn back so it releases bolt's writer lock. A write
// transaction opened before the failure thus commits nothing after it.
//
// A write commit runs under the flusher's flush lock, since bolt flushes its
// file inside the commit: a full commit fsyncs, and on Linux an ordered one
// fdatasyncs its data pages. Linux reports a writeback error to one fsync of
// the file, so a commit beside the deadline flush could take the error the
// flush needed, or the reverse. A commit failing on storage fails the store:
// after a failed fsync Linux may mark the unwritten pages clean, so the next
// commit's fsync succeeds and its meta page points at pages the drive never
// held.
func (s *Store) commit(txn *bdb.Tx, commitFn func() error) error {
	// A read-only transaction writes nothing to fail.
	if !txn.Writable() {
		return commitFn()
	}

	// Commit unless the store failed, rolling back a refused transaction.
	ran := false
	err := s.flusher.Flushing(func() error {
		ran = true
		return commitFn()
	}, storageErr)
	if !ran {
		_ = txn.Rollback()
	}
	return err
}

// storageErr reports whether a commit error may come from writing or
// flushing the file. The errors bolt returns before it touches the file, or
// for a file it no longer writes, are not; any other error is, so an unknown
// one fails the store.
func storageErr(err error) bool {
	for _, refusal := range []error{
		bdberrors.ErrTxClosed,
		bdberrors.ErrTxNotWritable,
		bdberrors.ErrDatabaseNotOpen,
		bdberrors.ErrDatabaseReadOnly,
		bdberrors.ErrLockFileChanged,
		bdberrors.ErrMaxSizeReached,
	} {
		if errors.Is(err, refusal) {
			return false
		}
	}
	return true
}

// RefreshForCoordinationLock refreshes the underlying bbolt handle after an
// external coordinator has granted a write turn.
func (s *Store) RefreshForCoordinationLock() error {
	return s.db.RefreshForCoordinationLock()
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will be writable.
// Always call Discard() after using the transaction.
//
// A write transaction first takes the Store's writer turn, behind every
// waiting foreground writer when ctx carries block.WithBackground,
// and holds it until the transaction commits or is discarded.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Refuse every transaction once an ordered flush failed, since the file
	// may lack the ordered commits a transaction would read.
	if err := s.flusher.Failed(); err != nil {
		return nil, err
	}

	// Take the writer turn at the transaction's priority.
	var release func()
	if write {
		prio := writerForeground
		if block.IsBackground(ctx) {
			prio = writerBackground
		}
		var err error
		if release, err = s.writers.Lock(ctx, prio); err != nil {
			return nil, err
		}
	}

	// Open the Bolt transaction and attach its durability tracking Store.
	var txn *bdb.Tx
	var err error
	if write {
		txn, err = s.beginWrite()
	} else {
		txn, err = s.db.Begin(false)
	}
	if err != nil {
		if release != nil {
			release()
		}
		return nil, err
	}
	tx := NewTx(txn, s.bucket)
	tx.store = s
	tx.release = release
	return tx, nil
}

// Execute closes the database and returns ErrLockFileChanged once the
// database or lock file path no longer exists.
func (s *Store) Execute(ctx context.Context) error {
	// Bolt keeps its lock file beside the database file.
	dbPath := s.db.Path()
	lockPath := dbPath + "-lock"

	// Watch the directory holding both paths before checking them, so a change
	// after the first check still wakes this Store.
	sub, err := sharedDirWatcher.watch(filepath.Dir(dbPath))
	if err != nil {
		if pathErr := checkBoltPaths(dbPath, lockPath); pathErr != nil {
			_ = s.db.Close()
			return pathErr
		}
		return err
	}
	defer sharedDirWatcher.release(sub)

	// Check both paths after every entry change in the directory.
	for {
		if err := checkBoltPaths(dbPath, lockPath); err != nil {
			_ = s.db.Close()
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sub.changed:
		case err := <-sub.errs:
			if pathErr := checkBoltPaths(dbPath, lockPath); pathErr != nil {
				_ = s.db.Close()
				return pathErr
			}
			return err
		}
	}
}

// checkBoltPaths returns ErrLockFileChanged if the database or lock file path
// no longer exists.
func checkBoltPaths(dbPath, lockPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return errors.Join(bdberrors.ErrLockFileChanged, err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		return errors.Join(bdberrors.ErrLockFileChanged, err)
	}
	return nil
}

// Sync makes every completed ordered commit durable. It flushes only when an
// ordered commit is not yet covered by an earlier flush or full commit.
func (s *Store) Sync(ctx context.Context) error {
	return s.flusher.Sync(ctx)
}

// WaitDurable waits until every ordered commit completed before the call is
// durable, without forcing a flush. It returns the deadline flush error if
// that flush fails first.
func (s *Store) WaitDurable(ctx context.Context) error {
	return s.flusher.WaitDurable(ctx)
}

// Close stops the deadline, flushes pending ordered commits, and closes the
// database.
func (s *Store) Close() error {
	return errors.Join(s.flusher.Close(), s.db.Close())
}

// SupportsAtomicCommit reports true: a bolt write transaction commits all of
// its writes or none of them, and holds the database writer lock across its
// reads and writes. NoSync and NoFreelistSync weaken durability, not atomicity.
func (s *Store) SupportsAtomicCommit() bool {
	return true
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
