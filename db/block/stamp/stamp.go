// Package stamp orders events within one process by a stamp that reads as a
// wall-clock time but never steps backward.
//
// A pack sweep decides whether an operation began before a pack was held by
// comparing two times. Two wall-clock readings would let a backward clock step
// put a later operation before an earlier hold, and the sweep would skip its
// wait. Stamps taken from Next order those events as they happened, since
// every stamp is greater than every stamp taken before it in the process.
package stamp

import (
	"sync/atomic"
	"time"
)

var (
	// last is the greatest stamp issued or raised to.
	last atomic.Int64
	// clock reads the wall clock in Unix nanoseconds.
	clock atomic.Pointer[func() int64]
)

// Next returns a stamp in Unix nanoseconds: the wall clock, or one past the
// greatest stamp before it when the clock reads lower. A stamp taken after
// another returns, by any goroutine, is greater than it.
func Next() int64 {
	// Advance last to the clock or past itself, whichever is greater.
	now := wall()
	for {
		prev := last.Load()
		next := max(now, prev+1)
		if last.CompareAndSwap(prev, next) {
			return next
		}
	}
}

// Raise makes every later stamp greater than v. A process that loads events
// stamped before a restart raises the floor to them, so its own stamps order
// after them even when the clock stepped back across the restart.
func Raise(v int64) {
	for {
		prev := last.Load()
		if v <= prev || last.CompareAndSwap(prev, v) {
			return
		}
	}
}

// SetClock replaces the wall clock Next reads and returns the function that
// restores it. Tests use it to step the clock.
func SetClock(read func() int64) (restore func()) {
	prev := clock.Swap(&read)
	return func() { clock.Store(prev) }
}

// wall reads the clock.
func wall() int64 {
	if read := clock.Load(); read != nil {
		return (*read)()
	}
	return time.Now().UnixNano()
}
