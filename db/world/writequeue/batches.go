package writequeue

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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
		// An observed base can be replaced at the same sequence number.
		if !c.dropped && (c.baseOnly || c.seqno == 0 || c.seqno > seqno) {
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
			w = &queuedWrite{ctx: context.WithoutCancel(w.ctx), name: w.name, queued: w.queued, actor: w.actor, checks: w.checks, body: w.replay, replay: w.replay, arrival: w.arrival, answered: true, refusals: w.refusals, done: make(chan struct{})}
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
// MaxBatch writes, or the first write's limit, passing over the writes of an
// actor whose earlier write is parked. It ends the writes whose writers left
// before the fork and leaves them out.
func (q *Queue) take() []*queuedWrite {
	// Cut the batch from the head.
	q.mtx.Lock()
	var batch, rest []*queuedWrite
	limit := MaxBatch
	for _, w := range q.pending {
		if len(batch) >= limit || q.parkedLocked(w) {
			rest = append(rest, w)
			continue
		}
		if len(batch) == 0 && w.limit != 0 {
			limit = min(limit, w.limit)
		}
		batch = append(batch, w)
	}
	q.pending = rest
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
// Every queued write waits while a batch runs, so its block reads are urgent
// (block.ReadUrgent) and go ahead of every other read on a bounded link.
func (q *Queue) drain() {
	// Return a prepared claim even when every waiting writer left.
	var preparation world.AdmissionPreparation
	defer func() {
		if preparation != nil {
			preparation.Release()
		}
	}()
	ctx := block.WithReadClass(q.ctx, block.ReadUrgent)
	for {
		// Stop when no write may run; a parked write's fetch queues it again
		// and starts a drain.
		q.mtx.Lock()
		if !q.runnableLocked() {
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

		// A dirty failure discards the fork. If it read a changed prefix, run
		// that prefix on its own before attempting the failed write again.
		base, err := tx.GetSeqno(ctx)
		if err != nil {
			tx.Discard()
			q.refused(batch, err)
			continue
		}
		failed, carried, parked, err := q.run(ctx, tx, batch, base)
		if parked >= 0 && err == nil {
			// Fetch the blocks a body deferred with the lock released.
			tx.Discard()
			w := batch[parked]
			q.park(w, slices.Delete(batch, parked, parked+1))
			continue
		}
		if err != nil || failed >= 0 {
			if err != nil {
				tx.Discard()
				q.refused(batch, err)
				continue
			}
			if carried != 0 {
				tx.Discard()
				batch[0].limit = carried
				q.requeue(batch)
				continue
			}
			settle := q.finishOnBase(wtx, tx, []*queuedWrite{batch[failed]}, base)
			tx.Discard()
			settle()
			q.requeue(slices.Delete(batch, failed, failed+1))
			continue
		}

		// Queue the writes past the body budget again, then commit the rest.
		q.requeue(batch[carried:])
		preparation = q.commit(ctx, wtx, tx, batch[:carried], base)
	}
}

// run applies batch's writes to tx, under each write's own context, stopping
// at the body budget. A dirty failure returns its index and the size of the
// prefix to retry separately, or zero if its error depends only on the base.
// Otherwise the failed index is -1 and carried counts the writes to commit.
// A deferred peer read returns its index as parked, whatever the body
// returned. A batch error refuses the whole attempt.
func (q *Queue) run(ctx context.Context, tx Transaction, batch []*queuedWrite, base uint64) (int, int, int, error) {
	// Time the writer turn and track which bodies read earlier changes.
	started := time.Now()
	before := base
	for i, w := range batch {
		// The prior attempt's outcome no longer belongs to this write.
		w.bodyErr = nil
		var err error
		if i != 0 {
			before, err = tx.GetSeqno(ctx)
			if err != nil {
				return -1, 0, -1, err
			}
		}

		// Run and time the checks and body with the batch holding the writer.
		var spans bodySpans
		appliedBefore, bodyStarted := q.applied(w.ctx), time.Now()
		checked, err := w.run(q, tx, &spans)
		w.bodyStarted, w.bodyEnded = bodyStarted, time.Now()
		if elapsed := w.bodyEnded.Sub(bodyStarted); elapsed >= slowBodyWarn {
			logrus.WithFields(spans.fields(elapsed)).WithFields(logrus.Fields{
				"op":      q.name(w.ctx),
				"ops":     q.applied(w.ctx) - appliedBefore,
				"actor":   w.actor,
				"index":   i,
				"writes":  len(batch),
				"elapsed": elapsed.Round(time.Millisecond).String(),
			}).Warn("slow World write body; the writer lock is held while it runs")
		}

		// A stale fork or a deferred read invalidates the body's result.
		if errors.Is(err, coord.ErrStaleGeneration) {
			return -1, 0, -1, err
		}
		if w.deferring && w.peerReads.Pending() {
			return -1, 0, i, nil
		}

		// A write's body error may leave no changes, but its result belongs
		// to the World it read. Deliver it only once that World is durable.
		if err != nil {
			clean, cerr := q.endsClean(ctx, tx, before, checked, err)
			if cerr != nil {
				return -1, 0, -1, cerr
			}
			w.bodyErr = err
			if !clean {
				if before != base {
					return i, i, -1, nil
				}
				return i, 0, -1, nil
			}
		}

		// Leave later bodies for the next batch when this batch used its turn.
		if time.Since(started) >= q.bodyBudget {
			return -1, i + 1, -1, nil
		}
	}
	return -1, len(batch), -1, nil
}
