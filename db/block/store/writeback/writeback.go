// Package block_store_writeback keeps every block write in a local store and
// uploads it to a remote store in the background.
//
// A write is acknowledged once the local store holds the block and a durable
// marker names it. The marker lives in the same key-value store as the block
// and commits in the same transaction (see kvtx.BlockMarker), so a crash
// leaves either both or neither, and a publication of many blocks marks them
// all in its own transaction. Upload drains the markers to the remote store
// and clears each marker only after the remote store holds its block, so
// writes made while the remote is unreachable upload after it returns, across
// restarts. Backfill queues the blocks stored before a remote was chosen.
//
// A marker waits a settle age before it uploads, so a block the garbage
// collector sweeps meanwhile never reaches the remote: the upload finds the
// block gone and drops its marker.
package block_store_writeback

import (
	"bytes"
	"context"
	"slices"
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
	// markers holds one marker per block awaiting upload, in the key-value
	// store that holds the blocks.
	markers kvtx.Store
	// markersID is the ID of the object store that markers views.
	markersID string
	// mtx orders upload clears, backfill queueing and enablement changes with
	// their projection into status. Block writes mark without it, inside their
	// own transactions.
	mtx csync.Mutex
	// evictMtx runs one eviction pass at a time, and disabling uploads waits
	// for the pass to end.
	evictMtx csync.Mutex
	// bcast guards status, and wakes Upload and status watchers.
	bcast broadcast.Broadcast
	// status projects the committed markers and the upload policy.
	status Status
	// gen counts enablement changes, guarded by bcast. A transaction that
	// marked blocks projects them into status only if no change followed it.
	gen uint64
	// reads bounds the upload's local reads in flight by the disk's load.
	reads *readGate
}

// GetStoreParts returns the local store that holds every block.
func (s *Store) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "local", Store: s.local}}
}

// NewStore builds a Store over local, counting the markers left by a previous
// run. Queued blocks upload at once until SetPolicy sets a settle age. values
// holds the same blocks as local without its ownership tracking.
//
// markers is the object store with ID markersID in the key-value store that
// holds the blocks of local. Writes through the Store mark their blocks in the
// transaction that stores them, so local must store its blocks in that
// key-value store with kvtx.BlockMarker support (see the kvtx Volume).
//
// When enabled is false, writes are not queued until SetEnabled enables them.
func NewStore(
	ctx context.Context,
	local, values block.StoreOps,
	markers kvtx.Store,
	markersID string,
	enabled bool,
) (*Store, error) {
	// Connect local block writes to the durable upload markers.
	s := &Store{local: local, values: values, markers: markers, markersID: markersID, reads: newReadGate()}
	s.MarkingStore = NewTxMarkingStore(local, s)

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
	s.reads.setPolicy(policy.MaxUploadReads, policy.UploadReadSlow)
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.status.Policy != policy {
			s.status.Policy = policy
			broadcast()
		}
	})
}

// GetStatus returns the upload status and a channel closed when it changes.
func (s *Store) GetStatus() (Status, <-chan struct{}) {
	// Copy the status and its change channel.
	var status Status
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		status, changed = s.status, getWaitCh()
	})

	// Add the read bound, which moves without a broadcast.
	status.UploadReads = s.reads.getLimit()
	return status, changed
}

// SetEnabled starts or stops queueing writes for upload.
//
// Enabling returns once every write transaction that began while queueing was
// off has ended, so a Backfill that follows lists their blocks. Disabling
// drops the queued markers, the eviction queue and the backfill target: the
// remote store no longer needs the markers, and the local store must keep the
// blocks it holds.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	// Serialize upload enablement with marker mutations.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Drop pending markers and the backfill target after any eviction pass,
	// whose commits would count entries dropped here.
	if !enabled {
		releaseEvict, err := s.evictMtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer releaseEvict()
	}

	// Publish upload enablement first, so a write that starts after this point
	// queues or skips by the new setting, and one in flight projects nothing.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.gen++
		s.status.Enabled = enabled
		if !enabled {
			s.status.Pending, s.status.PendingBytes = 0, 0
			s.status.EvictableBytes = 0
			s.status.Oldest = time.Time{}
			s.status.Err, s.status.Target = nil, ""
		}
		broadcast()
	})

	// A write transaction ahead of this one in the store's order either marked
	// by the old setting or ends before the write below returns. Dropping the
	// markers also removes those a disabled-to-enabled race left behind.
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		if enabled {
			return nil
		}
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
		return errors.Wrap(err, "change upload enablement")
	}
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
		if err := s.queue(ctx, marks); err != nil {
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

// GetUploadReads returns how many local reads the upload currently keeps in
// flight at most.
func (s *Store) GetUploadReads() int {
	return s.reads.getLimit()
}

// MarkerStoreID returns the ID of the object store that holds the markers.
func (s *Store) MarkerStoreID() string {
	return s.markersID
}

// MarkBlocks queues the blocks a write stores, in the transaction that stores
// them. The queue status follows once the transaction commits.
//
// A disabled store queues nothing. SetEnabled orders itself against the
// transactions in flight, so a block stored while disabled is listed by the
// Backfill that follows enabling.
func (s *Store) MarkBlocks(ctx context.Context, tx kvtx.Tx, blocks []kvtx.BlockMark) (func(), error) {
	// Skip upload markers while queueing is disabled.
	gen, enabled := s.getEnabledGen()
	if !enabled {
		return nil, nil
	}

	// Queue the blocks at one write time.
	now := time.Now()
	marks := make([]Mark, len(blocks))
	for i, b := range blocks {
		marks[i] = Mark{Hash: b.Hash, Size: b.Size, At: now}
	}
	delta, err := queueMarks(ctx, tx, marks)
	if err != nil {
		return nil, errors.Wrap(err, "queue block upload")
	}
	if delta.added == 0 {
		return nil, nil
	}
	return func() { s.applyQueued(gen, delta) }, nil
}

// queue queues blocks for upload in one transaction of their own, as a
// backfill does for blocks that are stored already.
//
// A disabled store skips the lock. The check repeats under the lock so a call
// racing SetEnabled(false) cannot queue after the queue is dropped.
func (s *Store) queue(ctx context.Context, marks []Mark) error {
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
	gen, enabled := s.getEnabledGen()
	if !enabled {
		return nil
	}

	// Queue the markers in one write transaction.
	var delta queueDelta
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		delta, err = queueMarks(ctx, tx, marks)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "queue block upload")
	}
	s.applyQueued(gen, delta)
	return nil
}

// applyQueued projects blocks queued by a committed transaction into the
// status and wakes Upload, unless queueing was enabled or disabled since the
// transaction marked them: the change dropped or rebuilt the status.
func (s *Store) applyQueued(gen uint64, delta queueDelta) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Ignore a transaction that predates the latest enable or disable.
		if s.gen != gen || !s.status.Enabled {
			return
		}
		if s.status.Pending <= 0 || delta.oldest.Before(s.status.Oldest) {
			s.status.Oldest = delta.oldest
		}
		s.status.Pending += delta.added
		s.status.PendingBytes += delta.addedBytes
		broadcast()
	})
}

// clear removes the markers of uploaded blocks, queues them for eviction at
// the current epoch, and clears the last failure.
//
// A dropped block written again since the batch read it keeps its marker: the
// write changed the queue entry's value, and the block is on the local store
// alone.
func (s *Store) clear(ctx context.Context, batch []pendingBlock) error {
	// Serialize uploaded marker removal with queue status updates.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Remove uploaded block markers, count their cleared bytes, and read the
	// write time of the queue's new head.
	var cleared, uploaded int
	var clearedBytes, uploadedBytes, queuedBytes int64
	var oldest time.Time
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Delete the markers still queued, counting what they held, and queue
		// the uploaded blocks for eviction.
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		cleared, uploaded, clearedBytes, uploadedBytes, queuedBytes = 0, 0, 0, 0, 0
		for _, pending := range batch {
			value, found, err := tx.Get(ctx, pending.queueKey)
			if err != nil {
				return err
			}
			if !found || (pending.dropped && !bytes.Equal(value, pending.value)) {
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
				uploaded++
				uploadedBytes += pending.size
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
		s.status.Uploaded += int64(uploaded)
		s.status.UploadedBytes += uploadedBytes
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

// getEnabledGen reports whether writes are queued for upload, and the count of
// enablement changes that setting follows.
func (s *Store) getEnabledGen() (uint64, bool) {
	var gen uint64
	var enabled bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		gen, enabled = s.gen, s.status.Enabled
	})
	return gen, enabled
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
	_ kvtx.BlockMarker = (*Store)(nil)
)
