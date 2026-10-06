package block

import "context"

// PendingBatch is an immutable borrow of buffered entries. Complete must be
// called exactly once after the publisher is finished, even on admission failure.
// Entries remain readable through the BufferedStore until successful completion.
type PendingBatch struct {
	// Entries are the borrowed batch entries.
	Entries []*PutBatchEntry
	// complete resolves the borrow with the publication result.
	complete func(error)
	// completed records that Complete already ran; further calls are no-ops.
	completed bool
}

// Complete resolves the borrow exactly once with the publication result.
func (b *PendingBatch) Complete(err error) { b.complete(err) }

// TakePending borrows all currently queued entries without writing or forgetting
// them. Further writes can prepare the next publication while this borrow is in
// flight. Both queued and borrowed content count against the existing bounds.
func (s *BufferedStore) TakePending(ctx context.Context) (*PendingBatch, error) {
	// Serialize the publication borrow with buffered drains.
	release, err := s.drainMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// A publication completes without a context to pin with, so write pinned
	// roots through the drain first.
	var pinned bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pinned = len(s.pins) != 0
	})
	if pinned {
		if err := s.drainQueue(ctx); err != nil {
			return nil, err
		}
	}

	// Borrow all queued entries under the store state lock.
	var batch *drainBatch
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if err = ctx.Err(); err != nil {
			return
		}
		if err = s.drainErr; err != nil {
			return
		}
		batch = s.takeDrainBatchLocked(0)
	})
	if err != nil {
		return nil, err
	}

	// Expose borrowed entries with their publication completion callback.
	out := &PendingBatch{complete: func(error) {}}
	if batch != nil {
		out.Entries = batch.entries
		out.complete = func(err error) {
			s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
				if out.completed {
					return
				}
				out.completed = true
				s.completeBatchLocked(batch, err)
				broadcastFn()
			})
		}
	}
	return out, nil
}
