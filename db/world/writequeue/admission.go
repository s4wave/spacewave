package writequeue

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// commit has the engine admit tx, which carries batch, answers the writes
// that succeed at admission, and waits for the durable result on its own
// goroutine, so drain forks the next batch at once.
func (q *Queue) commit(ctx context.Context, wtx world.Tx, tx Transaction, batch []*queuedWrite, base uint64) world.AdmissionPreparation {
	// An unchanged transaction has no publication of its own. Its body
	// errors follow the base receipt, if the base is still speculative.
	var settleBase func()
	if slices.ContainsFunc(batch, func(w *queuedWrite) bool { return w.bodyErr != nil }) {
		after, err := tx.GetSeqno(ctx)
		if err != nil {
			tx.Discard()
			q.refused(batch, err)
			return nil
		}
		if after == base {
			readers := make([]*queuedWrite, 0, len(batch))
			batch = slices.DeleteFunc(batch, func(w *queuedWrite) bool {
				if w.bodyErr == nil {
					return false
				}
				readers = append(readers, w)
				return true
			})
			settleBase = q.finishOnBase(wtx, tx, readers, base)
		}
	}

	// A batch that only read its base has nothing to commit.
	if len(batch) == 0 {
		tx.Discard()
		settleBase()
		return nil
	}

	// Count the batch as committing from its oldest write.
	c := &committingBatch{writes: batch, oldest: batch[0].queued, waiting: true}
	for _, w := range batch[1:] {
		if w.queued.Before(c.oldest) {
			c.oldest = w.queued
		}
	}
	q.mtx.Lock()
	q.committing = append(q.committing, c)
	q.mtx.Unlock()

	// Admit the batch. A batch the engine dropped meanwhile already queued
	// again.
	var adm world.Admission
	c.admitStarted = time.Now()
	admitCtx := withCommitParts(ctx, &c.parts)
	err := q.watch(len(batch), func() (err error) {
		adm, err = tx.Admit(admitCtx)
		return err
	})
	c.admitted = time.Now()

	// Release the fork before any wait for its inherited base.
	tx.Discard()
	if settleBase != nil {
		settleBase()
	}

	// Refused batches either prepare outside the writer or return to the queue.
	q.mtx.Lock()
	if err != nil {
		var retry world.AdmissionRetry
		if errors.As(err, &retry) {
			q.mtx.Unlock()
			return q.prepareAdmissionRetry(ctx, c, retry)
		}
		q.forgetLocked(c)
		dropped := c.dropped
		q.mtx.Unlock()
		if !dropped {
			q.refused(batch, err)
		}
		return nil
	}

	// Answer the writes that succeed at admission, unless the engine
	// already dropped the batch.
	c.seqno = adm.Seqno()
	if !c.dropped {
		c.waiting = false
		for _, w := range batch {
			switch {
			case w.ended():
			case w.replay == nil || w.bodyErr != nil:
				c.waiting = true
			default:
				w.acked = true
				w.seqno = c.seqno
				w.finish(nil)
			}
		}
	}
	q.mtx.Unlock()
	go q.settle(c, adm, tx.Durable)
	return nil
}

// settle waits for c's durable result and ends its waiting writes with it.
// A batch the engine dropped queued again in its reinstall hook, or here
// when the engine reported no reinstall.
func (q *Queue) settle(c *committingBatch, adm world.Admission, durable func(uint64)) {
	// Wait without claiming the writer or the queue mutex.
	seqno, err := adm.WaitDurable(withCommitParts(q.ctx, &c.parts))
	adm.Release()

	// A discarded publication requeues the writers that depend on it.
	q.mtx.Lock()
	q.forgetLocked(c)
	if c.dropped {
		q.mtx.Unlock()
		return
	}
	if err != nil {
		logrus.WithError(err).WithField("seqno", c.seqno).Warn("World publication failed with no reinstall reported; its writes queue again")
		q.dropLocked(c, err)
		q.mtx.Unlock()
		return
	}
	q.mtx.Unlock()

	// Publish auxiliary reads and refresh the views of the actors the batch changed.
	durable(seqno)
	if !c.baseOnly {
		q.observeStages(c, time.Now())
		if q.options.Committed != nil {
			q.options.Committed()
		}
		q.recordOwners(c.writes, seqno)
	}

	// Deliver the outcomes of this durable attempt.
	for _, w := range c.writes {
		if !w.ended() {
			if w.bodyErr == nil {
				w.seqno = seqno
			}
			w.finish(w.bodyErr)
		}
	}
}

// finishOnBase resolves a set of errors from a fork that carried no changes
// they read. Capture the exact base under the writer lock and call the
// returned function after releasing the writer.
func (q *Queue) finishOnBase(wtx world.Tx, tx Transaction, writes []*queuedWrite, base uint64) func() {
	// A fork of the durable World can answer without a commit.
	adm := world.BaseAdmission(wtx, base)
	if adm == nil {
		for _, w := range writes {
			w.finish(w.bodyErr)
		}
		return func() {}
	}

	// Register speculative errors before the fork releases the writer. A
	// reinstall requeues them through the same path as discarded commits.
	c := &committingBatch{writes: writes, baseOnly: true, oldest: writes[0].queued, seqno: base, waiting: true}
	for _, w := range writes[1:] {
		if w.queued.Before(c.oldest) {
			c.oldest = w.queued
		}
	}
	q.mtx.Lock()
	q.committing = append(q.committing, c)
	q.mtx.Unlock()
	return func() { go q.settle(c, adm, tx.Durable) }
}

// forgetLocked stops counting c as committing, with mtx held.
func (q *Queue) forgetLocked(c *committingBatch) {
	if i := slices.Index(q.committing, c); i >= 0 {
		q.committing = slices.Delete(q.committing, i, i+1)
	}
}

// Ahead reports whether the queue has admitted writes that are not durable
// yet, or is admitting a batch, so the durable World may lag the one the
// next write forks from.
func (q *Queue) Ahead() bool {
	q.mtx.Lock()
	defer q.mtx.Unlock()
	return len(q.committing) != 0
}

// refused ends or requeues the writes of a batch the engine refused with err.
// A batch over the engine's size limit queues again in halves, so only a
// write too large alone fails. A stale base, or a failed publication the
// batch was admitted behind, queues the batch again for a fresh fork: the
// engine fails every publication admitted after a failed one, since each
// derives from the World before it, and reinstalls the World without them, so
// their writes have not happened and may run again. The failed publication's
// own writes fail with its error. Any other error ends every write in the
// batch.
//
// A requeued batch runs after the batches the engine admitted behind it. No
// writer in it has a result yet, so none can have seen it run first.
func (q *Queue) refused(batch []*queuedWrite, err error) {
	switch {
	case q.oversized(err) && len(batch) > 1:
		for _, w := range batch {
			w.limit = len(batch) / 2
		}
	case errors.Is(err, coord.ErrStaleGeneration) || errors.Is(err, block.ErrPublicationDependency):
		for _, w := range batch {
			w.refusals++
			if w.refusals >= batchAttemptLimit && !w.ended() {
				w.fail(errors.Wrap(err, "World write attempt limit exhausted"))
			}
		}
	default:
		for _, w := range batch {
			if !w.ended() {
				w.fail(err)
			}
		}
		return
	}
	q.requeue(batch)
}

// prepareAdmissionRetry waits after Discard and outside the queue mutex. Keep
// c registered so latency accounting and a concurrent reinstall still see it.
func (q *Queue) prepareAdmissionRetry(ctx context.Context, c *committingBatch, retry world.AdmissionRetry) world.AdmissionPreparation {
	// A reinstall may already have requeued the batch.
	q.mtx.Lock()
	dropped := c.dropped
	q.mtx.Unlock()
	var preparation world.AdmissionPreparation
	var err error
	if !dropped {
		preparation, err = retry.Prepare(ctx)
	}

	// Stop counting this attempt before its writes return to the pending queue.
	q.mtx.Lock()
	q.forgetLocked(c)
	dropped = c.dropped
	q.mtx.Unlock()
	if dropped {
		if preparation != nil {
			preparation.Release()
		}
		return nil
	}
	if err != nil {
		q.refused(c.writes, err)
		return nil
	}
	q.requeue(c.writes)
	return preparation
}
