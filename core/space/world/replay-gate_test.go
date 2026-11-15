package space_world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// replayHandlerWait is replay's state while it waits for a handler.
var replayHandlerWait = &sobject.SOReplayState{State: &sobject.SOReplayState_Waiting{Waiting: &sobject.SOReplayWait{
	OpTypeId: "test/op",
	State:    &sobject.SOReplayWait_Handler{Handler: &sobject.SOReplayWaitHandler{}},
}}}

// replayCaughtUp is replay's state at the end of its order.
var replayCaughtUp = &sobject.SOReplayState{State: &sobject.SOReplayState_CaughtUp{CaughtUp: &sobject.SOReplayCaughtUp{}}}

// TestWriteWaitsWhileReplayOpens holds a write at the gate of a real Adapter
// while replay opens, then moves replay through ObserveReplay, as the driver
// does: the write is admitted once replay is caught up, refused with the
// typed wait when replay enters a wait instead, and ends with its context
// otherwise. A refused or ended write is never admitted.
func TestWriteWaitsWhileReplayOpens(t *testing.T) {
	cases := []struct {
		name string
		// next moves replay, or cancels the write's context.
		next func(a *Adapter, cancel context.CancelFunc)
		// admitted reports whether the write is admitted.
		admitted bool
		// want matches the write's error, nil when admitted.
		want error
	}{
		{"caught up", func(a *Adapter, _ context.CancelFunc) { a.ObserveReplay(replayCaughtUp) }, true, nil},
		{"enters a wait", func(a *Adapter, _ context.CancelFunc) { a.ObserveReplay(replayHandlerWait) }, false, sobject.ErrReplayWaits},
		{"context ends", func(_ *Adapter, cancel context.CancelFunc) { cancel() }, false, context.Canceled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Start a write while replay opens.
			a, _ := newVolumeAdapter(t, 1<<20, time.Hour)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			adm := a.newAdmission(world.Candidate{}, []byte("op"), nil, nil)
			done := make(chan error, 1)
			go func() { done <- a.admitWrite(ctx, adm) }()

			// It waits.
			select {
			case err := <-done:
				t.Fatalf("a write while replay opens returned %v, want it held", err)
			case <-time.After(50 * time.Millisecond):
			}

			// Move replay: the write ends as the next state decides.
			c.next(a, cancel)
			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the held write never ended")
			}
			if (c.want == nil) != (err == nil) || (c.want != nil && !errors.Is(err, c.want)) {
				t.Fatalf("write = %v, want %v", err, c.want)
			}
			var pending int
			a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { pending = len(a.pending) })
			if admitted := pending == 1; admitted != c.admitted {
				t.Fatalf("write admitted = %v, want %v", admitted, c.admitted)
			}
		})
	}
}

// TestHandlerWatchFollowsReplayState checks that the handler watch lasts only
// while the replay state waits for a handler: a move to any other state ends
// it, and a handler or approval wait keeps it.
func TestHandlerWatchFollowsReplayState(t *testing.T) {
	a, _ := newVolumeAdapter(t, 1<<20, time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a.handlerWait = &handlerWait{opTypeID: "test/op", cancel: cancel}

	// A handler wait keeps the watch.
	a.ObserveReplay(replayHandlerWait)
	if a.handlerWait == nil || ctx.Err() != nil {
		t.Fatal("a handler wait ended the handler watch")
	}

	// A gap ends it.
	gap := &sobject.SOReplayState{State: &sobject.SOReplayState_Waiting{Waiting: &sobject.SOReplayWait{
		State: &sobject.SOReplayWait_Gap{Gap: &sobject.SOReplayWaitGap{}},
	}}}
	a.ObserveReplay(gap)
	if a.handlerWait != nil || ctx.Err() == nil {
		t.Fatal("the handler watch outlived the handler wait")
	}
}
