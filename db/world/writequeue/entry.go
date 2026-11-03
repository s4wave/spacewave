package writequeue

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// queuedWrite is one caller write waiting in a Queue.
type queuedWrite struct {
	// ctx is the writer's context; its body runs under it.
	ctx context.Context
	// name observes the operation for a permanently failed replay.
	name func() string
	// queued is when the write joined the queue.
	queued time.Time
	// actor is the application actor ID of the actor the write acts for, or zero.
	actor uint64
	// checks run in the batch transaction before body, in place of the
	// checks the writer's engine runs when it opens a write.
	checks []Check
	// body is the writer's mutation.
	body func(context.Context, world.Tx) error
	// replay is a body making the same change that touches nothing of the
	// writer's, so it can run after the writer heard success, or nil. A write
	// with one succeeds at admission.
	replay func(context.Context, world.Tx) error
	// arrival numbers the write in arrival order; a write queued again keeps
	// its number.
	arrival uint64
	// answered reports that the writer heard success at an admission the
	// engine then dropped, and the write runs again without a writer.
	answered bool
	// err is the write's result, set before done closes.
	err error
	// done closes when the write has its result.
	done chan struct{}
	// runMtx is held while the checks and body run, and by a writer that
	// leaves while it marks the write abandoned.
	runMtx sync.Mutex
	// abandoned is set under runMtx once the writer stopped waiting; the
	// body never runs after it, including when a batch reruns.
	abandoned bool

	// The fields below belong to the goroutine holding the write's batch,
	// and pass to the next through mtx when the write queues again.

	// refusals counts the commits the engine refused for a stale base or a
	// failed predecessor.
	refusals int
	// limit is the most writes a batch carrying this write may hold, after
	// the engine refused a larger one as oversized, or zero for no limit.
	limit int
	// acked is set under the queue's mtx once the write succeeded at
	// admission.
	acked bool
	// seqno is the World revision the write committed, set before done
	// closes on success.
	seqno uint64
}

// Check authorizes a write inside the transaction that carries it.
type Check func(context.Context, world.WorldState) error

// batchedWriteKey holds the queue whose batch runs a body.
type batchedWriteKey struct{}

// endsClean reports whether a write that failed with err left tx as it found
// it at seqno before: its checks refused it, or its body changed nothing.
// Every World change moves the seqno, and every SQL write follows one, but a
// body can fail inside a change before the change moves the seqno, so only a
// refusing check or the configured unchanged sentinel counts.
func (q *Queue) endsClean(ctx context.Context, tx world.Tx, before uint64, checked bool, err error) (bool, error) {
	if checked && !errors.Is(err, q.options.Unchanged) {
		return false, nil
	}
	after, serr := tx.GetSeqno(ctx)
	if serr != nil {
		return false, serr
	}
	return after == before, nil
}

// run applies w's checks and body to tx, a transaction of q. A writer that
// left during its body fails it, so its partial work never commits. It
// reports whether the checks passed, so the body ran.
func (w *queuedWrite) run(q *Queue, tx Transaction) (bool, error) {
	// Run only a write whose writer still waits; a writer that leaves
	// mid-body waits here for the body to return.
	w.runMtx.Lock()
	defer w.runMtx.Unlock()
	if w.abandoned {
		return false, context.Cause(w.ctx)
	}

	// Authorize and apply the body in the attempt's auxiliary context.
	ctx := context.WithValue(tx.BodyContext(w.ctx), batchedWriteKey{}, q)
	for _, check := range w.checks {
		if err := check(ctx, tx); err != nil {
			return false, err
		}
	}
	if err := w.body(ctx, tx); err != nil {
		return true, err
	}
	return true, w.ctx.Err()
}

// finish ends w with err.
func (w *queuedWrite) finish(err error) {
	w.err = err
	close(w.done)
}

// fail ends w with err, logging the loss of a write whose writer already
// heard success.
func (w *queuedWrite) fail(err error) {
	if w.answered {
		op := w.name()
		logrus.WithError(err).WithFields(logrus.Fields{"op": op, "actor": w.actor}).Error("World write answered at admission was dropped and could not run again")
	}
	w.finish(err)
}

// ended reports whether w has its result.
func (w *queuedWrite) ended() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}
