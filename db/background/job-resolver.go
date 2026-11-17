package background

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/s4wave/spacewave/db/block"
)

// jobResolver runs one RunJob: it waits for a run slot, runs an attempt as
// background work, and retries after a failure until an attempt succeeds or
// the directive is released. It publishes each transition as the value.
type jobResolver struct {
	// c is the controller that owns the run slots.
	c *Controller
	// dir is the directive being resolved.
	dir RunJob
	// valID is the id of the published state, zero before the first. Only
	// Resolve uses it.
	valID uint32
}

// newJobResolver constructs a resolver for one RunJob.
func newJobResolver(c *Controller, dir RunJob) *jobResolver {
	return &jobResolver{c: c, dir: dir}
}

// Resolve runs the job until an attempt succeeds or ctx ends.
func (r *jobResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Start each resolution with fresh state and backoff.
	r.valID = 0
	bo := r.c.conf.GetRetryBackoff().Construct()
	run := r.dir.RunJobFunc()

	// Run attempts until one succeeds.
	for attempt := uint32(1); ; attempt++ {
		// Wait for a run slot.
		r.publish(handler, &JobState{State: &JobState_Waiting{Waiting: &JobWaiting{}}})
		if err := r.c.slots.Acquire(ctx, 1); err != nil {
			return context.Canceled
		}

		// Run one attempt as background work.
		r.publish(handler, &JobState{State: &JobState_Running{Running: &JobRunning{Attempt: attempt}}})
		err := run(block.WithBackground(ctx))
		r.c.slots.Release(1)
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err == nil {
			r.publish(handler, &JobState{State: &JobState_Done{Done: &JobDone{}}})
			return nil
		}

		// Wait out the backoff. A job is standing demand, so a finite backoff
		// starts over instead of giving up.
		delay := bo.NextBackOff()
		if delay == backoff.Stop {
			bo.Reset()
			delay = bo.NextBackOff()
		}
		r.c.le.WithError(err).WithField("job", r.dir.RunJobKey()).Warnf("background job failed: retrying in %s", delay)
		r.publish(handler, &JobState{State: &JobState_Retrying{Retrying: &JobRetrying{
			Error:   err.Error(),
			RetryAt: timestamppb.New(time.Now().Add(delay)),
		}}})
		select {
		case <-ctx.Done():
			return context.Canceled
		//gostyle:timer backoff
		case <-time.After(delay):
		}
	}
}

// publish replaces the published state with state.
func (r *jobResolver) publish(handler directive.ResolverHandler, state *JobState) {
	if r.valID != 0 {
		_, _ = handler.RemoveValue(r.valID)
	}
	r.valID, _ = handler.AddValue(state)
}

// _ is a type assertion
var _ directive.Resolver = (*jobResolver)(nil)
