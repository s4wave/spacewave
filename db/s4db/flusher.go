package s4db

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// syncDeadline is how long an ordered commit may stay unflushed before the
// flusher flushes it.
const syncDeadline = time.Second

// flusher tracks which commits are durable and shares flushes between the
// commits waiting on them.
type flusher struct {
	// s is the database file.
	s storage
	// cur is the published state; a flush covers every commit in it.
	cur *atomic.Pointer[state]
	// onFlush, when set, runs after each durable flush with the state it
	// covered, so the writer can learn the marks without polling.
	onFlush func(*state)

	// bcast guards the fields below and wakes waiters when a flush ends.
	bcast broadcast.Broadcast
	// durable is the last commit known flushed to the drive.
	durable uint64
	// logged is the logged mark of the newest state known flushed. A record
	// carries the durable mark it was written under, and recovery checks the
	// values of every record past the highest mark in the log, ending the
	// log at the first whose values are gone. A value chunk freed by a
	// commit past logged may still be checked, so it stays pending.
	logged uint64
	// flushing is set while one caller flushes for every waiting commit.
	flushing bool
	// deadline flushes ordered commits syncDeadline after the first one
	// left unflushed; nil when none is armed.
	deadline *time.Timer
	// deadlineFlushing is set while a fired deadline flushes.
	deadlineFlushing bool
	// err is the error of the last deadline flush, cleared by a later
	// successful flush.
	err error
	// closed stops arming the deadline.
	closed bool
}

// newFlusher returns a flusher of s whose published state is cur.
func newFlusher(s storage, cur *atomic.Pointer[state]) *flusher {
	return &flusher{s: s, cur: cur}
}

// marks returns the durable commit and the bound for reusing the value
// chunks commits freed.
func (f *flusher) marks() (durable, logged uint64) {
	l := f.bcast.Lock()
	defer l.Unlock()
	return f.durable, f.logged
}

// setDurable records that the state st is durable: a flush through another
// path, such as the flush job's, covered it.
func (f *flusher) setDurable(st *state) {
	// Raise the marks and wake the waiters.
	l := f.bcast.Lock()
	f.durable = max(f.durable, st.seq)
	f.logged = max(f.logged, st.logged)
	l.Broadcast()
	l.Unlock()
}

// ordered arms the deadline flush after an ordered commit unless one is
// armed.
func (f *flusher) ordered() {
	l := f.bcast.Lock()
	defer l.Unlock()
	if f.closed || f.deadline != nil {
		return
	}
	f.deadline = time.AfterFunc(syncDeadline, f.flushDeadline)
}

// flushDeadline flushes the commits published when the deadline fired.
func (f *flusher) flushDeadline() {
	// Let the next ordered commit arm a new deadline, and mark the flush
	// running so close waits for it.
	l := f.bcast.Lock()
	f.deadline = nil
	if f.closed {
		l.Unlock()
		return
	}
	f.deadlineFlushing = true
	l.Unlock()

	// Flush, and report a failure to the waiters.
	err := f.syncThrough(context.Background(), f.cur.Load().seq)
	l = f.bcast.Lock()
	f.deadlineFlushing = false
	if err != nil {
		f.err = err
	}
	l.Broadcast()
	l.Unlock()
}

// waitDurable returns once commit seq is durable without flushing, or with
// the error of a failed deadline flush.
func (f *flusher) waitDurable(ctx context.Context, seq uint64) error {
	return f.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return f.durable >= seq, f.err
	})
}

// close stops the deadline flush and waits for a running flush, so no flush
// outlives the file. Later flushes fail with ErrClosed. The caller flushes
// afterward.
func (f *flusher) close() {
	// Disarm the deadline.
	l := f.bcast.Lock()
	f.closed = true
	if f.deadline != nil {
		f.deadline.Stop()
		f.deadline = nil
	}
	l.Unlock()

	// Wait for a deadline that already fired and a shared flush.
	_ = f.bcast.Wait(context.Background(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return !f.deadlineFlushing && !f.flushing, nil
	})
}

// syncThrough returns once commit seq is durable. Commits share flushes: one
// caller flushes for every commit published before its flush starts, and
// callers arriving meanwhile wait for it, then flush the next group if it did
// not cover them.
func (f *flusher) syncThrough(ctx context.Context, seq uint64) error {
	for {
		// Finish when covered, or wait for a running flush.
		l := f.bcast.Lock()
		if f.durable >= seq {
			l.Unlock()
			return nil
		}
		if f.closed && !f.flushing {
			l.Unlock()
			return ErrClosed
		}
		if f.flushing {
			wait := l.WaitCh()
			l.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
			}
			continue
		}

		// Claim the flush of every published commit.
		f.flushing = true
		target := f.cur.Load()
		l.Unlock()

		// Flush and wake the waiters.
		err := f.s.flushDurable()
		l = f.bcast.Lock()
		f.flushing = false
		if err == nil {
			f.durable = max(f.durable, target.seq)
			f.logged = max(f.logged, target.logged)
			f.err = nil
		}
		l.Broadcast()
		l.Unlock()
		if err != nil {
			return err
		}
		if f.onFlush != nil {
			f.onFlush(target)
		}
	}
}
