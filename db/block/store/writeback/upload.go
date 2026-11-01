package block_store_writeback

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/errgroup"
)

const (
	// uploadBatchBlocks bounds the blocks one upload batch carries to a remote
	// that sizes none.
	uploadBatchBlocks = 4096
	// uploadBatchBytes bounds the recorded block bytes one upload batch
	// carries to a remote that sizes none. A batch holds at least one block,
	// whatever its size.
	uploadBatchBytes = 4 << 20
	// uploadWorkers bounds the batches written at once to a remote that sizes
	// none. A remote with a long round trip, such as a database across the
	// internet, uploads one batch per round trip per worker.
	uploadWorkers = 16
)

// batchLimitsOf returns the batches Upload builds for remote: the remote's
// own, where it sizes them, over the defaults for a remote that does not.
func batchLimitsOf(remote block.StoreOps) block.BatchLimits {
	// Start from the defaults and stop there for a remote that sizes none.
	limits := block.BatchLimits{Bytes: uploadBatchBytes, Blocks: uploadBatchBlocks, Workers: uploadWorkers}
	sizer, ok := remote.(block.BatchSizer)
	if !ok {
		return limits
	}

	// Take each limit the remote sets.
	sized := sizer.GetBatchLimits()
	if sized.Bytes > 0 {
		limits.Bytes = sized.Bytes
	}
	if sized.Blocks > 0 {
		limits.Blocks = sized.Blocks
	}
	if sized.Workers > 0 {
		limits.Workers = sized.Workers
	}
	limits.Linger = max(sized.Linger, 0)
	return limits
}

// Upload drains the markers to remote, oldest first, until ctx is canceled.
// A marker uploads once it is older than the policy's settle age, or earlier
// while the queue exceeds its byte limit.
//
// The blocks upload in batches of the size the remote takes best (see
// block.BatchSizer), several at once. Where the remote keeps each batch as a
// unit, only a full batch uploads, until the oldest settled block has waited
// the remote's linger time: a block uploads at most its settle age plus that
// linger, plus the time the batches ahead of it take, after it was written.
// A batch clears its markers once the remote holds its blocks, so a failure
// repeats only the batches that did not finish.
//
// Returns the first upload failure, which stays in the status until the next
// successful batch. The caller retries with its own backoff.
func (s *Store) Upload(ctx context.Context, remote block.StoreOps) error {
	limits := batchLimitsOf(remote)
	for {
		round, err := s.nextRound(ctx, limits)
		if err != nil {
			return err
		}
		if err := s.uploadRound(ctx, remote, round, limits.Workers); err != nil {
			if ctx.Err() == nil {
				s.SetError(err)
			}
			return err
		}
	}
}

// waitDue waits until the oldest marker has settled or the queue exceeds its
// byte limit. A mark or policy change wakes it to recompute the deadline,
// which reads no markers.
func (s *Store) waitDue(ctx context.Context) error {
	for {
		// Compute the oldest marker's due time from the projected queue.
		now := time.Now()
		var due time.Time
		var pending, ready bool
		var changed <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			pending = s.status.Pending > 0
			due = s.status.Oldest.Add(s.status.Policy.Settle)
			ready = pending && (!now.Before(due) || s.status.Policy.overLimit(s.status.PendingBytes))
		})
		if ready {
			return nil
		}

		// Sleep until the due time, a mark, or a policy change.
		var timer *time.Timer
		var expired <-chan time.Time
		if pending {
			timer = time.NewTimer(due.Sub(now))
			expired = timer.C
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-expired:
		}
		if timer != nil {
			timer.Stop()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// pendingBlock is a queued block and its marker keys.
type pendingBlock struct {
	queueKey  []byte
	markedKey []byte
	// value is the queue entry's value when the batch read it. A write of the
	// block since then changes the entry's value.
	value []byte
	ref   *block.BlockRef
	size  int64
	// at is the block's write time.
	at time.Time
	// dropped is set when the local store no longer held the block, so the
	// batch did not upload it.
	dropped bool
}

// uploadRound is the batches to upload together: spans of entries, each the
// blocks of one batch.
type uploadRound struct {
	entries []pendingBlock
	spans   [][2]int
}

// queueHead is the start of the upload queue, read to choose a round.
type queueHead struct {
	// entries are the queued blocks in write order, grouped into batches by
	// ends.
	entries []pendingBlock
	// due is how many leading entries may upload: they settled, or the queue
	// exceeds its byte limit.
	due int
	// forced reports that an entry is due only because the queue exceeds its
	// byte limit.
	forced bool
	// ends holds the end index of each batch the next entry did not fit. The
	// entries after the last end start a batch that may still grow.
	ends []int
	// fullAt is when the first closed batch has settled whole, zero before one
	// closed.
	fullAt time.Time
	// policy, oldest and pendingBytes are the status the read was chosen
	// against.
	policy       Policy
	oldest       time.Time
	pendingBytes int64
}

// nextRound waits until a round of batches is worth uploading and returns it.
func (s *Store) nextRound(ctx context.Context, limits block.BatchLimits) (uploadRound, error) {
	for {
		// Wait for the oldest block to settle, then read the queue's head.
		if err := s.waitDue(ctx); err != nil {
			return uploadRound{}, err
		}
		head, err := s.scanHead(ctx, limits)
		if err != nil {
			return uploadRound{}, err
		}

		// Upload the round, or wait for the batch to fill or linger.
		round, readyAt := head.plan(time.Now(), limits.Linger)
		if len(round.spans) != 0 {
			return round, nil
		}
		if err := s.waitFill(ctx, head, limits, readyAt); err != nil {
			return uploadRound{}, err
		}
	}
}

// scanHead reads the queue from its oldest block, up to the batches one round
// uploads and one block past the last, which tells that the last is full. A
// round stays within limits.Workers batches, so a read holds at most that many
// batches of keys, never the queue.
func (s *Store) scanHead(ctx context.Context, limits block.BatchLimits) (*queueHead, error) {
	// Read the status the batches are chosen against.
	var policy Policy
	var oldest time.Time
	var pendingBytes int64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		policy, oldest, pendingBytes = s.status.Policy, s.status.Oldest, s.status.PendingBytes
	})
	settled := time.Now().Add(-policy.Settle)

	// Read the queue in write order into batches.
	var head *queueHead
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Start a head and track the open batch.
		head = &queueHead{policy: policy, oldest: oldest, pendingBytes: pendingBytes}
		var batchBlocks int
		var batchBytes, dueBytes int64
		ended := false
		err := scanQueue(ctx, tx, func(entry queueEntry) error {
			// Close the open batch when the entry does not fit it.
			if batchBlocks != 0 && (batchBlocks == limits.Blocks || batchBytes+entry.size > limits.Bytes) {
				head.ends = append(head.ends, len(head.entries))
				if len(head.ends) == 1 {
					head.fullAt = head.entries[len(head.entries)-1].at.Add(policy.Settle)
				}
				batchBlocks, batchBytes = 0, 0
				if len(head.ends) == limits.Workers {
					return errScanDone
				}
			}

			// End the due entries at the first that neither settled nor
			// overflowed the queue.
			if !ended {
				settledEntry := !entry.at.After(settled)
				if settledEntry || policy.overLimit(pendingBytes-dueBytes) {
					head.forced = head.forced || !settledEntry
					dueBytes += entry.size
				} else {
					ended = true
				}
			}

			// Stop once the due entries ended and a batch is closed: no later
			// batch can upload whole.
			if ended && len(head.ends) != 0 {
				return errScanDone
			}

			// Add the entry to the open batch.
			head.entries = append(head.entries, pendingBlock{
				queueKey:  entry.key,
				markedKey: markedKey(entry.hash),
				value:     entry.value,
				ref:       block.NewBlockRef(entry.hash),
				size:      entry.size,
				at:        entry.at,
			})
			batchBlocks++
			batchBytes += entry.size
			if !ended {
				head.due = len(head.entries)
			}
			return nil
		})
		if errors.Is(err, errScanDone) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan pending uploads")
	}
	return head, nil
}

// plan chooses the batches to upload at now: every closed batch whose entries
// are all due, then the due entries after them as one short batch once the
// first of them waited linger past its settle age, or at once while the queue
// exceeds its byte limit. With no batch chosen it returns when to look again.
func (h *queueHead) plan(now time.Time, linger time.Duration) (uploadRound, time.Time) {
	// Take the closed batches that are entirely due.
	round := uploadRound{entries: h.entries}
	start := 0
	for _, end := range h.ends {
		if end > h.due {
			break
		}
		round.spans = append(round.spans, [2]int{start, end})
		start = end
	}

	// Take the due entries after them once they waited long enough to fill.
	var readyAt time.Time
	switch {
	case start < h.due:
		readyAt = h.entries[start].at.Add(h.policy.Settle + linger)
		if h.forced || !now.Before(readyAt) {
			round.spans = append(round.spans, [2]int{start, h.due})
		}
	case len(h.entries) != 0:
		readyAt = h.entries[0].at.Add(h.policy.Settle)
	}

	// The first batch also fills when its last entry settles.
	if !h.fullAt.IsZero() && (readyAt.IsZero() || h.fullAt.Before(readyAt)) {
		readyAt = h.fullAt
	}
	return round, readyAt
}

// waitFill waits for readyAt, or until a change could make a read of the queue
// choose a batch: a policy change, a new queue head, or enough queued bytes to
// close the first batch. A mark that changes none of them wakes it and goes back
// to sleep without reading markers. A zero readyAt waits for a change alone.
func (s *Store) waitFill(ctx context.Context, head *queueHead, limits block.BatchLimits, readyAt time.Time) error {
	for {
		// Decide from the projected status whether to read the queue again.
		var changed <-chan struct{}
		var again bool
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			st := &s.status
			again = st.Pending == 0 ||
				st.Policy != head.policy ||
				!st.Oldest.Equal(head.oldest) ||
				!head.policy.overLimit(head.pendingBytes) && st.Policy.overLimit(st.PendingBytes) ||
				len(head.ends) == 0 && (st.PendingBytes >= limits.Bytes || st.Pending >= limits.Blocks)
		})
		if again {
			return nil
		}

		// Sleep until readyAt or the next status change.
		var timer *time.Timer
		var expired <-chan time.Time
		if !readyAt.IsZero() {
			timer = time.NewTimer(time.Until(readyAt))
			expired = timer.C
		}
		select {
		case <-ctx.Done():
		case <-changed:
		case <-expired:
			return nil
		}
		if timer != nil {
			timer.Stop()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// uploadRound writes the round's batches to remote, at most workers at once.
// Each batch clears its markers as soon as the remote holds its blocks, so a
// failed batch repeats alone: the remote keeps a repeated block as another
// copy, and a bucket of packfiles as another pack.
func (s *Store) uploadRound(ctx context.Context, remote block.StoreOps, round uploadRound, workers int) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(workers)
	for _, span := range round.spans {
		batch := round.entries[span[0]:span[1]]
		eg.Go(func() error {
			if err := s.uploadChunk(egCtx, remote, batch); err != nil {
				return err
			}
			// Clear under the caller's context, so a sibling's failure does
			// not leave an uploaded batch queued.
			return s.clear(ctx, batch)
		})
	}
	return eg.Wait()
}

// uploadChunk writes the batch's blocks with their refs to remote. A block the
// local store no longer holds is marked dropped without upload. A block it
// holds without its refs fails the batch, since the remote would keep it as a
// leaf.
func (s *Store) uploadChunk(ctx context.Context, remote block.StoreOps, batch []pendingBlock) error {
	// Read the blocks with their refs from the local store, as many at once
	// as the disk's load admits.
	read := make([]*block.PutBatchEntry, len(batch))
	eg, egCtx := errgroup.WithContext(ctx)
	for i := range batch {
		eg.Go(func() error {
			return s.reads.read(egCtx, func() error {
				entry, err := s.readPending(egCtx, &batch[i])
				read[i] = entry
				return err
			})
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Write the read blocks to the remote in one batch.
	entries := slices.DeleteFunc(read, func(entry *block.PutBatchEntry) bool { return entry == nil })
	if len(entries) == 0 {
		return nil
	}
	if _, err := remote.PutBlockBatch(ctx, entries); err != nil {
		return errors.Wrap(err, "upload blocks")
	}
	return nil
}

// readPending reads the pending block with its refs from the local store. It
// marks a block the store no longer holds dropped and returns nil, and fails
// on a block held without its refs, since the remote would keep it as a leaf.
func (s *Store) readPending(ctx context.Context, pending *pendingBlock) (*block.PutBatchEntry, error) {
	// Read the block, marking it dropped when gone.
	stored, err := s.local.GetStoredBlock(ctx, pending.ref)
	if err != nil {
		return nil, errors.Wrap(err, "read pending block")
	}
	if stored == nil {
		pending.dropped = true
		return nil, nil
	}

	// Upload only a block whose refs are known.
	if !stored.RefsKnown {
		return nil, errors.Wrapf(block.ErrRefsUnknown, "upload block %s", pending.ref.MarshalString())
	}
	return &block.PutBatchEntry{Ref: pending.ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true}, nil
}
