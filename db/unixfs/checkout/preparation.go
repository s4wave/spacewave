package unixfs_checkout

import (
	"context"
	"path/filepath"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/filelock"
)

// registry is the set of running checkouts.
type registry struct {
	// ctx owns workers independently of each subscribing caller.
	ctx context.Context
	// bcast guards every field below and every job's, and wakes waiters on
	// each change. Nothing holds it across I/O or a caller's callback.
	bcast broadcast.Broadcast
	// jobs holds each running or stopping worker by key. A worker leaves when
	// it exits, so a later caller checks the published directory again.
	jobs map[string]*job
}

// job is one worker writing one checkout.
type job struct {
	// subscribers counts the callers waiting on the result.
	subscribers int
	// foreground counts those callers that are not background prefetches.
	foreground int
	// progress is the worker's latest report.
	progress Progress
	// stop cancels the worker, once subscribers reaches zero.
	stop context.CancelFunc
	// stopping is set once stop is called. A stopping job takes no new
	// subscribers: a later caller waits for it to exit and starts afresh.
	stopping bool
	// exited is set when the worker returned and released its lease.
	exited bool
	// err is the worker's result once exited.
	err error
}

// Fetch is how a caller of Checkout reaches content and what it observes.
type Fetch struct {
	// Remotes opens the stores to read blocks from, first to last, for the
	// worker that writes the checkout. It runs with the worker's context, so
	// a caller that leaves early cannot close a store the worker still reads,
	// and release runs when the worker exits.
	Remotes func(ctx context.Context) (remotes []block.StoreOps, release func())
	// Background marks a prefetch: its remote reads wait while a foreground
	// caller waits on another checkout, and resume when that one finishes.
	Background bool
	// Progress observes the checkout's phase and bytes. It runs on
	// the caller's goroutine, may skip intermediate reports, and always sees
	// the last one before Checkout returns successfully.
	Progress func(Progress)
}

// work runs one checkout attempt for a worker under its destination lease.
// ctx ends when the last caller leaves. report publishes progress.
type work func(ctx context.Context, report func(Progress), yield func(context.Context) error) error

// run waits for the checkout key names, starting its worker unless one is
// running, and returns the worker's result or ctx's end. Leaving early drops
// only this caller; the worker stops when no caller remains, and a caller
// arriving while it stops waits for its exit, so two workers never overlap.
func (r *registry) run(ctx context.Context, key string, fetch Fetch, do work) error {
	// Join or start the shared attempt for this subscriber.
	j, err := r.subscribe(ctx, key, !fetch.Background, do)
	if err != nil {
		return err
	}
	defer r.unsubscribe(j, !fetch.Background)

	// Report each change until the worker exits.
	var shown Progress
	reported := false
	for {
		lock := r.bcast.Lock()
		progress, exited, result, wait := j.progress, j.exited, j.err, lock.WaitCh()
		lock.Unlock()
		if fetch.Progress != nil && (!reported || progress != shown) {
			fetch.Progress(progress)
			shown, reported = progress, true
		}
		if exited {
			return result
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-wait:
		}
	}
}

// subscribe joins the running job for key, waiting out one that is stopping,
// or starts a new one.
func (r *registry) subscribe(ctx context.Context, key string, foreground bool, do work) (*job, error) {
	for {
		lock := r.bcast.Lock()
		j := r.jobs[key]
		if j != nil && j.stopping {
			wait := lock.WaitCh()
			lock.Unlock()
			select {
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			case <-wait:
			}
			continue
		}
		if j == nil {
			j = r.start(key, do)
		}
		j.subscribers++
		if foreground {
			j.foreground++
		}
		lock.Broadcast()
		lock.Unlock()
		return j, nil
	}
}

// start registers a job for key and launches its worker. The caller holds
// the registry lock.
func (r *registry) start(key string, do work) *job {
	// Start a worker whose lifetime follows all subscribers.
	ctx, stop := context.WithCancel(r.ctx)
	j := &job{stop: stop}
	r.jobs[key] = j
	report := func(progress Progress) {
		lock := r.bcast.Lock()
		j.progress = progress
		lock.Broadcast()
		lock.Unlock()
	}
	go func() {
		// Publish the worker's final result before waking its subscribers.
		err := do(ctx, report, func(ctx context.Context) error { return r.yield(ctx, j) })
		stop()
		lock := r.bcast.Lock()
		j.exited, j.err = true, err
		if r.jobs[key] == j {
			delete(r.jobs, key)
		}
		lock.Broadcast()
		lock.Unlock()
	}()
	return j
}

// unsubscribe drops one caller of j, stopping its worker when none remain.
func (r *registry) unsubscribe(j *job, foreground bool) {
	// Remove this subscriber and cancel only the last one's worker.
	lock := r.bcast.Lock()
	defer lock.Unlock()
	j.subscribers--
	if foreground {
		j.foreground--
	}
	if j.subscribers == 0 && !j.exited {
		j.stopping = true
		j.stop()
	}
	lock.Broadcast()
}

// yield holds j's next remote read while no foreground caller waits on j and
// one waits on another job that needs the network. A job waiting for its
// destination's lease does not, so a prefetch holding that lease finishes. A
// read already issued finishes first.
func (r *registry) yield(ctx context.Context, j *job) error {
	for {
		lock := r.bcast.Lock()
		held := j.foreground == 0 && r.demanded()
		wait := lock.WaitCh()
		lock.Unlock()
		if !held {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-wait:
		}
	}
}

// demanded reports whether a foreground caller waits on a job that is not
// waiting for its lease. The caller holds the registry lock; the registry
// holds a few jobs at most.
func (r *registry) demanded() bool {
	for _, j := range r.jobs {
		if j.foreground != 0 && j.progress.Phase != PhaseWaiting {
			return true
		}
	}
	return false
}

// DestinationLease returns the lease scope that excludes every other writer
// of directory, in this process or another, below root. Checkouts hold it
// from their reuse check through cleanup; eviction probes it.
func DestinationLease(root, directory string) (*filelock.Coordinator, coord.Scope, error) {
	key, err := filepath.Rel(root, directory)
	if err != nil {
		return nil, coord.Scope{}, err
	}
	// Keyed scopes never reach the inner coordinator.
	return filelock.NewCoordinator(root, root, nil), coord.Scope{Key: filepath.ToSlash(key)}, nil
}
