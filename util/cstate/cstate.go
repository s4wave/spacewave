package cstate

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/sync/semaphore"
)

// CState maintains an operation queue and a set of watchers.
// If any operation returns true for "dirty" the watchers are called.
//
// Allows a single operation to execute at a time.
type CState[T any] struct {
	// wake wakes the execution loop, optionally setting dirty=true.
	wake chan bool
	// opQueue is pushed with pending operations.
	// used whenever mtx needs to be locked
	opQueue chan *queuedOp[T]
	// mtx guards below fields
	mtx semaphore.Weighted
	// watchers contains the list of watchers.
	watchers []*watcher[T]
	// obj contains the state object
	obj T
}

// watcher contains a watcher callback function.
type watcher[T any] struct {
	ctx     context.Context
	changed func(ctx context.Context, state T)
}

// NewCState constructs a CState with an object.
func NewCState[T any](obj T) *CState[T] {
	return &CState[T]{
		wake:    make(chan bool, 1),
		opQueue: make(chan *queuedOp[T], 1),
		obj:     obj,
		mtx:     *semaphore.NewWeighted(1),
	}
}

// Obj returns the controlled object.
func (c *CState[T]) Obj() T {
	return c.obj
}

// View calls a callback while the mutex is locked.
func (c *CState[T]) View(
	ctx context.Context,
	cb func(ctx context.Context, value T) error,
) (err error) {
	err = c.mtx.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	defer func() {
		if perr := recover(); perr != nil {
			if err == nil {
				err, _ = perr.(error)
			}
			if err == nil {
				err = errors.New("view callback paniced")
			}
		}
		c.mtx.Release(1)
	}()
	return cb(ctx, c.obj)
}

// Wait waits for the callback to return true, nil before returning.
// Returns nil only if the callback returned true, nil.
func (c *CState[T]) Wait(
	ctx context.Context,
	cb func(ctx context.Context, val T) (bool, error),
) error {
	resultCh := make(chan error, 1)
	rel, err := c.AddWatcher(ctx, true, func(ctx context.Context, state T) {
		matched, err := cb(ctx, state)
		if err != nil || matched {
			select {
			case resultCh <- err:
			default:
			}
		}
	})
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		rel()
		return ctx.Err()
	case err := <-resultCh:
		rel()
		return err
	}
}

// Wake wakes the execution loop.
func (c *CState[T]) Wake(setDirty bool) {
	select {
	case c.wake <- setDirty:
	default:
	}
}

// AddWatcher adds a watcher callback function.
// The state will be locked while the watcher executes.
// Called if the state changes (dirty=true).
// If initial=true, calls with initial value immediately.
// Returns a function to remove the watcher.
// The ctx is used to call the watcher, if canceled, watcher is removed.
// Returns an error only if ctx is canceled.
func (c *CState[T]) AddWatcher(
	ctx context.Context,
	initial bool,
	cb func(ctx context.Context, state T),
) (func(), error) {
	if cb == nil {
		return func() {
			// cb is nil, no-op
		}, nil
	}
	err := c.mtx.Acquire(ctx, 1)
	if err != nil {
		return nil, err
	}
	wt := &watcher[T]{
		ctx:     ctx,
		changed: cb,
	}
	var removeOnce sync.Once
	c.watchers = append(c.watchers, wt)
	if initial {
		cb(ctx, c.obj)
	}
	c.mtx.Release(1)
	return func() {
		removeOnce.Do(func() {
			_ = c.mtx.Acquire(context.Background(), 1)
			for i, exw := range c.watchers {
				if exw == wt {
					c.watchers[i] = c.watchers[len(c.watchers)-1]
					c.watchers[len(c.watchers)-1] = nil
					c.watchers = c.watchers[:len(c.watchers)-1]
					break
				}
			}
			c.mtx.Release(1)
		})
	}, nil
}

// Apply queues & applies the given operation.
func (c *CState[T]) Apply(
	ctx context.Context,
	op func(ctx context.Context, v *CStateWriter[T]) (dirty bool, err error),
) (dirty bool, err error) {
	resCh := make(chan error, 1)
	qOp := newQueuedOp(ctx, resCh, op)
	select {
	case <-ctx.Done():
		return false, context.Canceled
	case c.opQueue <- qOp:
		c.Wake(false)
	}
	select {
	case <-ctx.Done():
		return false, context.Canceled
	case err = <-qOp.resCh:
		if err == nil {
			dirty = qOp.dirty
		}
		return dirty, err
	}
}

// Execute executes the operation loop.
// errCh can be used to interrupt the Execute loop, and can be nil.
func (c *CState[T]) Execute(ctx context.Context, errCh <-chan error) error {
	w := &CStateWriter[T]{CState: c}
	var dirty bool
	for {
		// lock mtx
		err := c.mtx.Acquire(ctx, 1)
		if err != nil {
			return err
		}

		// flush wake queue
		select {
		case setDirty := <-c.wake:
			if setDirty {
				dirty = true
			}
		default:
		}

		processOp := func(op *queuedOp[T]) (dirty bool, err error) {
			if op.op != nil {
				dirty, err = op.op(ctx, w)
			}
			return dirty, err
		}

		// process op queue
	OpQueue:
		for {
			var op *queuedOp[T]
			select {
			case op = <-c.opQueue:
			default:
				break OpQueue
			}
			if op == nil {
				continue
			}
			// mark op with result
			opDirty, err := processOp(op)
			if err == nil && opDirty {
				dirty = true
			}
			op.finish(err, opDirty)
		}

		// call watchers
		if dirty {
		WatcherLoop:
			for i := 0; i < len(c.watchers); i++ {
				wt := c.watchers[i]
				wtCtx := wt.ctx
				select {
				case <-wtCtx.Done():
					c.watchers[i] = c.watchers[len(c.watchers)-1]
					c.watchers[len(c.watchers)-1] = nil
					c.watchers = c.watchers[:len(c.watchers)-1]
					i--
					continue WatcherLoop
				default:
				}
				wt.changed(wtCtx, c.obj)
			}
		}

		// unlock
		c.mtx.Release(1)

		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case dirty = <-c.wake:
		}
	}
}
