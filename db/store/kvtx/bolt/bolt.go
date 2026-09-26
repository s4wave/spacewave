//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	bdberrors "github.com/aperturerobotics/bbolt/errors"
	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/util/broadcast"
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
	db     *bdb.DB
	bucket []byte

	// ordered counts completed ordered commits.
	ordered atomic.Uint64
	// durable is the highest ordered count a flush or full commit has made
	// durable. Sync flushes only while durable is behind ordered.
	durable atomic.Uint64
	// syncMtx serializes flushes.
	syncMtx sync.Mutex

	// bcast guards the fields below and broadcasts when durable advances or
	// the deadline flush fails.
	bcast broadcast.Broadcast
	// deadline flushes pending ordered commits, nil while none is armed.
	deadline *time.Timer
	// closed stops arming the deadline.
	closed bool
	// syncErr is the last deadline flush failure, cleared by a flush.
	syncErr error
}

// NewStore constructs a new key-value store from a bolt db.
func NewStore(db *bdb.DB, bucket []byte) *Store {
	return &Store{db: db, bucket: bucket}
}

// Open opens a bolt database store. It flushes once, so ordered commits that
// an earlier process left in the page cache are durable before this store's
// counters start.
func Open(path string, mode os.FileMode, options *bdb.Options, bucket []byte) (*Store, error) {
	if len(bucket) == 0 {
		return nil, errors.New("bucket len cannot be zero")
	}

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
	txn, err := s.db.Begin(write)
	if err != nil {
		return nil, err
	}
	tx := NewTx(txn, s.bucket)
	tx.store = s
	return tx, nil
}

// Execute executes the given store.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Store) Execute(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	dbPath := s.db.Path()
	lockPath := dbPath + "-lock"
	if err := checkBoltPaths(dbPath, lockPath); err != nil {
		_ = s.db.Close()
		return err
	}
	for _, path := range []string{dbPath, lockPath} {
		if err := watcher.Add(path); err != nil {
			if pathErr := checkBoltPaths(dbPath, lockPath); pathErr != nil {
				_ = s.db.Close()
				return pathErr
			}
			return err
		}
	}

	dirs := make(map[string]struct{})
	for _, path := range []string{dbPath, lockPath} {
		dirs[filepath.Dir(path)] = struct{}{}
	}
	for dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			if pathErr := checkBoltPaths(dbPath, lockPath); pathErr != nil {
				_ = s.db.Close()
				return pathErr
			}
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Every commit writes the database file; only creating, removing,
			// or renaming an entry can change what the paths refer to.
			if !ev.Has(fsnotify.Create | fsnotify.Remove | fsnotify.Rename) {
				continue
			}
			if err := checkBoltPaths(dbPath, lockPath); err != nil {
				_ = s.db.Close()
				return err
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
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
	ordered := s.ordered.Load()
	if s.durable.Load() >= ordered {
		return nil
	}
	s.syncMtx.Lock()
	defer s.syncMtx.Unlock()
	if s.durable.Load() >= ordered {
		return nil
	}
	if err := s.db.Sync(); err != nil {
		return err
	}
	s.markDurable(ordered)
	return nil
}

// WaitDurable waits until every ordered commit completed before the call is
// durable, without forcing a flush. It returns the deadline flush error if
// that flush fails first.
func (s *Store) WaitDurable(ctx context.Context) error {
	target := s.ordered.Load()
	for {
		var waitCh <-chan struct{}
		var err error
		done := false
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if s.durable.Load() >= target {
				done = true
				return
			}
			err = s.syncErr
			waitCh = getWaitCh()
		})
		if done || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// Close stops the deadline, flushes pending ordered commits, and closes the
// database.
func (s *Store) Close() error {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.closed = true
		if s.deadline != nil {
			s.deadline.Stop()
			s.deadline = nil
		}
	})
	return errors.Join(s.Sync(context.Background()), s.db.Close())
}

// markOrdered counts a completed ordered commit and arms the deadline flush
// unless one is already armed.
func (s *Store) markOrdered() {
	s.ordered.Add(1)
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.closed || s.deadline != nil {
			return
		}
		s.deadline = time.AfterFunc(SyncDeadline, s.flushDeadline)
	})
}

// flushDeadline flushes the ordered commits pending when the deadline fired.
func (s *Store) flushDeadline() {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.deadline = nil
	})
	err := s.Sync(context.Background())
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if err != nil {
			s.syncErr = err
			broadcast()
		}
	})
}

// markDurable records that the first n ordered commits are durable.
func (s *Store) markDurable(n uint64) {
	for {
		durable := s.durable.Load()
		if durable >= n {
			return
		}
		if s.durable.CompareAndSwap(durable, n) {
			break
		}
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.syncErr = nil
		broadcast()
	})
}

// SupportsAtomicCommit excludes unsafe or externally deferred durability modes.
func (s *Store) SupportsAtomicCommit() bool {
	return !s.db.NoSync && !s.db.NoFreelistSync
}

// _ is a type assertion
var _ kvtx.Store = (*Store)(nil)
