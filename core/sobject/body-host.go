package sobject

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// BodyHost runs the BodyDriver of the body a SharedObject carries, for the
// provider that hosts the SharedObject. The provider implements SetBody with
// it. With a signer it also checkpoints as the driver owes, and votes in the
// group's decisions with the driver as the checkpoint judge.
type BodyHost struct {
	// le is the logger.
	le *logrus.Entry
	// so is the hosted SharedObject.
	so SharedObject
	// report receives what the health shows of the replay, or is nil.
	report func(*BodyReport)
	// host holds the state, or is nil for a host that signs nothing.
	host *SOHost
	// privKey signs as the local participant when host is set.
	privKey crypto.PrivKey
	// sfs builds the key epoch transforms when host is set.
	sfs *block_transform.StepFactorySet
	// deadline is the checkpointer's deadline when host is set.
	deadline time.Duration
	// rc runs the driver of the current body.
	rc *routine.StateRoutineContainer[Body]
	// bcast guards body and wakes the attaches waiting for its release.
	bcast broadcast.Broadcast
	// body is the attached body, or nil.
	body Body
}

// NewBodyHost returns the body host of so. report receives what the health
// shows of the replay, and may be nil.
func NewBodyHost(le *logrus.Entry, so SharedObject, report func(*BodyReport)) *BodyHost {
	h := &BodyHost{le: le, so: so, report: report}
	h.rc = routine.NewStateRoutineContainer[Body](
		func(a, b Body) bool { return a == b },
		routine.WithExitLogger(le.WithField("routine", "body-driver")),
	)
	h.rc.SetStateRoutine(h.execute)
	return h
}

// SetSigner has the host checkpoint and vote as the participant privKey,
// through host, checkpointing with the deadline BodyDriver.SetCheckpointer
// describes. Call it before SetContext.
func (h *BodyHost) SetSigner(host *SOHost, privKey crypto.PrivKey, sfs *block_transform.StepFactorySet, deadline time.Duration) {
	h.host, h.privKey, h.sfs, h.deadline = host, privKey, sfs, deadline
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
	d := NewBodyDriver(h.le, h.so, body, h.report)
	if h.host == nil {
		return d.Execute(ctx)
	}
	d.SetCheckpointer(h.host, h.privKey, h.sfs, h.deadline)

	// Vote in the group's decisions beside it, and join both before returning.
	control, err := newControl(h.le, h.host, h.privKey, d)
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
