package kvtx

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// OrderedFlusher makes a store's ordered commits durable in groups. The store
// counts each ordered commit after it completes, and a flush covers every
// commit counted before it began. One flush runs at a time. Callers that queue
// behind it find their commits covered or share the next one. An ordered
// commit becomes durable at the first of: Sync, a full commit the store
// reports with MarkDurable, the deadline after the first pending ordered
// commit, or Close.
//
// The flusher fails stop. Once a flush fails, no later flush or full commit
// proves the commits before it durable: after a failed fdatasync Linux may
// mark the unwritten pages clean, so the next one succeeds without them.
// Invariant: after the first failed flush, or the first failure a store
// reports with Fail, Sync, WaitDurable and Close return it joined with
// ErrFlushFailed without calling flush, MarkDurable marks nothing, Failed
// returns it, and Admit and Flushing run nothing. A store checks Failed to
// refuse every new transaction, and runs each commit and each write to its
// file through Admit, or through Flushing when the call flushes the file the
// flush does, so none starts after the failure is recorded. FailStopOff turns
// this off.
//
// The failure, the flush lock and the gate are the file's FailStop, which
// every flusher of the file shares. Flushes of one file run one at a time,
// each recording its failure before the next starts. Linux reports a
// writeback error to one fdatasync of an open file, so of two overlapping
// flushes one could fail while the other returns success for the same lost
// pages and marks them durable.
type OrderedFlusher struct {
	// flush makes every commit completed before the call durable.
	flush func() error
	// deadline is how long an ordered commit may stay pending before the
	// flusher flushes it on its own.
	deadline time.Duration

	// ordered counts completed ordered commits.
	ordered atomic.Uint64
	// durable is the highest ordered count a flush or full commit has made
	// durable.
	durable atomic.Uint64
	// stop is the fail-stop state of the file flush flushes.
	stop *FailStop

	// bcast guards the fields below and broadcasts when durable advances or
	// the deadline flush fails.
	bcast broadcast.Broadcast
	// timer flushes pending ordered commits, nil while none is armed.
	timer *time.Timer
	// closed stops arming the timer.
	closed bool
	// flushErr is the last deadline flush failure, cleared by a flush.
	flushErr error
}

// NewOrderedFlusher returns a flusher that calls flush to make commits durable
// and flushes a pending ordered commit within deadline. stop is the fail-stop
// state of the file flush flushes, shared with every other flusher of it.
func NewOrderedFlusher(flush func() error, deadline time.Duration, stop *FailStop) *OrderedFlusher {
	return &OrderedFlusher{flush: flush, deadline: deadline, stop: stop}
}

// Failed returns the first failure of the flusher's file, joined with
// ErrFlushFailed, or nil. A store refuses new transactions and commits while
// it is set.
func (f *OrderedFlusher) Failed() error {
	return f.stop.Failed()
}

// Ordered counts a completed ordered commit and arms the deadline flush
// unless one is already armed.
func (f *OrderedFlusher) Ordered() {
	f.ordered.Add(1)
	f.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if f.closed || f.timer != nil {
			return
		}
		f.timer = time.AfterFunc(f.deadline, f.flushDeadline)
	})
}

// Count returns the number of ordered commits counted so far. A store whose
// full commit also flushes reads it before that commit and passes it to
// MarkDurable after.
func (f *OrderedFlusher) Count() uint64 {
	return f.ordered.Load()
}

// Pending reports whether a counted ordered commit is not yet durable.
func (f *OrderedFlusher) Pending() bool {
	return f.durable.Load() < f.ordered.Load()
}

// Sync makes every ordered commit counted before the call durable. It flushes
// only when one is not yet covered by an earlier flush or full commit.
// It waits for the flush regardless of ctx: a store calls it after its commit
// is visible, and a commit that returned on cancellation would be reported
// failed while readers take it for stored.
func (f *OrderedFlusher) Sync(ctx context.Context) error {
	// Skip the flush when the counted commits are already durable.
	target := f.ordered.Load()
	if f.durable.Load() >= target {
		return nil
	}

	// Flush once for every commit counted by the time this flush begins,
	// unless a flush of the file failed.
	return f.stop.Flushing(func() error {
		// Skip commits a flush that held the lock before this one covered.
		if f.durable.Load() >= target {
			return nil
		}

		// Flush, then mark the commits counted before it durable.
		covered := f.ordered.Load()
		if err := f.flush(); err != nil {
			return err
		}
		f.MarkDurable(covered)
		return nil
	}, nil)
}

// Fail records err as the file's failure, as FailStop.Fail does. A store
// calls it when a write it made durable itself fails, such as a full commit's
// own fsync. It waits for the calls Admit is running on every flusher of the
// file, so it must not be called from one.
func (f *OrderedFlusher) Fail(err error) error {
	return f.stop.Fail(err)
}

// Admit runs call unless the file failed, as FailStop.Admit does, so each call
// any flusher of the file admits ends before a failure or does not start. A
// store runs through it each commit that makes writes visible before a flush,
// and each write to its file that could carry a failed commit, such as a
// checkpoint. call must not call Admit, Fail or Flushing.
func (f *OrderedFlusher) Admit(call func() error) error {
	return f.stop.Admit(call)
}

// Flushing runs call, a commit or write that flushes the file the flush
// does, after every earlier flush has ended and recorded its failure, unless
// a flush failed, and returns the failure instead. It records an error of
// call for which storageErr reports true as a failed flush, and returns call's
// other errors as they are, such as an error refusing a closed transaction.
// call must not call Sync, Close, Admit, Fail or Flushing.
func (f *OrderedFlusher) Flushing(call func() error, storageErr func(error) bool) error {
	return f.stop.Flushing(call, storageErr)
}

// WaitDurable waits until every ordered commit counted before the call is
// durable, without forcing a flush. It returns the deadline flush error if
// that flush fails first.
func (f *OrderedFlusher) WaitDurable(ctx context.Context) error {
	target := f.ordered.Load()
	for {
		var waitCh <-chan struct{}
		var err error
		done := false
		f.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if f.durable.Load() >= target {
				done = true
				return
			}
			err = f.Failed()
			if err == nil {
				err = f.flushErr
			}
			waitCh = getWaitCh()
		})
		if done || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-f.stop.Done():
		case <-waitCh:
		}
	}
}

// MarkDurable records that the first n ordered commits are durable. After a
// failed flush it marks nothing, since a later flush need not hold them.
func (f *OrderedFlusher) MarkDurable(n uint64) {
	// Advance the durable count unless a later flush already passed n or a
	// flush failed.
	if f.Failed() != nil {
		return
	}
	for {
		durable := f.durable.Load()
		if durable >= n {
			return
		}
		if f.durable.CompareAndSwap(durable, n) {
			break
		}
	}

	// Wake the waiters and clear a deadline failure the flush recovered from.
	f.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		f.flushErr = nil
		broadcast()
	})
}

// Close stops the deadline and flushes pending ordered commits.
func (f *OrderedFlusher) Close() error {
	f.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		f.closed = true
		if f.timer != nil {
			f.timer.Stop()
			f.timer = nil
		}
	})
	return f.Sync(context.Background())
}

// flushDeadline flushes the ordered commits pending when the deadline fired.
func (f *OrderedFlusher) flushDeadline() {
	f.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		f.timer = nil
	})
	err := f.Sync(context.Background())
	f.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if err != nil {
			f.flushErr = err
			broadcast()
		}
	})
}
