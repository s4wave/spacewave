package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/sirupsen/logrus"
)

func newNamedRoutineContainer(le *logrus.Entry, name string) *routine.RoutineContainer {
	if le == nil {
		return routine.NewRoutineContainer()
	}
	return routine.NewRoutineContainerWithLogger(le.WithField("routine", name))
}

type coalescedTriggerRoutine struct {
	rc      *routine.RoutineContainer
	bcast   broadcast.Broadcast
	pending bool
	run     func(context.Context)
}

func newCoalescedTriggerRoutine(
	le *logrus.Entry,
	name string,
	run func(context.Context),
) *coalescedTriggerRoutine {
	r := &coalescedTriggerRoutine{
		rc:  newNamedRoutineContainer(le, name),
		run: run,
	}
	r.rc.SetRoutine(r.execute)
	return r
}

func (r *coalescedTriggerRoutine) SetContext(ctx context.Context) {
	r.rc.SetContext(ctx, true)
}

func (r *coalescedTriggerRoutine) ClearContext() {
	r.rc.ClearContext()
}

func (r *coalescedTriggerRoutine) Trigger() {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.pending = true
		broadcast()
	})
}

func (r *coalescedTriggerRoutine) Pending() bool {
	if r == nil {
		return false
	}
	var pending bool
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = r.pending
	})
	return pending
}

func (r *coalescedTriggerRoutine) execute(ctx context.Context) error {
	for {
		if err := r.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			if !r.pending {
				return false, nil
			}
			r.pending = false
			return true, nil
		}); err != nil {
			return err
		}
		r.run(ctx)
		if err := ctx.Err(); err != nil {
			return context.Canceled
		}
	}
}
