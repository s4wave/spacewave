package kvtx

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// OrderedFlusher makes a store's ordered commits durable in groups. The store
// counts each ordered commit after it completes, and a flush covers every
// commit counted before it began. One flush runs at a time; callers that queue
// behind it find their commits covered or share the next one. An ordered
// commit becomes durable at the first of: Sync, a full commit the store
// reports with MarkDurable, the deadline after the first pending ordered
// commit, or Close.
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
	// flushMtx serializes flushes.
	flushMtx sync.Mutex

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
// and flushes a pending ordered commit within deadline.
func NewOrderedFlusher(flush func() error, deadline time.Duration) *OrderedFlusher {
	return &OrderedFlusher{flush: flush, deadline: deadline}
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
func (f *OrderedFlusher) Sync(ctx context.Context) error {
	// Skip the flush when the counted commits are already durable.
	target := f.ordered.Load()
	if f.durable.Load() >= target {
		return nil
	}

	// Flush once for every commit counted by the time this flush begins.
	f.flushMtx.Lock()
	defer f.flushMtx.Unlock()
	if f.durable.Load() >= target {
		return nil
	}
	covered := f.ordered.Load()
	if err := f.flush(); err != nil {
		return err
	}
	f.MarkDurable(covered)
	return nil
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
			err = f.flushErr
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

// MarkDurable records that the first n ordered commits are durable.
func (f *OrderedFlusher) MarkDurable(n uint64) {
	// Advance the durable count unless a later flush already passed n.
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
