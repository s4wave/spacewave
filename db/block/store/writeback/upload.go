package block_store_writeback

import (
	"bytes"
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
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
	// clearLocatedBlocks bounds the hashes one ClearLocated round looks up
	// and clears; clear bounds each of its transactions itself.
	clearLocatedBlocks = 1024
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
	return s.UploadPaced(ctx, remote, nil)
}

// UploadAdmit is called before each upload round with the bytes the previous
// round uploaded, zero before the first, and returns once the round may
// start or with ctx's error. It paces the upload, which is background work,
// against the foreground load it competes with: the round's local reads, its
// marker clears and its remote writes. A nil UploadAdmit admits each round at
// once.
type UploadAdmit func(ctx context.Context, lastBytes int64) error

// UploadPaced is Upload with each round admitted by admit. Pacing delays the
// upload only: the blocks stay queued, and the local store keeps them, until
// a round uploads them.
func (s *Store) UploadPaced(ctx context.Context, remote block.StoreOps, admit UploadAdmit) error {
	// Upload rounds until one fails or ctx ends.
	limits := batchLimitsOf(remote)
	var head *queueHead
	var lastBytes int64
	for {
		var round uploadRound
		var err error
		round, head, err = s.nextRound(ctx, limits, head)
		if err != nil {
			return err
		}
		if admit != nil {
			if err := admit(ctx, lastBytes); err != nil {
				return err
			}
		}
		lastBytes = 0
		for _, entry := range round.entries {
			lastBytes += entry.size
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
// which reads no markers. It sleeps on the queue's head, Oldest, rather than
// its count, which may still be counting after an open.
func (s *Store) waitDue(ctx context.Context) error {
	for {
		// Compute the oldest marker's due time from the projected queue.
		now := time.Now()
		var due time.Time
		var pending, ready bool
		var changed <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			pending = !s.status.Oldest.IsZero()
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
	// against, and queued the store's count of queueing transactions then.
	policy       Policy
	oldest       time.Time
	pendingBytes int64
	queued       uint64
}

// nextRound waits until a round of batches is worth uploading and returns it,
// with the queue head it was chosen from. prev is the head the last call
// returned, or nil; each read of the queue reuses the entries the read before
// it parsed.
func (s *Store) nextRound(ctx context.Context, limits block.BatchLimits, prev *queueHead) (uploadRound, *queueHead, error) {
	for {
		// Wait for the oldest block to settle, then read the queue's head.
		if err := s.waitDue(ctx); err != nil {
			return uploadRound{}, prev, err
		}
		head, err := s.scanHead(ctx, limits, prev)
		if err != nil {
			return uploadRound{}, prev, err
		}
		prev = head

		// Upload the round, or wait for the batch to fill or linger.
		round, readyAt := head.plan(time.Now(), limits.Linger)
		if len(round.spans) != 0 {
			return round, head, nil
		}
		if err := s.waitFill(ctx, head, limits, readyAt); err != nil {
			return uploadRound{}, head, err
		}
	}
}

// scanHead reads the queue from its oldest block, up to the batches one round
// uploads and one block past the last, which tells that the last is full. A
// round stays within limits.Workers batches, so a read holds at most that many
// batches of keys, never the queue.
//
// prev is an earlier head of the same queue, or nil. An entry whose key and
// value match one prev read is taken from prev rather than parsed again: a
// queue key names one block written at one time, and a repeated write of the
// block changes the value. Both list keys in order, so one pass matches them.
func (s *Store) scanHead(ctx context.Context, limits block.BatchLimits, prev *queueHead) (*queueHead, error) {
	// Read the status the batches are chosen against.
	var policy Policy
	var oldest time.Time
	var pendingBytes int64
	var floor []byte
	var queued uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		policy, oldest, pendingBytes = s.status.Policy, s.status.Oldest, s.status.PendingBytes
		floor, queued = s.floor, s.queued
	})
	settled := time.Now().Add(-policy.Settle)

	// Read the queue in write order into batches.
	var head *queueHead
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Start a head and track the open batch.
		head = &queueHead{policy: policy, oldest: oldest, pendingBytes: pendingBytes, queued: queued}
		var known []pendingBlock
		if prev != nil {
			known = prev.entries
			head.entries = make([]pendingBlock, 0, len(known))
		}
		var batchBlocks int
		var batchBytes, dueBytes int64
		ended := false

		// A transaction that commits after the floor read lowered the floor
		// first, so the next round reads its blocks if this one misses them.
		err := scanQueue(ctx, tx, floor, func(item queueItem) error {
			// Take the entry from the earlier head, or parse it.
			for len(known) != 0 && bytes.Compare(known[0].queueKey, item.key) < 0 {
				known = known[1:]
			}
			var entry pendingBlock
			if len(known) != 0 && bytes.Equal(known[0].queueKey, item.key) && bytes.Equal(known[0].value, item.value) {
				entry, known = known[0], known[1:]
				entry.dropped = false
			} else {
				var err error
				if entry, err = item.pending(); err != nil {
					return err
				}
			}

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
				forcedEntry := policy.overLimit(pendingBytes - dueBytes)
				if settledEntry || forcedEntry {
					head.forced = head.forced || forcedEntry
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
			head.entries = append(head.entries, entry)
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
// choose a batch: a policy change, a new queue head, enough queued bytes to
// close the first batch, or any queueing after a read that found the queue
// empty, since Oldest can stay at a cleared head while a writer that lowered
// the floor aborts. A mark that changes none of them wakes it and goes back to
// sleep without reading markers. A zero readyAt waits for a change alone.
func (s *Store) waitFill(ctx context.Context, head *queueHead, limits block.BatchLimits, readyAt time.Time) error {
	for {
		// Decide from the projected status whether to read the queue again.
		var changed <-chan struct{}
		var again bool
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			st := &s.status
			again = st.Oldest.IsZero() ||
				len(head.entries) == 0 && s.queued != head.queued ||
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
			if err := s.clear(ctx, batch); err != nil {
				return err
			}
			s.SetError(nil)
			return nil
		})
	}
	return eg.Wait()
}

// BlockLocator is a remote that can say which blocks it holds from a local
// index, without reading the remote.
type BlockLocator interface {
	// LocateBlocks reports, for each ref, whether the remote holds it.
	LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
	// KeepsBlocks reports that the remote never reclaims a block it holds. A
	// remote that does relies on the next writer of a dead block uploading it
	// again, so a block it locates may not stay.
	KeepsBlocks() bool
}

// SetLocateUploaded makes Upload skip the blocks the remote already holds, such
// as those another writer uploaded, while on is set. Otherwise it reads and
// writes every queued block.
func (s *Store) SetLocateUploaded(on *atomic.Bool) {
	s.locate.Store(on)
}

// locatedBlocks reports which blocks of batch the remote holds, or nil to
// upload them all. A failed lookup uploads them: a repeated block is only
// stored twice, and a block wrongly skipped could lose its last copy.
func (s *Store) locatedBlocks(ctx context.Context, remote block.StoreOps, batch []pendingBlock) []bool {
	located, err := s.locateBlocks(ctx, remote, batch)
	if err != nil {
		return nil
	}
	return located
}

// locateBlocks reports which blocks of batch the remote's index names. It
// returns nil and no error when the switch is off or remote has no index that
// keeps its blocks, and an error when the lookup fails.
func (s *Store) locateBlocks(ctx context.Context, remote block.StoreOps, batch []pendingBlock) ([]bool, error) {
	// Only a remote with an index can answer without reading the remote.
	locator, ok := remote.(BlockLocator)
	on := s.locate.Load()
	if !ok || on == nil || !on.Load() || !locator.KeepsBlocks() {
		return nil, nil
	}

	// Ask about the whole batch at once.
	refs := make([]*block.BlockRef, len(batch))
	for i := range batch {
		refs[i] = batch[i].ref
	}
	located, err := locator.LocateBlocks(ctx, refs)
	if err != nil {
		return nil, errors.Wrap(err, "locate queued blocks")
	}
	if len(located) != len(batch) {
		return nil, errors.Errorf("located %d of %d queued blocks", len(located), len(batch))
	}
	return located, nil
}

// ClearLocated clears the queue entries of the blocks another writer just
// uploaded, wherever they stand in the queue, so that a backlog ahead of them
// does not keep them on the local store. It follows the same rule as the skip
// in uploadChunk: an entry goes only when the switch is on, remote keeps its
// blocks and its index names the block. Anything else stays queued for Upload,
// which is the only thing that must eventually drain the queue, so a block this
// misses costs only its turn. A block with no entry is left alone. A failed
// lookup, or ctx ending, returns an error, so a caller that answers for the
// clear can tell it did not finish; with the switch off it returns nil having
// cleared nothing.
//
// The queue's head stays the oldest block the remote may lack, so UploadedBefore
// keeps its meaning. Each block clears once, here or in Upload, so this adds no
// marker writes to the ones its upload would have made; it only moves them
// earlier. Clears wait behind foreground writers, so the caller must not hold up
// anything that matters on this call. A clear says nothing about the upload's
// health, so it leaves the status's failure alone.
func (s *Store) ClearLocated(ctx context.Context, remote block.StoreOps, hashes []*hash.Hash) error {
	for chunk := range slices.Chunk(hashes, clearLocatedBlocks) {
		// Find the blocks still queued.
		batch, err := s.queuedBlocks(ctx, chunk)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			continue
		}

		// Keep those the remote's index names.
		located, err := s.locateBlocks(ctx, remote, batch)
		if err != nil {
			return err
		}
		if located == nil {
			continue
		}
		held := batch[:0]
		for i := range batch {
			if located[i] {
				held = append(held, batch[i])
			}
		}
		if len(held) == 0 {
			continue
		}
		if err := s.clear(ctx, held); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// queuedBlocks returns the queue entries of the hashes that have one.
func (s *Store) queuedBlocks(ctx context.Context, hashes []*hash.Hash) ([]pendingBlock, error) {
	var batch []pendingBlock
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Read each block's marker, which names its queue entry.
		batch = batch[:0]
		keys := make([][]byte, len(hashes))
		for i, h := range hashes {
			keys[i] = markedKey(h)
		}
		marked, found, err := kvtx.GetBatch(ctx, tx, keys)
		if err != nil {
			return err
		}
		queued := make([][]byte, 0, len(marked))
		for i := range marked {
			if found[i] && len(marked[i]) != 0 {
				queued = append(queued, marked[i])
			}
		}
		if len(queued) == 0 {
			return nil
		}

		// Read the entries and decode them.
		values, found, err := kvtx.GetBatch(ctx, tx, queued)
		if err != nil {
			return err
		}
		for i := range queued {
			if !found[i] {
				continue
			}
			entry, err := queueItem{key: queued[i], value: values[i]}.pending()
			if err != nil {
				return err
			}
			batch = append(batch, entry)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read queued blocks")
	}
	return batch, nil
}

// uploadChunk writes the batch's blocks with their refs to remote. A block the
// remote already holds is neither read nor written, and clears as uploaded. A
// block the local store no longer holds is marked dropped without upload. A
// block it holds without its refs fails the batch, since the remote would keep
// it as a leaf.
func (s *Store) uploadChunk(ctx context.Context, remote block.StoreOps, batch []pendingBlock) error {
	// Leave out the blocks the remote holds.
	located := s.locatedBlocks(ctx, remote, batch)

	// Read the blocks with their refs from the local store, as many at once
	// as the disk's load admits.
	read := make([]*block.PutBatchEntry, len(batch))
	err := s.reads.readEach(ctx, len(batch), func(ctx context.Context, i int) error {
		if located != nil && located[i] {
			return nil
		}
		entry, err := s.readPending(ctx, &batch[i])
		read[i] = entry
		return err
	})
	if err != nil {
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
// marks a block the store no longer holds dropped and returns nil. A block
// held without its refs uploads with its refs unknown, which the remote
// keeps as unknown rather than as a leaf.
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
	return &block.PutBatchEntry{Ref: pending.ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown}, nil
}
