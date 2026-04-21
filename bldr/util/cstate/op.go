package cstate

import (
	"context"
	"sync"
)

// CStateWriter is passed to the CState callback with a SetObj function.
type CStateWriter[T any] struct {
	*CState[T]
}

// SetObj sets the object.
func (w *CStateWriter[T]) SetObj(obj T) {
	w.obj = obj
}

// queuedOp is an operation queued for Execute to process.
type queuedOp[T any] struct {
	// ctx is the operation context
	ctx context.Context
	// resCh is written to when the operation completes.
	// if ctx is canceled, it will not be written to.
	resCh chan error
	// resOnce ensures resCh is written to only once.
	resOnce sync.Once
	// op is the operation to apply
	op func(ctx context.Context, obj *CStateWriter[T]) (dirty bool, err error)
	// dirty indicates if the state is dirty after calling the op.
	// written before resCh is resolved.
	// read only after resCh is resolved.
	dirty bool
}

// newQueuedOp builds a new remoteOp.
// be sure resCh is buffered and won't block.
// doOp is called while mtx is locked.
func newQueuedOp[T any](
	ctx context.Context,
	resCh chan error,
	op func(ctx context.Context, obj *CStateWriter[T]) (dirty bool, err error),
) *queuedOp[T] {
	return &queuedOp[T]{
		ctx:   ctx,
		resCh: resCh,
		op:    op,
	}
}

// finish marks the operation as complete.
// called by Execute
func (r *queuedOp[T]) finish(err error, dirty bool) {
	r.resOnce.Do(func() {
		r.dirty = dirty
		select {
		case r.resCh <- err:
		default:
		}
	})
}
