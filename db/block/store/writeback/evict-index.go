package block_store_writeback

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// evictIndexPrefix maps each hash to its canonical epoch queue key. While a
// marker covers the value, its pointer reserves an absent entry and contributes
// no eviction bytes. Entries, pointers and EvictableBytes change atomically. Index
// repair changes metadata only: it never deletes a block value or remote
// object. A marker covers a value instead of contributing eviction bytes;
// retry-on clear restores queue coverage atomically, even after an absent read.
const evictIndexPrefix = "evictable-index/"

// evictQueueIndex selects indexed accounting; foreground paths read it atomically.
var evictQueueIndex atomic.Bool

// evictIndexPolicy guards the policy revision, waking stores after either
// switch changes. It holds no store registry or storage lock.
var evictIndexPolicy struct {
	// bcast guards revision and wakes each store's owned repair loop.
	bcast broadcast.Broadcast
	// revision invalidates every in-memory repair cursor after a switch change.
	revision atomic.Uint64
}

// SetEvictQueueIndex selects indexed accounting without enabling eviction.
// Both this switch and refusal retry must be on. Turning either off selects
// the legacy queue for new work. A dropped clear honors a retained reservation;
// retry-off passes still have the legacy stranding behavior. Re-enabling starts
// a fresh bounded repair of recorded entries, not a scan of unqueued values.
// No saved completion state is trusted.
func SetEvictQueueIndex(on bool) {
	if evictQueueIndex.Swap(on) != on {
		evictIndexPolicy.bcast.HoldLock(func(changed func(), _ func() <-chan struct{}) {
			evictIndexPolicy.revision.Add(1)
			changed()
		})
	}
}

// readEvictIndexPolicy reads the effective policy and its notification together.
func readEvictIndexPolicy() (on bool, revision uint64, changed <-chan struct{}) {
	evictIndexPolicy.bcast.HoldLock(func(_ func(), wait func() <-chan struct{}) {
		on = evictQueueIndex.Load() && evictRefusedRetry.Load()
		revision = evictIndexPolicy.revision.Load()
		changed = wait()
	})
	return
}

// evictIndexKey returns the hash-first pointer key.
func evictIndexKey(h *hash.Hash) []byte {
	return []byte(evictIndexPrefix + h.MarshalString())
}

// deleteEvictEntry subtracts only an entry actually deleted. A clear or fill
// may have moved the pointer since the scan: delete it only if it still names
// this exact entry. Reads and mutations share the caller's transaction.
func deleteEvictEntry(ctx context.Context, tx kvtx.Tx, entry evictEntry) (int64, error) {
	// Read the live entry rather than subtracting a stale scan's size.
	value, found, err := tx.Get(ctx, entry.key)
	if err != nil || !found {
		return 0, err
	}
	size, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return 0, err
	}
	if err := tx.Delete(ctx, entry.key); err != nil {
		return 0, err
	}

	// Keep a pointer installed by a newer queue move.
	key := evictIndexKey(entry.hash)
	pointer, _, err := tx.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if bytes.Equal(pointer, entry.key) {
		if err := tx.Delete(ctx, key); err != nil {
			return 0, err
		}
	}
	return -size, nil
}

// coverMarkedEvict removes indexed bytes and retains the epoch key as a
// marker-covered reservation. It has no queue entry or byte contribution.
// A later dropped clear honors a retained pointer even if retry was disabled;
// legacy markers without a reservation retain the off policy.
func coverMarkedEvict(ctx context.Context, tx kvtx.Tx, h *hash.Hash) (int64, error) {
	// Read one pointer and reserve coverage for a new marker if none exists.
	key := evictIndexKey(h)
	pointer, found, err := tx.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, tx.Set(ctx, key, evictKey(0, h))
	}

	// Remove only the pointed entry, preserving the pointer as a reservation.
	value, found, err := tx.Get(ctx, pointer)
	if err != nil || !found {
		return 0, err
	}
	size, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return 0, err
	}
	return -size, tx.Delete(ctx, pointer)
}

// queueIndexedEvict moves an indexed entry to epoch, returning the exact byte
// delta. A missing pointer may represent a legacy entry: exact-key lookup
// avoids counting a current-epoch entry twice while repair catches older ones.
func queueIndexedEvict(ctx context.Context, tx kvtx.Tx, epoch uint64, h *hash.Hash, size int64) (int64, error) {
	// A marker already accounts for the value, including a marked cache fill.
	marked, err := tx.Exists(ctx, markedKey(h))
	if err != nil {
		return 0, err
	}
	if marked {
		return coverMarkedEvict(ctx, tx, h)
	}

	// Resolve the old epoch directly, without probing epochs or scanning hashes.
	key, index := evictKey(epoch, h), evictIndexKey(h)
	pointer, _, err := tx.Get(ctx, index)
	if err != nil {
		return 0, err
	}
	var delta int64
	if len(pointer) != 0 && !bytes.Equal(pointer, key) {
		value, found, err := tx.Get(ctx, pointer)
		if err != nil {
			return 0, err
		}
		if found {
			previous, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				return 0, err
			}
			if err := tx.Delete(ctx, pointer); err != nil {
				return 0, err
			}
			delta -= previous
		}
	}

	// Reuse an exact current-epoch entry, including an unindexed legacy one.
	value, found, err := tx.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	var previous int64
	if found {
		previous, err = strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return 0, err
		}
	}
	if !found || previous != size {
		if err := tx.Set(ctx, key, []byte(strconv.FormatInt(size, 10))); err != nil {
			return 0, err
		}
	}
	if !bytes.Equal(pointer, key) {
		if err := tx.Set(ctx, index, key); err != nil {
			return 0, err
		}
	}
	return delta + size - previous, nil
}

// rebuildEvictIndex runs bounded repairs under the store's owning context.
// It does no work while either opt-in switch is off. Every reopen or policy
// revision restarts the walk; revisiting an indexed entry changes no bytes.
func (s *Store) rebuildEvictIndex(ctx context.Context) {
	// Release lifecycle waiters only after the repair stops touching the store.
	defer close(s.indexDone)

	// Yield storage admission to foreground writes and retain ordered recovery.
	ctx = kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(ctx))
	for ctx.Err() == nil {
		// Capture the policy's wait before repairing so a toggle cannot be missed.
		on, revision, changed := readEvictIndexPolicy()
		if on {
			if err := s.repairEvictIndex(ctx, revision); err != nil && ctx.Err() == nil {
				timer := time.NewTimer(countRetry)
				select {
				case <-ctx.Done():
				case <-changed:
				case <-timer.C:
				}
				timer.Stop()
				continue
			}
		}

		// Complete and default-off workers wait on policy, never status traffic.
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
	}
}

// repairEvictIndex walks a closed epoch range. Advancing epoch before the
// walk makes all concurrent inserts sort beyond its boundary; there is no
// foreground scan or startup dependency. No persistent completion bit survives
// a reopen or switch flip. Cursor progress follows successful commits only.
func (s *Store) repairEvictIndex(ctx context.Context, revision uint64) error {
	// Fence legacy producers before the walk with the existing durable epoch.
	var boundary uint64
	err := kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		epoch, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		boundary = epoch + 1
		return tx.Set(ctx, []byte(epochKey), []byte(strconv.FormatUint(boundary, 10)))
	})
	if err != nil {
		return err
	}

	// Read and commit one bounded slice before admitting the next writer.
	var from []byte
	for {
		on, current, _ := readEvictIndexPolicy()
		if !on || current != revision {
			return nil
		}
		next, done, err := s.repairEvictIndexSlice(ctx, from, boundary)
		if err != nil || done {
			return err
		}
		from = next
	}
}

// repairEvictIndexSlice repairs at most 64 scanned entries, keeping only that
// slice in memory. Cold iteration runs outside the writer; exact live reads
// and pointer changes commit atomically and yield before the next slice.
func (s *Store) repairEvictIndexSlice(ctx context.Context, from []byte, boundary uint64) ([]byte, bool, error) {
	// Capture generation outside SetEnabled's published-zero/reset interval,
	// then release its gate before any cold read, I/O or writer admission.
	release, err := s.mtx.Lock(ctx)
	if err != nil {
		return nil, false, err
	}
	gen, _ := s.getEnabledGen()
	release()

	// Take a bounded snapshot of entries before the fenced epoch.
	var batch []evictEntry
	var next []byte
	done := true
	err = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		batch, next, done = nil, nil, true
		err := scanEvictable(ctx, tx, from, func(entry evictEntry) error {
			if entry.epoch >= boundary || len(batch) == evictRetryCommitBlocks {
				next, done = entry.key, entry.epoch >= boundary
				return errScanDone
			}
			batch = append(batch, entry)
			return nil
		})
		if err == errScanDone {
			return nil
		}
		return err
	})
	if err != nil || len(batch) == 0 {
		return next, done, err
	}

	// Reconcile live entries instead of trusting snapshot sizes or pointers.
	var delta int64
	err = kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		delta = 0
		if current, _ := s.getEnabledGen(); current != gen {
			return nil
		}
		for _, entry := range batch {
			n, err := repairEvictEntry(ctx, tx, entry)
			if err != nil {
				return err
			}
			delta += n
		}
		return addEvictableBytes(ctx, tx, delta)
	}, kvtx.RetryInvalidSnapshot)
	if err == nil && delta != 0 {
		s.bcast.HoldLock(func(changed func(), _ func() <-chan struct{}) {
			if s.gen == gen {
				s.status.EvictableBytes += delta
				changed()
			}
		})
	}
	return next, done, err
}

// repairEvictEntry removes marked overlaps and duplicate live entries, or
// installs a missing pointer. It never reads or mutates block values.
func repairEvictEntry(ctx context.Context, tx kvtx.Tx, entry evictEntry) (int64, error) {
	// Recheck whether a concurrent clear or fill removed this snapshot entry.
	found, err := tx.Exists(ctx, entry.key)
	if err != nil || !found {
		return 0, err
	}
	marked, err := tx.Exists(ctx, markedKey(entry.hash))
	if err != nil {
		return 0, err
	}
	if marked {
		delta, err := deleteEvictEntry(ctx, tx, entry)
		if err != nil {
			return 0, err
		}
		// Preserve the clear reservation across runtime policy disablement.
		key := evictIndexKey(entry.hash)
		found, err := tx.Exists(ctx, key)
		if err != nil || found {
			return delta, err
		}
		return delta, tx.Set(ctx, key, entry.key)
	}

	// Prefer the existing live pointer; its entry may have moved beyond the
	// fenced range. An interrupted walk can revisit that canonical entry.
	key := evictIndexKey(entry.hash)
	pointer, exists, err := tx.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if exists && !bytes.Equal(pointer, entry.key) {
		live, err := tx.Exists(ctx, pointer)
		if err != nil {
			return 0, err
		}
		if live {
			return deleteEvictEntry(ctx, tx, entry)
		}
	}
	return 0, tx.Set(ctx, key, entry.key)
}
