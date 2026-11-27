package s4db

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/kvtx"
)

// failStop is the storage of a writable handle. It records the first failure
// of a durable flush or a writeback, and then fails every flush, write and
// truncation with it without calling the file. kvtx.FailStopOff turns it
// off.
//
// A commit is published before its flush, so a failed flush leaves readers
// seeing commits the file may not hold. A later flush cannot prove they are
// held: after a failed fdatasync Linux may mark the unwritten pages clean, so
// the next fdatasync succeeds without them, and a device keeps any part of a
// failed call. The file shows what it holds only to the next writable open,
// which flushes it and then drops its cached pages (storage.evict) before
// recovery reads it: the flush makes durable whatever the file holds, dirty
// pages and device writes included, and the drop makes recovery read what
// the drive holds rather than pages that never reached it.
//
// Invariants, once a flush or writeback of this handle fails:
//   - no later flush succeeds, so no commit published before the failure is
//     reported durable, through syncThrough or the flush job's setDurable;
//   - no write, truncation or writeback starts after the failure is
//     recorded, and close sends no write the handle still holds, so no
//     record, free or shortening is built on a state the file may not hold;
//   - the handle refuses every new transaction and every commit
//     (DB.failure), so no caller reads a commit whose flush failed and takes
//     it for stored.
type failStop struct {
	storage
	// gate orders each call to the file against the failure: a call holds it
	// shared from its check through its end, and recording the failure holds
	// it alone, so a call either ends before the failure is recorded or sees
	// it and does not start.
	gate sync.RWMutex
	// err is the first failure, joined with kvtx.ErrFlushFailed, or nil.
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
// recorded failure. A nil err records nothing. The caller must not hold gate.
func (s *failStop) fail(err error) error {
	// A call that succeeded records nothing.
	if err == nil {
		return nil
	}

	// Record the failure once no file call runs, then wake the waiters.
	joined := errors.Join(kvtx.ErrFlushFailed, err)
	s.gate.Lock()
	recorded := s.err.CompareAndSwap(nil, &joined)
	s.gate.Unlock()
	if recorded {
		l := s.bcast.Lock()
		l.Broadcast()
		l.Unlock()
	}
	return s.failed()
}

// pass runs call under the shared gate unless a flush failed.
func (s *failStop) pass(call func() error) error {
	s.gate.RLock()
	defer s.gate.RUnlock()
	if err := s.failed(); err != nil {
		return err
	}
	return call()
}

// flushDurable flushes the file unless a flush failed.
func (s *failStop) flushDurable() error {
	return s.fail(s.pass(s.storage.flushDurable))
}

// writeback writes the chunks back unless a flush failed. A writeback error
// can lose pages as a failed flush does, so it fails the handle too.
func (s *failStop) writeback(chunks []uint64) error {
	return s.fail(s.pass(func() error { return s.storage.writeback(chunks) }))
}

// WriteAt writes p at off unless a flush failed.
func (s *failStop) WriteAt(p []byte, off int64) (int, error) {
	var n int
	err := s.pass(func() error {
		var err error
		n, err = s.storage.WriteAt(p, off)
		return err
	})
	return n, err
}

// truncate sets the file length unless a flush failed.
func (s *failStop) truncate(n int64) error {
	return s.pass(func() error { return s.storage.truncate(n) })
}

// close drops the writes the storage still holds once a flush failed, then
// closes it. A device handle would otherwise send a failed commit's writes,
// and the next open would read the commit as stored before any flush.
func (s *failStop) close() error {
	if s.failed() != nil {
		s.discard()
	}
	return s.storage.close()
}

// failure returns why the handle refuses transactions and commits: the
// failed flush, or nil.
func (db *DB) failure() error {
	if db.failStop == nil {
		return nil
	}
	return db.failStop.failed()
}
