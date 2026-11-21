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
			w = &queuedWrite{
				ctx: context.WithoutCancel(w.ctx), name: w.name, queued: w.queued, actor: w.actor, checks: w.checks,
				body: w.replay, replay: w.replay, arrival: w.arrival, answered: true, refusals: w.refusals,
				done: make(chan struct{}),
			}
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

		// Run the bodies. A failed write leaves the rest to run again in a
		// fresh fork, since its partial changes are in the transaction.
		ran, err := q.run(ctx, tx, batch)
		if ran.parked >= 0 && err == nil {
			// Fetch the blocks a body deferred with the lock released.
			tx.Discard()
			w := batch[ran.parked]
			q.park(w, slices.Delete(batch, ran.parked, ran.parked+1))
			continue
		}
		if err != nil {
			tx.Discard()
			q.refused(batch, err)
			continue
		}
		if ran.failed >= 0 {
			// Drop the failed write's partial changes. A failure that read
			// the changes before it runs again once they commit: the next
			// batch carries only them.
			tx.Discard()
			if !batch[ran.failed].ended() {
				limitPrefix(batch[:ran.failed])
			}
			q.requeue(batch)
			continue
		}

		// Queue the withheld writes and those past the body budget again,
		// then commit the rest.
		q.requeue(append(ran.withheld, batch[ran.carried:]...))
		committed := batch[:ran.carried]
		if len(ran.withheld) != 0 {
			withheld := make(map[*queuedWrite]bool, len(ran.withheld))
			for _, w := range ran.withheld {
				withheld[w] = true
			}
			committed = slices.DeleteFunc(committed, func(w *queuedWrite) bool { return withheld[w] })
		}
		preparation = q.commit(ctx, tx, committed)
	}
}

// limitPrefix has the next batch carry only prefix's waiting writes, the
// writes before a failure that read their changes, so they commit before the
// failed write runs again.
func limitPrefix(prefix []*queuedWrite) {
	prefix = slices.DeleteFunc(slices.Clone(prefix), (*queuedWrite).ended)
	if len(prefix) != 0 && (prefix[0].limit == 0 || len(prefix) < prefix[0].limit) {
		prefix[0].limit = len(prefix)
	}
}

// batchRun is how run left a batch.
type batchRun struct {
	// carried counts the writes from the batch's head the transaction carries.
	carried int
	// failed is the index of a write that failed after changing the
	// transaction, or -1.
	failed int
	// parked is the index of a write whose body deferred a peer read, or -1.
	parked int
	// withheld are the carried writes that failed cleanly after reading
	// changes of earlier writes in the batch; they run again in a later
	// batch.
	withheld []*queuedWrite
}

// run applies batch's writes to tx, under each write's own context, stopping
// at the body budget.
//
// A write's error is an outcome only of the World its body read. A write
// that read only the fork's base, with no change of an earlier write in the
// batch, has the outcome of the World the batch forked, and finishes in
// place. A write that read such changes may have read a World that never
// commits, since the batch may be discarded or refused, so its error is
// withheld and the write runs again after those changes commit: a clean
// failure, see endsClean, joins withheld, and a write that failed after
// changing the transaction returns as failed, unfinished. A write that
// failed after changing a transaction holding only its own changes returns
// as failed, finished.
//
// A write whose body deferred a peer read returns its index as parked,
// unfinished, whatever its body returned, since a body may have taken the
// failed read for an absent block. An error refuses the batch.
func (q *Queue) run(ctx context.Context, tx Transaction, batch []*queuedWrite) (batchRun, error) {
	// Note the fork's base, which a write reads alone until a body changes it.
	ran := batchRun{carried: len(batch), failed: -1, parked: -1}
	started := time.Now()
	base, err := tx.GetSeqno(ctx)
	if err != nil {
		return ran, err
	}
	for i, w := range batch {
		before, err := tx.GetSeqno(ctx)
		if err != nil {
			return ran, err
		}
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
		if errors.Is(err, coord.ErrStaleGeneration) {
			return ran, err
		}
		if w.deferring && w.peerReads.Pending() {
			ran.parked = i
			return ran, nil
		}
		if err != nil {
			// A write that left the transaction as it was ends alone, so the
			// writes before it do not run again under the writer lock.
			clean, cerr := q.endsClean(ctx, tx, before, checked, err)
			if cerr != nil {
				return ran, cerr
			}
			switch {
			case before == base:
				w.finish(err)
			case clean:
				ran.withheld = append(ran.withheld, w)
			}
			if !clean {
				ran.failed = i
				return ran, nil
			}
		}
		if time.Since(started) >= q.bodyBudget {
			ran.carried = i + 1
			return ran, nil
		}
	}
	return ran, nil
}
