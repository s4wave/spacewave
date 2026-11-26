package sobject

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// BodyHost runs the BodyDriver of the body a SharedObject carries, for the
// provider that hosts the SharedObject. The provider implements SetBody with
// it. With a checkpointer it also checkpoints as the driver owes, and votes in
// the group's decisions with the driver as the checkpoint judge.
type BodyHost struct {
	// le is the logger.
	le *logrus.Entry
	// so is the hosted SharedObject.
	so SharedObject
	// report receives what the health shows of the replay, or is nil.
	report func(*BodyReport)
	// checkpointer signs as the local participant, or is nil for a host that
	// signs nothing.
	checkpointer *BodyCheckpointer
	// rc runs the driver of the current body.
	rc *routine.StateRoutineContainer[Body]
	// bcast guards body and wakes the attaches waiting for its release.
	bcast broadcast.Broadcast
	// body is the attached body, or nil.
	body Body
}

// NewBodyHost returns the body host of so. report receives what the health
// shows of the replay, and may be nil. checkpointer signs and votes as the
// local participant for the host's life, or is nil for a host that signs
// nothing.
func NewBodyHost(le *logrus.Entry, so SharedObject, report func(*BodyReport), checkpointer *BodyCheckpointer) *BodyHost {
	h := &BodyHost{le: le, so: so, report: report, checkpointer: checkpointer}
	h.rc = routine.NewStateRoutineContainer[Body](
		func(a, b Body) bool { return a == b },
		routine.WithExitLogger(le.WithField("routine", "body-driver")),
	)
	h.rc.SetStateRoutine(h.execute)
	return h
}

// SetContext runs the body's driver under ctx, until the next SetContext or
// ClearContext.
func (h *BodyHost) SetContext(ctx context.Context) {
	h.rc.SetContext(ctx, true)
}

// ClearContext stops the driver.
func (h *BodyHost) ClearContext() {
	h.rc.ClearContext()
}

// SetBody waits until no body is attached, then claims it for body.
// stop cancels and joins the driver and voting work while retaining the claim.
// release stops the body before freeing its claim for the next body.
// Both functions may be called repeatedly, including concurrently.
// Returns ctx's error if it ends before the body attaches.
func (h *BodyHost) SetBody(ctx context.Context, body Body) (stop, release func(), err error) {
	// Wait for the previous body's release, then attach it.
	for {
		var attached bool
		var wait <-chan struct{}
		h.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if h.body == nil {
				h.body, attached = body, true
				h.rc.SetState(body)
				return
			}
			wait = getWaitCh()
		})
		if attached {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, context.Cause(ctx)
		case <-wait:
		}
	}

	// Stop and join all body calls without letting another body attach.
	var stopped, released bool
	var exited <-chan struct{}
	stop = func() {
		// Retain the first stop's join channel for concurrent and later calls.
		var wait <-chan struct{}
		h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if !stopped {
				exited, _, _, _ = h.rc.SetState(nil)
				stopped = true
			}
			wait = exited
		})

		// Join outside the claim lock, including a driver already canceled.
		if wait != nil {
			<-wait
		}
	}

	// Free the claim only after its driver and its owner's cleanup ended.
	release = func() {
		// Join the body before publishing the claim's release.
		stop()

		// Wake the next body only after the previous body stopped.
		h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if !released {
				released = true
				h.body = nil
				broadcast()
			}
		})
	}
	return stop, release, nil
}

// execute drives body, and votes with it as the judge when the host signs.
func (h *BodyHost) execute(ctx context.Context, body Body) error {
	// Build the driver.
	d := NewBodyDriver(h.le, h.so, body, h.report, h.checkpointer)
	if h.checkpointer == nil {
		return d.Execute(ctx)
	}

	// Vote in the group's decisions beside it, and join both before returning.
	control, err := newControl(h.le, h.checkpointer.host, h.checkpointer.privKey, d)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var g errgroup.Group
	g.Go(func() error {
		if err := control.Execute(ctx); err != nil && ctx.Err() == nil {
			h.le.WithError(err).Warn("stopped voting in group decisions")
		}
		return nil
	})

	// End voting when the driver returns, then join it before freeing the body.
	err = d.Execute(ctx)
	cancel()
	_ = g.Wait()
	return err
}
