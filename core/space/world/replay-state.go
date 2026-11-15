package space_world

import (
	"github.com/s4wave/spacewave/core/sobject"
)

// replayGate is the adapter's copy of the replay state the body driver steps,
// which decides whether the Space takes writes. The driver is its only
// writer, through ObserveReplay; serve reads it to publish the engine, and
// Admit and writeOp to refuse a write. The adapter's bcast guards it.
//
// Invariant: a write is admitted only on a World that applied every operation
// ordered before it that this device has seen; while replay opens or waits at
// any position, the Space is read only on this device. A turn builds on the
// installed head, which lacks the operation replay waits at and every one
// after it, and a recorded write applied after them would replace what they
// wrote, such as an object root, with nothing to detect it.
type replayGate struct {
	// state is the driver's last replay state, nil before its first step.
	state *sobject.SOReplayState
	// caughtUp is closed on the next move to caught up while the state is not
	// caught up, and nil while it is.
	caughtUp chan struct{}
}

// newReplayGate returns the gate before the driver's first step.
func newReplayGate() replayGate {
	return replayGate{caughtUp: make(chan struct{})}
}

// set moves the gate to state, closing caughtUp on a move to caught up and
// opening a new one on a move away from it.
func (g *replayGate) set(state *sobject.SOReplayState) {
	was := g.caughtUp == nil
	g.state = state
	switch now := state.GetCaughtUp() != nil; {
	case now && !was:
		close(g.caughtUp)
		g.caughtUp = nil
	case !now && was:
		g.caughtUp = make(chan struct{})
	}
}

// writeWait returns the wait a write meets, nil while caught up. Its Wake
// closes once replay is caught up again. It carries no Wait: what replay
// waits for is published once, in the Space's health.
func (g *replayGate) writeWait() *sobject.BodyWait {
	if g.caughtUp == nil {
		return nil
	}
	return &sobject.BodyWait{Wake: g.caughtUp}
}

// ObserveReplay takes the replay state the driver reached, waking serve and
// any watcher on a change.
func (a *Adapter) ObserveReplay(state *sobject.SOReplayState) {
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.replayGate.state.EqualVT(state) {
			return
		}
		a.replayGate.set(state)
		broadcast()
	})
}

// writeWait returns the wait a write meets while replay is not caught up, or
// nil. Admit and writeOp check it again where they admit, so a wait that
// began after Begin still refuses the write.
func (a *Adapter) writeWait() error {
	var wait *sobject.BodyWait
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		wait = a.replayGate.writeWait()
	})
	if wait == nil {
		return nil
	}
	return wait
}

// _ is a type assertion
var _ sobject.ReplayObserver = (*Adapter)(nil)
