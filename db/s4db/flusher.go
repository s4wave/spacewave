package s4db

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// syncDeadline is how long an ordered commit may stay unflushed before the
// flusher flushes it.
const syncDeadline = time.Second

// flusher tracks which commits and checkpoints are durable and shares
// flushes between the commits waiting on them.
type flusher struct {
	// f is the database file.
	f *os.File
	// cur is the published state; a flush covers every commit in it.
	cur *atomic.Pointer[state]

	// bcast guards the fields below and wakes waiters when a flush ends.
	bcast broadcast.Broadcast
	// durable is the last commit known flushed to the drive.
	durable uint64
	// ckptWritten is the checkpoint of the newest superblock in the file.
	ckptWritten uint64
	// ckptDurable is the newest checkpoint known flushed. Pages a checkpoint
	// replaces stay unreleased until its superblock is durable, so a crash
	// that reverts to the previous superblock finds its tree intact.
	ckptDurable uint64
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

// newFlusher returns a flusher of f whose published state is cur.
func newFlusher(f *os.File, cur *atomic.Pointer[state]) *flusher {
	return &flusher{f: f, cur: cur}
}

// marks returns the durable commit and checkpoint.
func (f *flusher) marks() (durable, ckpt uint64) {
	l := f.bcast.Lock()
	defer l.Unlock()
	return f.durable, f.ckptDurable
}

// newest returns the checkpoint of the newest superblock in the file.
func (f *flusher) newest() uint64 {
	l := f.bcast.Lock()
	defer l.Unlock()
	return f.ckptWritten
}

// written records that the file holds a superblock of checkpoint ckpt.
func (f *flusher) written(ckpt uint64) {
	l := f.bcast.Lock()
	f.ckptWritten = max(f.ckptWritten, ckpt)
	l.Unlock()
}

// setDurable records that commit seq and checkpoint ckpt are durable.
func (f *flusher) setDurable(seq, ckpt uint64) {
	// Raise the marks and wake the waiters.
	l := f.bcast.Lock()
	f.durable = max(f.durable, seq)
	f.ckptDurable = max(f.ckptDurable, ckpt)
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
	err := f.syncThrough(context.Background(), f.cur.Load().seq, 0)
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

// close stops the deadline flush and waits for a running one, so no flush
// outlives the file. The caller flushes afterward.
func (f *flusher) close() {
	// Disarm the deadline.
	l := f.bcast.Lock()
	f.closed = true
	if f.deadline != nil {
		f.deadline.Stop()
		f.deadline = nil
	}
	l.Unlock()

	// Wait for a deadline that already fired.
	_ = f.bcast.Wait(context.Background(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return !f.deadlineFlushing, nil
	})
}

// syncThrough returns once commit seq and checkpoint ckpt are durable.
// Commits share flushes: one caller flushes for every commit published before
// its flush starts, and callers arriving meanwhile wait for it, then flush the
// next group if it did not cover them.
func (f *flusher) syncThrough(ctx context.Context, seq, ckpt uint64) error {
	for {
		// Finish when covered, or wait for a running flush.
		l := f.bcast.Lock()
		if f.durable >= seq && f.ckptDurable >= ckpt {
			l.Unlock()
			return nil
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

		// Claim the flush of every published commit and the newest
		// superblock.
		f.flushing = true
		target, targetCkpt := f.cur.Load().seq, f.ckptWritten
		l.Unlock()

		// Flush and wake the waiters.
		err := flushDurable(f.f)
		l = f.bcast.Lock()
		f.flushing = false
		if err == nil {
			f.durable = max(f.durable, target)
			f.ckptDurable = max(f.ckptDurable, targetCkpt)
			f.err = nil
		}
		l.Broadcast()
		l.Unlock()
		if err != nil {
			return err
		}
	}
}
