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
	// writers orders the write transactions ahead of bolt's own writer lock,
	// so a transaction opened with kvtx.WithBackgroundWriteContext waits
	// behind every foreground writer that is waiting. Bolt's lock grants
	// waiters in no priority order, so without it a burst of background
	// writes, such as cache fills or upload marker clears, queues ahead of a
	// player's write.
	writers csync.PrioMutex
}

// Priorities of Store.writers.
const (
	// writerBackground is the priority of write transactions opened with
	// kvtx.WithBackgroundWriteContext.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
)

// NewStore constructs a new key-value store from a bolt db.
func NewStore(db *bdb.DB, bucket []byte) *Store {
	return &Store{db: db, bucket: bucket, flusher: kvtx.NewOrderedFlusher(db.Sync, SyncDeadline)}
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

// RefreshForCoordinationLock refreshes the underlying bbolt handle after an
// external coordinator has granted a write turn.
func (s *Store) RefreshForCoordinationLock() error {
	return s.db.RefreshForCoordinationLock()
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
//
// A write transaction first takes the Store's writer turn, behind every
// waiting foreground writer when ctx carries kvtx.WithBackgroundWriteContext,
// and holds it until the transaction commits or is discarded.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Take the writer turn at the transaction's priority.
	var release func()
	if write {
		prio := writerForeground
		if kvtx.IsBackgroundWriteContext(ctx) {
			prio = writerBackground
		}
		var err error
		if release, err = s.writers.Lock(ctx, prio); err != nil {
			return nil, err
		}
	}

	// Open the Bolt transaction and attach its durability tracking Store.
	txn, err := s.db.Begin(write)
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
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
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

// SupportsAtomicCommit excludes unsafe or externally deferred durability modes.
func (s *Store) SupportsAtomicCommit() bool {
	return !s.db.NoSync && !s.db.NoFreelistSync
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
