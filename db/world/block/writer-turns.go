package world_block

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
)

// WriterTurns counts the write turns of one World in this process, so that
// background work sharing the CPU, the store and the caches with its writers,
// such as a regeneration's build, can yield to them between short slices. A
// writer then runs without the work beside it, and the work still runs one
// slice between cohorts of writers, so it slows under constant writes but
// never stops. The zero value is ready to use.
type WriterTurns struct {
	// bcast guards begun and ended and wakes Yield when a turn ends.
	bcast broadcast.Broadcast
	// begun counts the turns begun, waiting ones included, and ended those
	// that ended. bcast guards both.
	begun, ended uint64
	// busy is set while begun exceeds ended, so a background slice checks
	// for writers without taking a lock.
	busy atomic.Bool
}

// Begin counts a turn. Call it before the turn waits for its lock, so
// background work yields to a writer still waiting, and call End once for
// each Begin.
func (w *WriterTurns) Begin() {
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		w.begun++
		w.busy.Store(true)
	})
}

// End counts a turn that ended and wakes the yielding background work.
func (w *WriterTurns) End() {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		w.ended++
		w.busy.Store(w.begun != w.ended)
		broadcast()
	})
}

// Yield waits until as many turns have ended as had begun when it was
// called. A caller that holds a turn must not call it: it would wait for its
// own turn. With no turn begun it returns after one atomic load.
func (w *WriterTurns) Yield(ctx context.Context) error {
	if !w.busy.Load() {
		return nil
	}
	var target uint64
	var set bool
	return w.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if !set {
			target, set = w.begun, true
		}
		return w.ended >= target, nil
	})
}
