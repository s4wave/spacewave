// Package block_store_writeback keeps every block write in a local store and
// uploads it to a remote store in the background.
//
// A write is acknowledged once the local store holds the block and a durable
// marker names it. Upload drains the markers to the remote store and clears
// each marker only after the remote store holds its block, so writes made
// while the remote is unreachable upload after it returns, across restarts.
// Backfill queues the blocks stored before a remote was chosen.
//
// A marker waits a settle age before it uploads, so a block the garbage
// collector sweeps meanwhile never reaches the remote: the upload finds the
// block gone and drops its marker.
package block_store_writeback

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// backfillKey names the remote whose backfill completed.
const backfillKey = "backfill"

// formerPrefixes match the keys of the marker layout before the queue,
// dropped at open so the next backfill queues their blocks again.
var formerPrefixes = []string{"pending/", "target"}

// uploadBatchBlocks bounds the blocks one upload batch carries.
const uploadBatchBlocks = 4096

// uploadBatchBytes bounds the recorded block bytes one upload batch carries. A
// batch holds at least one block, whatever its size.
const uploadBatchBytes = 4 << 20

// backfillBatchSize bounds the blocks one backfill marker transaction queues.
const backfillBatchSize = 256

// errScanDone stops a queue scan.
var errScanDone = errors.New("queue scan done")

// Store is a local block store whose writes queue for upload.
type Store struct {
	*MarkingStore

	// local holds every block and serves the uploads.
	local block.StoreOps
	// values holds the local block values below any ownership tracking, so
	// eviction and refill change values alone.
	values block.StoreOps
	// markers holds one marker per block awaiting upload.
	markers kvtx.Store
	// mtx orders marker mutations with their projection into status.
	mtx csync.Mutex
	// evictMtx runs one eviction pass at a time, and disabling uploads waits
	// for the pass to end.
	evictMtx csync.Mutex
	// bcast guards status, and wakes Upload and status watchers.
	bcast broadcast.Broadcast
	// status projects the committed markers and the upload policy.
	status Status
}

// GetStoreParts returns the local store that holds every block.
func (s *Store) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "local", Store: s.local}}
}

// NewStore builds a Store over local, counting the markers left by a previous
// run. Queued blocks upload at once until SetPolicy sets a settle age. values
// holds the same blocks as local without its ownership tracking.
//
// When enabled is false, writes are not queued until SetEnabled enables them.
func NewStore(
	ctx context.Context,
	local, values block.StoreOps,
	markers kvtx.Store,
	enabled bool,
) (*Store, error) {
	// Connect local block writes to the durable upload markers.
	s := &Store{local: local, values: values, markers: markers}
	s.MarkingStore = NewMarkingStore(local, s.mark)

	// Restore the pending upload counts, eviction queue size and completed
	// backfill target.
	var pending int
	var pendingBytes, evictableBytes int64
	var target []byte
	var oldest time.Time
	err := kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Drop the former marker layout before reading the queue.
		for _, prefix := range formerPrefixes {
			if err := deletePrefix(ctx, tx, prefix); err != nil {
				return err
			}
		}

		// Read the completed backfill target and the eviction queue size, then
		// count the upload queue.
		pending, pendingBytes, oldest = 0, 0, time.Time{}
		var err error
		target, _, err = tx.Get(ctx, []byte(backfillKey))
		if err != nil {
			return err
		}
		evictableBytes, err = readEvictableBytes(ctx, tx)
		if err != nil {
			return err
		}
		return scanQueue(ctx, tx, func(entry queueEntry) error {
			if pending == 0 {
				oldest = entry.at
			}
			pending++
			pendingBytes += entry.size
			return nil
		})
	})
	if err != nil {
		return nil, errors.Wrap(err, "count pending uploads")
	}

	// Initialize upload status from the committed marker records.
	s.status = Status{
		Enabled:        enabled,
		Pending:        pending,
		PendingBytes:   pendingBytes,
		Oldest:         oldest,
		Target:         string(target),
		EvictableBytes: evictableBytes,
	}
	return s, nil
}

// SetPolicy sets when queued blocks upload and the resident budget, waking
// Upload and budget watchers to apply it.
func (s *Store) SetPolicy(policy Policy) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.status.Policy != policy {
			s.status.Policy = policy
			broadcast()
		}
	})
}

// GetStatus returns the upload status and a channel closed when it changes.
func (s *Store) GetStatus() (Status, <-chan struct{}) {
	var status Status
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		status, changed = s.status, getWaitCh()
	})
	return status, changed
}

// SetEnabled starts or stops queueing writes for upload.
//
// Disabling drops the queued markers, the eviction queue and the backfill
// target: the remote store no longer needs the markers, and the local store
// must keep the blocks it holds.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	// Serialize upload enablement with marker mutations.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Drop pending markers and the backfill target when uploads are disabled,
	// after any eviction pass, whose commits would count entries dropped here.
	if !enabled {
		releaseEvict, err := s.evictMtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer releaseEvict()
		err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			for _, prefix := range []string{markedPrefix, queuePrefix, evictPrefix} {
				if err := deletePrefix(ctx, tx, prefix); err != nil {
					return err
				}
			}
			if err := tx.Delete(ctx, []byte(evictableBytesKey)); err != nil {
				return err
			}
			return tx.Delete(ctx, []byte(backfillKey))
		})
		if err != nil {
			return errors.Wrap(err, "drop pending uploads")
		}
	}

	// Publish upload enablement and the resulting queue status.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Enabled = enabled
		if !enabled {
			s.status.Pending, s.status.PendingBytes = 0, 0
			s.status.EvictableBytes = 0
			s.status.Oldest = time.Time{}
			s.status.Err, s.status.Target = nil, ""
		}
		broadcast()
	})
	return nil
}

// Backfill queues every block list names for upload to target, unless a
// backfill for target already completed. Call it after enabling uploads for
// target, so the backfill and new writes together cover every block.
//
// A block the local store does not hold is skipped. The completed target is
// recorded durably, so an interrupted backfill runs again on the next call.
func (s *Store) Backfill(
	ctx context.Context,
	target string,
	list func(ctx context.Context) ([]*block.BlockRef, error),
) error {
	// Skip backfill when the selected remote target is already complete.
	status, _ := s.GetStatus()
	if status.Target == target {
		return nil
	}

	// Queue existing local blocks for the selected upload target.
	refs, err := list(ctx)
	if err != nil {
		return errors.Wrap(err, "list blocks to upload")
	}
	for batch := range slices.Chunk(refs, backfillBatchSize) {
		marks := make([]Mark, 0, len(batch))
		for _, ref := range batch {
			stat, err := s.local.StatBlock(ctx, ref)
			if err != nil {
				return errors.Wrap(err, "stat block to upload")
			}
			if stat != nil {
				marks = append(marks, Mark{Hash: ref.GetHash(), Size: max(stat.Size, 0)})
			}
		}
		if err := s.mark(ctx, marks); err != nil {
			return err
		}
	}

	// Record and publish the completed upload backfill target.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(backfillKey), []byte(target))
	})
	if err != nil {
		return errors.Wrap(err, "record upload backfill")
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Target = target
		broadcast()
	})
	return nil
}

// Upload drains the markers to remote, oldest first, until ctx is canceled.
// A marker uploads once it is older than the policy's settle age, or earlier
// while the queue exceeds its byte limit.
//
// Returns the first upload failure, which stays in the status until the next
// successful batch. The caller retries with its own backoff.
func (s *Store) Upload(ctx context.Context, remote block.StoreOps) error {
	for {
		if err := s.waitDue(ctx); err != nil {
			return err
		}
		batch, err := s.scanBatch(ctx)
		if err != nil {
			return err
		}
		if err := s.uploadBatch(ctx, remote, batch); err != nil {
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
			pending = s.status.Pending != 0
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
	ref       *block.BlockRef
	size      int64
	// dropped is set when the local store no longer held the block, so the
	// batch did not upload it.
	dropped bool
}

// scanBatch reads the due markers, oldest first, up to uploadBatchBlocks
// blocks and uploadBatchBytes bytes. A marker is due once settled, or while
// the bytes still queued after the batch would exceed the policy's limit.
func (s *Store) scanBatch(ctx context.Context) ([]pendingBlock, error) {
	// Read the policy and queue size the batch is chosen against.
	var policy Policy
	var pendingBytes int64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		policy, pendingBytes = s.status.Policy, s.status.PendingBytes
	})
	settled := time.Now().Add(-policy.Settle)

	// Collect the batch from the queue head.
	var batch []pendingBlock
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Scan the queue in write order within the upload batch limits.
		batch = batch[:0]
		var batchBytes int64
		err := scanQueue(ctx, tx, func(entry queueEntry) error {
			// Stop at the first marker that has neither settled nor overflowed.
			if entry.at.After(settled) && !policy.overLimit(pendingBytes-batchBytes) {
				return errScanDone
			}

			// Require the pending block to fit within the upload byte limit.
			if len(batch) != 0 && batchBytes+entry.size > uploadBatchBytes {
				return errScanDone
			}

			// Append the pending block and enforce the batch block limit.
			batch = append(batch, pendingBlock{
				queueKey:  entry.key,
				markedKey: markedKey(entry.hash),
				ref:       block.NewBlockRef(entry.hash),
				size:      entry.size,
			})
			batchBytes += entry.size
			if len(batch) == uploadBatchBlocks {
				return errScanDone
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
	return batch, nil
}

// uploadBatch writes the batch's blocks with their refs to remote, then clears
// their markers. A block the local store no longer holds is dropped without
// upload. A block it holds without its refs fails the batch, since the remote
// would keep it as a leaf.
func (s *Store) uploadBatch(ctx context.Context, remote block.StoreOps, batch []pendingBlock) error {
	entries := make([]*block.PutBatchEntry, 0, len(batch))
	for i, pending := range batch {
		stored, err := s.local.GetStoredBlock(ctx, pending.ref)
		if err != nil {
			return errors.Wrap(err, "read pending block")
		}
		if stored == nil {
			batch[i].dropped = true
			continue
		}
		if !stored.RefsKnown {
			return errors.Wrapf(block.ErrRefsUnknown, "upload block %s", pending.ref.MarshalString())
		}
		entries = append(entries, &block.PutBatchEntry{Ref: pending.ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true})
	}
	if len(entries) != 0 {
		if _, err := remote.PutBlockBatch(ctx, entries); err != nil {
			return errors.Wrap(err, "upload blocks")
		}
	}
	return s.clear(ctx, batch)
}

// mark queues written blocks for upload in one transaction.
//
// A disabled store skips the lock. The check repeats under the lock so a mark
// racing SetEnabled(false) cannot queue after the queue is dropped.
func (s *Store) mark(ctx context.Context, marks []Mark) error {
	// Skip upload markers while queueing is disabled.
	if !s.isEnabled() {
		return nil
	}

	// Serialize marker writes and recheck upload enablement.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !s.isEnabled() {
		return nil
	}

	// A repeated write finds its marker and keeps its first write time.
	var added int
	var addedBytes int64
	var oldest time.Time
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		added, addedBytes, oldest = 0, 0, time.Time{}
		for _, mark := range marks {
			key := markedKey(mark.Hash)
			found, err := tx.Exists(ctx, key)
			if err != nil {
				return err
			}
			if found {
				continue
			}
			at := queueTime(mark.At)
			if err := tx.Set(ctx, key, nil); err != nil {
				return err
			}
			if err := tx.Set(ctx, queueKey(at, mark.Hash), []byte(strconv.FormatInt(mark.Size, 10))); err != nil {
				return err
			}
			if added == 0 || at.Before(oldest) {
				oldest = at
			}
			added++
			addedBytes += mark.Size
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "queue block upload")
	}
	if added != 0 {
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if s.status.Pending == 0 || oldest.Before(s.status.Oldest) {
				s.status.Oldest = oldest
			}
			s.status.Pending += added
			s.status.PendingBytes += addedBytes
			broadcast()
		})
	}
	return nil
}

// clear removes the markers of uploaded blocks, queues them for eviction at
// the current epoch, and clears the last failure.
//
// A dropped block written again since the batch read it keeps its marker: the
// write found the marker and queued nothing, so clearing it would leave the
// block on the local store alone.
func (s *Store) clear(ctx context.Context, batch []pendingBlock) error {
	// Serialize uploaded marker removal with queue status updates.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Keep the markers of dropped blocks the local store holds again. A write
	// stores its block before it takes the lock to mark it, so a block absent
	// here is marked again after the clear.
	for i := range batch {
		if !batch[i].dropped {
			continue
		}
		stat, err := s.local.StatBlock(ctx, batch[i].ref)
		if err != nil {
			return errors.Wrap(err, "stat dropped block")
		}
		if stat != nil {
			batch[i].queueKey = nil
		}
	}

	// Remove uploaded block markers, count their cleared bytes, and read the
	// write time of the queue's new head.
	var cleared int
	var clearedBytes, queuedBytes int64
	var oldest time.Time
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Delete the markers still queued, counting what they held, and queue
		// the uploaded blocks for eviction.
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		cleared, clearedBytes, queuedBytes = 0, 0, 0
		for _, pending := range batch {
			if pending.queueKey == nil {
				continue
			}
			found, err := tx.Exists(ctx, pending.queueKey)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			if err := tx.Delete(ctx, pending.queueKey); err != nil {
				return err
			}
			if err := tx.Delete(ctx, pending.markedKey); err != nil {
				return err
			}
			if !pending.dropped {
				added, err := queueEvict(ctx, tx, epoch, pending.ref.GetHash(), pending.size)
				if err != nil {
					return err
				}
				queuedBytes += added
			}
			cleared++
			clearedBytes += pending.size
		}
		if err := addEvictableBytes(ctx, tx, queuedBytes); err != nil {
			return err
		}

		// Read the new queue head's write time.
		oldest = time.Time{}
		err = scanQueue(ctx, tx, func(entry queueEntry) error {
			oldest = entry.at
			return errScanDone
		})
		if errors.Is(err, errScanDone) {
			return nil
		}
		return err
	})
	if err != nil {
		return errors.Wrap(err, "clear uploaded blocks")
	}

	// Publish the remaining upload queue and clear its last failure.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Apply the cleared counts and the new head, then wake watchers.
		s.status.Pending -= cleared
		s.status.PendingBytes -= clearedBytes
		s.status.EvictableBytes += queuedBytes
		s.status.Err = nil
		s.status.Oldest = oldest
		broadcast()
	})
	return nil
}

// SetError records an upload failure in the status, such as a remote store
// that cannot be opened. The next successful batch clears it.
func (s *Store) SetError(err error) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Err = err
		broadcast()
	})
}

// isEnabled reports whether writes are queued for upload.
func (s *Store) isEnabled() bool {
	var enabled bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		enabled = s.status.Enabled
	})
	return enabled
}

// readTx opens a read transaction on the marker store.
func (s *Store) readTx(ctx context.Context) (kvtx.Tx, error) {
	return s.markers.NewTransaction(ctx, false)
}

// writeTx opens a write transaction on the marker store.
func (s *Store) writeTx(ctx context.Context) (kvtx.Tx, error) {
	return s.markers.NewTransaction(ctx, true)
}

// _ is a type assertion
var (
	_ block.StoreOps   = (*Store)(nil)
	_ block.StoreParts = (*Store)(nil)
)
