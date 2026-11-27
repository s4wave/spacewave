package s4db

import (
	"errors"
	"os"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
)

// FailStopEnv names the switch for fail-stop. Set to "off", a failed flush
// fails only its own call, and a later flush that succeeds marks every commit
// published before it durable, as before fail-stop.
const FailStopEnv = "SPACEWAVE_S4DB_FAIL_STOP"

// failStopOff turns fail-stop off for every handle opened in this process.
var failStopOff = os.Getenv(FailStopEnv) == "off"

// failStop is the storage of a writable handle. It records the first failure
// of a durable flush or a writeback, and fails every later one with it
// without calling the file.
//
// A commit is published before its flush, so a failed flush leaves readers
// seeing commits the file may not hold. A later flush cannot prove they are
// held: after a failed fdatasync Linux may mark the unwritten pages clean, so
// the next fdatasync succeeds without them, and a device keeps any part of a
// failed call. Only recovery from the file, at the next open, shows what it
// holds.
//
// Invariant: once a flush of this handle fails, no later flush succeeds, so
// no commit published before the failure is reported durable, through
// syncThrough or the flush job's setDurable. The handle also refuses every new
// transaction and every commit (DB.failure), so no caller reads a commit whose
// flush failed and takes it for stored.
type failStop struct {
	storage
	// err is the first failure, joined with ErrFlushFailed, or nil.
	err atomic.Pointer[error]
	// bcast wakes the handle's waiters once the failure is recorded, so a
	// commit waiting for a flush to drain the memtable ends.
	bcast *broadcast.Broadcast
}

// newFailStop wraps s, waking bcast's waiters when a flush first fails.
func newFailStop(s storage, bcast *broadcast.Broadcast) *failStop {
	return &failStop{storage: s, bcast: bcast}
}

// failed returns the recorded failure, or nil.
func (s *failStop) failed() error {
	if p := s.err.Load(); p != nil {
		return *p
	}
	return nil
}

// fail records err as the failure unless one is recorded, and returns the
// recorded failure. A nil err records nothing.
func (s *failStop) fail(err error) error {
	if err == nil {
		return nil
	}
	joined := errors.Join(ErrFlushFailed, err)
	if s.err.CompareAndSwap(nil, &joined) {
		l := s.bcast.Lock()
		l.Broadcast()
		l.Unlock()
	}
	return s.failed()
}

// flushDurable flushes the file unless a flush failed.
func (s *failStop) flushDurable() error {
	if err := s.failed(); err != nil {
		return err
	}
	return s.fail(s.storage.flushDurable())
}

// writeback writes the chunks back unless a flush failed. A writeback error
// can lose pages as a failed flush does, so it fails the handle too.
func (s *failStop) writeback(chunks []uint64) error {
	if err := s.failed(); err != nil {
		return err
	}
	return s.fail(s.storage.writeback(chunks))
}

// failure returns why the handle refuses transactions and commits: the
// failed flush, or nil.
func (db *DB) failure() error {
	if db.failStop == nil {
		return nil
	}
	return db.failStop.failed()
}
