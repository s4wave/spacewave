package provider_spacewave

import (
	"context"
	"sync"

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

// asyncCallbackJobs runs independent jobs bound to one lifecycle context.
// Clearing or replacing the context stops the running jobs and waits for them.
type asyncCallbackJobs struct {
	mtx  sync.Mutex
	ctx  context.Context
	run  func(context.Context)
	next uint64
	jobs map[uint64]*routine.RoutineContainer
}

// newAsyncCallbackJobs builds jobs whose Trigger runs run.
func newAsyncCallbackJobs(run func(context.Context)) *asyncCallbackJobs {
	if run == nil {
		run = func(context.Context) {}
	}
	return &asyncCallbackJobs{
		run:  run,
		jobs: make(map[uint64]*routine.RoutineContainer),
	}
}

// SetContext binds new jobs to ctx and stops the jobs of the prior context.
func (j *asyncCallbackJobs) SetContext(ctx context.Context) {
	var stop []*routine.RoutineContainer
	j.mtx.Lock()
	if j.ctx == ctx {
		j.mtx.Unlock()
		return
	}
	if j.ctx != nil {
		stop = j.takeJobsLocked()
	}
	j.ctx = ctx
	j.mtx.Unlock()
	stopAsyncCallbackJobs(stop)
}

// ClearContext stops every job and drops later ones until the next context.
func (j *asyncCallbackJobs) ClearContext() {
	j.SetContext(nil)
}

// Trigger starts one run of the configured callback.
func (j *asyncCallbackJobs) Trigger() {
	j.Go(j.run)
}

// Go starts run as one job on the current context. It is dropped when no
// context is bound.
func (j *asyncCallbackJobs) Go(run func(context.Context)) {
	// Drop the job when no live context is bound.
	j.mtx.Lock()
	defer j.mtx.Unlock()
	if j.ctx == nil || j.ctx.Err() != nil {
		return
	}

	// Track the job until it exits so a context change can stop it.
	j.next++
	key := j.next
	rc := routine.NewRoutineContainer(routine.WithExitCb(func(error) {
		j.mtx.Lock()
		delete(j.jobs, key)
		j.mtx.Unlock()
	}))
	rc.SetRoutine(func(ctx context.Context) error {
		run(ctx)
		return nil
	})
	j.jobs[key] = rc
	rc.SetContext(j.ctx, false)
}

// Pending returns the number of running jobs.
func (j *asyncCallbackJobs) Pending() int {
	j.mtx.Lock()
	defer j.mtx.Unlock()
	return len(j.jobs)
}

func (j *asyncCallbackJobs) takeJobsLocked() []*routine.RoutineContainer {
	stop := make([]*routine.RoutineContainer, 0, len(j.jobs))
	for key, rc := range j.jobs {
		delete(j.jobs, key)
		stop = append(stop, rc)
	}
	return stop
}

func stopAsyncCallbackJobs(jobs []*routine.RoutineContainer) {
	wait := make([]<-chan struct{}, 0, len(jobs))
	for _, rc := range jobs {
		waitCh, _ := rc.SetRoutine(nil)
		if waitCh != nil {
			wait = append(wait, waitCh)
		}
		rc.ClearContext()
	}
	for _, ch := range wait {
		<-ch
	}
}
