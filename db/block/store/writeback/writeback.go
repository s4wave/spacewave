// Package block_store_writeback keeps every block write in a local store and
// uploads it to a remote store in the background.
//
// A write is acknowledged once the local store holds the block and a durable
// marker names it. Upload drains the markers to the remote store and clears
// each marker only after the remote store holds its block, so writes made
// while the remote is unreachable upload after it returns, across restarts.
// Backfill queues the blocks stored before a remote was chosen.
package block_store_writeback

import (
	"bytes"
	"context"
	"slices"
	"strconv"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// markerPrefix prefixes the marker key of each block awaiting upload.
const markerPrefix = "pending/"

// targetKey names the remote whose backfill completed.
const targetKey = "target"

// uploadBatchSize bounds the blocks one upload request batch carries.
const uploadBatchSize = 64

// backfillBatchSize bounds the blocks one backfill marker transaction queues.
const backfillBatchSize = 256

// errBatchFull stops a marker scan once a batch is full.
var errBatchFull = errors.New("upload batch full")

// Status is the upload state of a Store.
type Status struct {
	// Enabled reports whether writes are queued for upload.
	Enabled bool
	// Pending is the number of blocks awaiting upload.
	Pending int
	// PendingBytes is the size of the blocks awaiting upload.
	PendingBytes int64
	// Err is the last upload failure, cleared by the next successful batch.
	Err error
	// Target is the remote every block stored before it was chosen is queued
	// for, empty until a backfill completes.
	Target string
}

// Store is a local block store whose writes queue for upload.
type Store struct {
	*MarkingStore

	// local holds every block and serves the uploads.
	local block.StoreOps
	// markers holds one marker per block awaiting upload.
	markers kvtx.Store
	// mtx orders marker mutations with their projection into status.
	mtx csync.Mutex
	// bcast guards status and wakes Upload and status watchers.
	bcast broadcast.Broadcast
	// status projects the committed markers.
	status Status
}

// NewStore builds a Store over local, counting the markers left by a previous run.
//
// When enabled is false, writes are not queued until SetEnabled enables them.
func NewStore(ctx context.Context, local block.StoreOps, markers kvtx.Store, enabled bool) (*Store, error) {
	s := &Store{local: local, markers: markers}
	s.MarkingStore = NewMarkingStore(local, s.mark)

	var pending int
	var pendingBytes int64
	var target []byte
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		pending, pendingBytes = 0, 0
		var err error
		target, _, err = tx.Get(ctx, []byte(targetKey))
		if err != nil {
			return err
		}
		return tx.ScanPrefix(ctx, []byte(markerPrefix), func(_, value []byte) error {
			pending++
			pendingBytes += parseMarkerSize(value)
			return nil
		})
	})
	if err != nil {
		return nil, errors.Wrap(err, "count pending uploads")
	}
	s.status = Status{
		Enabled:      enabled,
		Pending:      pending,
		PendingBytes: pendingBytes,
		Target:       string(target),
	}
	return s, nil
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
// Disabling drops the queued markers and the backfill target: the local store
// keeps every block, and the remote store no longer needs them.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if !enabled {
		err := kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			var keys [][]byte
			err := tx.ScanPrefixKeys(ctx, []byte(markerPrefix), func(key []byte) error {
				keys = append(keys, bytes.Clone(key))
				return nil
			})
			if err != nil {
				return err
			}
			for _, key := range keys {
				if err := tx.Delete(ctx, key); err != nil {
					return err
				}
			}
			return tx.Delete(ctx, []byte(targetKey))
		})
		if err != nil {
			return errors.Wrap(err, "drop pending uploads")
		}
	}

	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Enabled = enabled
		if !enabled {
			s.status.Pending, s.status.PendingBytes = 0, 0
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
	status, _ := s.GetStatus()
	if status.Target == target {
		return nil
	}

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

	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte(targetKey), []byte(target))
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

// Upload drains the markers to remote until ctx is canceled.
//
// Returns the first upload failure, which stays in the status until the next
// successful batch. The caller retries with its own backoff.
func (s *Store) Upload(ctx context.Context, remote block.StoreOps) error {
	for {
		// Wake when a marker is queued.
		err := s.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			return s.status.Pending != 0, nil
		})
		if err != nil {
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

// pendingBlock is a queued block and its marker.
type pendingBlock struct {
	key  []byte
	ref  *block.BlockRef
	size int64
}

// scanBatch reads up to uploadBatchSize markers.
func (s *Store) scanBatch(ctx context.Context) ([]pendingBlock, error) {
	var batch []pendingBlock
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		batch = batch[:0]
		err := tx.ScanPrefix(ctx, []byte(markerPrefix), func(key, value []byte) error {
			h := &hash.Hash{}
			if err := h.ParseFromB58(string(key[len(markerPrefix):])); err != nil {
				return errors.Wrap(err, "parse pending upload key")
			}
			batch = append(batch, pendingBlock{
				key:  bytes.Clone(key),
				ref:  block.NewBlockRef(h),
				size: parseMarkerSize(value),
			})
			if len(batch) == uploadBatchSize {
				return errBatchFull
			}
			return nil
		})
		if errors.Is(err, errBatchFull) {
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
// upload.
func (s *Store) uploadBatch(ctx context.Context, remote block.StoreOps, batch []pendingBlock) error {
	entries := make([]*block.PutBatchEntry, 0, len(batch))
	for _, pending := range batch {
		stored, err := s.local.GetStoredBlock(ctx, pending.ref)
		if err != nil {
			return errors.Wrap(err, "read pending block")
		}
		if stored != nil {
			entries = append(entries, &block.PutBatchEntry{Ref: pending.ref, Data: stored.Data, Refs: stored.Refs})
		}
	}
	if len(entries) != 0 {
		if err := remote.PutBlockBatch(ctx, entries); err != nil {
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
	if !s.isEnabled() {
		return nil
	}

	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !s.isEnabled() {
		return nil
	}

	// A repeated write finds its marker and leaves the count unchanged.
	var added int
	var addedBytes int64
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		added, addedBytes = 0, 0
		for _, mark := range marks {
			key := []byte(markerPrefix + mark.Hash.MarshalString())
			found, err := tx.Exists(ctx, key)
			if err != nil {
				return err
			}
			if found {
				continue
			}
			if err := tx.Set(ctx, key, []byte(strconv.FormatInt(mark.Size, 10))); err != nil {
				return err
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
			s.status.Pending += added
			s.status.PendingBytes += addedBytes
			broadcast()
		})
	}
	return nil
}

// clear removes the markers of uploaded blocks and clears the last failure.
func (s *Store) clear(ctx context.Context, batch []pendingBlock) error {
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	var cleared int
	var clearedBytes int64
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		cleared, clearedBytes = 0, 0
		for _, pending := range batch {
			found, err := tx.Exists(ctx, pending.key)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			if err := tx.Delete(ctx, pending.key); err != nil {
				return err
			}
			cleared++
			clearedBytes += pending.size
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "clear uploaded blocks")
	}

	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.Pending -= cleared
		s.status.PendingBytes -= clearedBytes
		s.status.Err = nil
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

// parseMarkerSize parses a marker's recorded block size, or 0 when malformed.
func parseMarkerSize(value []byte) int64 {
	size, _ := strconv.ParseInt(string(value), 10, 64)
	return size
}

// _ is a type assertion
var _ block.StoreOps = (*Store)(nil)
