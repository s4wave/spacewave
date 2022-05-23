package web_runtime

import (
	"context"
	"sync"
)

// remoteOpFn is an operation function called with mtx locked.
// ctx is the context from Execute.
// returns dirty, error
type remoteOpFn func(ctx context.Context, r *Remote) (bool, error)

// remoteOp is an operation queued for Execute to process.
type remoteOp struct {
	// ctx is the operation context
	ctx context.Context
	// resCh is written to when the operation completes.
	// if ctx is canceled, it will not be written to.
	resCh chan error
	// resOnce ensures resCh is written to only once.
	resOnce sync.Once
	// opFn is the remote op fn
	opFn remoteOpFn
}

// newRemoteOp builds a new remoteOp.
// be sure resCh is buffered and won't block.
// doOp is called while mtx is locked.
func newRemoteOp(ctx context.Context, resCh chan error, opFn remoteOpFn) *remoteOp {
	return &remoteOp{
		ctx:   ctx,
		resCh: resCh,
		opFn:  opFn,
	}
}

// execRemoteOp creates and executes a remote op.
func execRemoteOp(ctx context.Context, r *Remote, opFn remoteOpFn) error {
	resCh := make(chan error, 1)
	op := newRemoteOp(ctx, resCh, opFn)
	return r.waitRemoteOp(op)
}

// finish marks the operation as complete.
// called by Execute
func (r *remoteOp) finish(err error) {
	r.resOnce.Do(func() {
		select {
		case r.resCh <- err:
		default:
		}
	})
}
