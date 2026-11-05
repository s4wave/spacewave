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
}

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
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Open the Bolt transaction and attach its durability tracking Store.
	txn, err := s.db.Begin(write)
	if err != nil {
		return nil, err
	}
	tx := NewTx(txn, s.bucket)
	tx.store = s
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

// SupportsAtomicCommit reports true: a bolt write transaction commits all of
// its writes or none of them, and holds the database writer lock across its
// reads and writes. NoSync and NoFreelistSync weaken durability, not atomicity.
func (s *Store) SupportsAtomicCommit() bool {
	return true
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
