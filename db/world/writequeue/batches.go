package writequeue

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// requeue returns writes that have no result to the queue, ahead of the
// writes that arrived after them, starting a drain when none runs.
func (q *Queue) requeue(writes []*queuedWrite) {
	q.mtx.Lock()
	q.requeueLocked(writes)
	q.mtx.Unlock()
}

// requeueLocked is requeue with mtx held.
func (q *Queue) requeueLocked(writes []*queuedWrite) {
	// Keep only the writes still waiting.
	writes = slices.DeleteFunc(slices.Clone(writes), (*queuedWrite).ended)
	if len(writes) == 0 {
		return
	}

	// Queue them in arrival order, starting a drain when none runs.
	q.pending = append(writes, q.pending...)
	slices.SortStableFunc(q.pending, func(a, b *queuedWrite) int { return cmp.Compare(a.arrival, b.arrival) })
	if !q.draining {
		q.draining = true
		go q.drain()
	}
}

// reinstalled queues again the writes of the batches the engine dropped,
// those admitted above seqno. It runs under the engine's writer lock, so no
// batch forks or is admitted while it runs: a batch counted as committing
// with no seqno yet was admitted last, and any drop includes it.
func (q *Queue) reinstalled(seqno uint64) {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	for _, c := range q.committing {
		if !c.dropped && (c.seqno == 0 || c.seqno > seqno) {
			q.dropLocked(c, coord.ErrStaleGeneration)
		}
	}
}

// dropLocked queues c's writes again after the engine dropped it, with mtx
// held: a writer still waiting keeps its write, and a write whose writer
// heard success runs again as its replay. A write past the attempt limit
// fails with err instead.
func (q *Queue) dropLocked(c *committingBatch, err error) {
	c.dropped = true
	var writes []*queuedWrite
	for _, w := range c.writes {
		if w.acked {
			w = &queuedWrite{ctx: context.WithoutCancel(w.ctx), queued: w.queued, actor: w.actor, checks: w.checks, body: w.replay, replay: w.replay, arrival: w.arrival, answered: true, refusals: w.refusals, done: make(chan struct{})}
		} else if w.ended() {
			continue
		}
		w.refusals++
		if w.refusals >= batchAttemptLimit {
			w.fail(errors.Wrap(err, "World write attempt limit exhausted"))
			continue
		}
		writes = append(writes, w)
	}
	q.requeueLocked(writes)
}

// take removes the next batch from the head of the queue: at most
// MaxBatch writes, or the head write's limit. It ends the writes whose
// writers left before the fork and leaves them out.
func (q *Queue) take() []*queuedWrite {
	// Cut the batch from the head.
	q.mtx.Lock()
	n := min(len(q.pending), MaxBatch)
	if n != 0 && q.pending[0].limit != 0 {
		n = min(n, q.pending[0].limit)
	}
	batch := slices.Clone(q.pending[:n])
	q.pending = slices.Delete(q.pending, 0, n)
	q.mtx.Unlock()

	// Drop the writes that ended or whose writers left.
	return slices.DeleteFunc(batch, func(w *queuedWrite) bool {
		if w.ended() {
			return true
		}
		if err := w.ctx.Err(); err != nil {
			w.finish(err)
			return true
		}
		return false
	})
}

// drain forks a batch for the pending writes until none remain. A batch
// transaction outlives any one writer, so it runs under no writer's context.
func (q *Queue) drain() {
	// Return a prepared claim even when every waiting writer left.
	var preparation world.AdmissionPreparation
	defer func() {
		if preparation != nil {
			preparation.Release()
		}
	}()
	ctx := q.ctx
	for {
		// Stop when no write waits.
		q.mtx.Lock()
		if len(q.pending) == 0 {
			q.draining = false
			q.mtx.Unlock()
			return
		}
		q.mtx.Unlock()

		// Fork, which waits for the engine to admit the batch before, then
		// take the writes that queued meanwhile.
		openCtx := ctx
		if preparation != nil {
			openCtx = preparation.WithContext(ctx)
		}
		wtx, err := q.engine.NewTransaction(openCtx, true)
		batch := q.take()
		if err != nil {
			if preparation != nil {
				preparation.Release()
				preparation = nil
			}
			q.refused(batch, err)
			continue
		}
		tx := q.options.Wrap(ctx, wtx)
		tx.UsePreparation(preparation)
		preparation = nil
		if len(batch) == 0 {
			tx.Discard()
			continue
		}

		// Run the bodies. A failed write leaves the rest to run again in a
		// fresh fork, since its partial changes are in the transaction.
		failed, carried, err := q.run(ctx, tx, batch)
		if err != nil || failed >= 0 {
			tx.Discard()
			if err != nil {
				q.refused(batch, err)
			} else {
				q.requeue(slices.Delete(batch, failed, failed+1))
			}
			continue
		}

		// Queue the writes past the body budget again, then commit the rest.
		q.requeue(batch[carried:])
		preparation = q.commit(ctx, tx, batch[:carried])
	}
}

// run applies batch's writes to tx, under each write's own context, stopping
// at the body budget. It returns the index of a write that failed after
// changing the transaction, after finishing it, or -1 with the number of
// writes the transaction carried. A write that ends cleanly, see endsClean,
// is finished in place and carried as ended. An error refuses the batch.
func (q *Queue) run(ctx context.Context, tx Transaction, batch []*queuedWrite) (int, int, error) {
	started := time.Now()
	for i, w := range batch {
		before, err := tx.GetSeqno(ctx)
		if err != nil {
			return -1, 0, err
		}
		appliedBefore, bodyStarted := q.applied(w.ctx), time.Now()
		checked, err := w.run(q, tx)
		if elapsed := time.Since(bodyStarted); elapsed >= slowBodyWarn {
			logrus.WithFields(logrus.Fields{
				"op":      q.name(w.ctx),
				"ops":     q.applied(w.ctx) - appliedBefore,
				"actor":   w.actor,
				"index":   i,
				"writes":  len(batch),
				"elapsed": elapsed.Round(time.Millisecond).String(),
			}).Warn("slow World write body; the writer lock is held while it runs")
		}
		if errors.Is(err, coord.ErrStaleGeneration) {
			return -1, 0, err
		}
		if err != nil {
			// A write that left the transaction as it was ends alone, so the
			// writes before it do not run again under the writer lock.
			clean, cerr := q.endsClean(ctx, tx, before, checked, err)
			if cerr != nil {
				return -1, 0, cerr
			}
			w.finish(err)
			if !clean {
				return i, 0, nil
			}
		}
		if time.Since(started) >= q.bodyBudget {
			return -1, i + 1, nil
		}
	}
	return -1, len(batch), nil
}
