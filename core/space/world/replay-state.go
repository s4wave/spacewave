package space_world

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
)

// newReplayOpening returns the replay state before the driver's first step.
func newReplayOpening() *sobject.SOReplayState {
	return &sobject.SOReplayState{State: &sobject.SOReplayState_Opening{Opening: &sobject.SOReplayOpening{}}}
}

// ObserveReplay takes the replay state the driver reached, waking serve, the
// writes waiting while replay opens, and any watcher on a change. A wait for a
// handler lasts only while the state waits for one: on any other state it ends.
// The driver calls ObserveReplay on its goroutine, which owns the handler wait.
func (a *Adapter) ObserveReplay(state *sobject.SOReplayState) {
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.replayState.EqualVT(state) {
			return
		}
		a.replayState = state
		broadcast()
	})
	switch state.GetWaiting().GetState().(type) {
	case *sobject.SOReplayWait_Handler, *sobject.SOReplayWait_Approval:
	default:
		a.endHandlerWait()
	}
}

// gateWrite runs admit, which may be nil, under the hold of bcast that finds
// replay caught up. While replay opens it waits for the state to move, until
// ctx ends. In any wait it returns a *sobject.WriteWait, whose Wake closes on
// the next change of the Adapter's state, without running admit.
//
// Invariant: a write is admitted only on a World that applied every operation
// ordered before it that this device has seen. While replay opens or waits at
// any position, the Space takes no write on this device. A turn builds on the
// installed head, which lacks the operation replay waits at and every one after
// it, and a recorded write applied after them would replace what they wrote,
// such as an object root, with nothing to detect the replacement. Opening holds
// the write rather than refuse it, as replay of what this device already holds
// usually reaches the end of its order in moments.
func (a *Adapter) gateWrite(ctx context.Context, admit func()) error {
	for {
		var opening bool
		var wait *sobject.WriteWait
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			switch {
			case a.replayState.GetCaughtUp() != nil:
				if admit != nil {
					admit()
				}
			case a.replayState.GetOpening() != nil:
				opening, waitCh = true, getWaitCh()
			default:
				wait = &sobject.WriteWait{Wake: getWaitCh()}
			}
		})
		if wait != nil {
			return wait
		}
		if !opening {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// _ is a type assertion
var _ sobject.ReplayObserver = (*Adapter)(nil)
