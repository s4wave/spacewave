package writequeue

import (
	"time"

	"github.com/sirupsen/logrus"
)

// OldestWait returns how long the oldest write still waiting for its result
// has waited at now, or zero when none waits.
func (q *Queue) OldestWait(now time.Time) time.Duration {
	// Take the oldest committing batch a writer waits on, else the oldest
	// pending write.
	q.mtx.Lock()
	var oldest time.Time
	for _, c := range q.committing {
		if c.waiting && (oldest.IsZero() || c.oldest.Before(oldest)) {
			oldest = c.oldest
		}
	}
	if len(q.pending) != 0 && (oldest.IsZero() || q.pending[0].queued.Before(oldest)) {
		oldest = q.pending[0].queued
	}
	for _, w := range q.parked {
		if oldest.IsZero() || w.queued.Before(oldest) {
			oldest = w.queued
		}
	}
	q.mtx.Unlock()
	if oldest.IsZero() {
		return 0
	}
	return now.Sub(oldest)
}

// watch runs commit, the commit of a batch of writes, warning while it runs
// past slowBatchWarn and once more when such a commit ends.
func (q *Queue) watch(writes int, commit func() error) error {
	// Warn at each interval the batch is still committing.
	started := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(slowBatchWarn)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status := q.Status()
				pending, committing := status.Pending, status.Committing
				logrus.WithFields(logrus.Fields{
					"writes":     writes,
					"pending":    pending,
					"committing": committing,
					"elapsed":    time.Since(started).Round(time.Second).String(),
				}).Warn("World write batch still being admitted; every batch behind it waits")
			}
		}
	}()

	// Commit and report a slow batch's end.
	err := commit()
	close(done)
	if elapsed := time.Since(started); elapsed >= slowBatchWarn {
		logrus.WithFields(logrus.Fields{
			"writes":  writes,
			"elapsed": elapsed.Round(time.Millisecond).String(),
		}).Warn("slow World write batch committed")
	}
	return err
}

// recordOwners records the actors of batch's writes, and the actors whose
// access they change, at seqno, the durable revision that carries them, once
// readers see it.
func (q *Queue) recordOwners(batch []*queuedWrite, seqno uint64) {
	// Collect the actors of the writes the commit carried: those answered
	// at admission or still waiting.
	if q.owners == nil {
		return
	}
	var actors []uint64
	for _, w := range batch {
		if !w.acked && w.ended() {
			continue
		}
		if w.actor != 0 {
			actors = append(actors, w.actor)
		}
		actors = append(actors, q.subjects(w.ctx)...)
	}
	if len(actors) != 0 {
		q.owners.Record(actors, seqno)
	}
}
