package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// evictPrefix prefixes the eviction queue entry of each uploaded block, ordered
// by the pass that queued it: evictable/<16 hex digits of epoch>/<hash>. The
// value is the block size.
const evictPrefix = "evictable/"

// epochKey holds the current eviction epoch, which each pass advances.
const epochKey = "epoch"

// evictableBytesKey holds the summed size of the eviction queue entries.
const evictableBytesKey = "evictable-bytes"

// EvictAdmit is called before each eviction batch with the bytes the previous
// batch dropped, zero before the first. It may wait, to pace a pass against the
// disk's other writers; its error ends the pass.
type EvictAdmit func(ctx context.Context, lastBytes int64) error

// evictRetryCommitBlocks bounds each metadata write when refusals are retried.
// The scan and remote probe keep their bulk size; a queue move always deletes
// its old key, inserts its replacement and adjusts bytes in one transaction.
const evictRetryCommitBlocks = 64

// evictRefusedRetry selects refusal retention and queue-before-value fills.
// The default preserves the earlier refusal-dropping behavior until enabled.
var evictRefusedRetry atomic.Bool

// SetEvictRefusedRetry selects refusal retention for passes started afterward.
// When on, a refused entry moves to the new epoch for a later pass to recheck,
// and a fill reserves its queue entry before exposing the local value.
// Turning it off restores refusal dropping and the earlier fill ordering
// without enabling eviction. Each pass and fill samples it once.
func SetEvictRefusedRetry(on bool) {
	if evictRefusedRetry.Swap(on) != on {
		evictIndexPolicy.bcast.HoldLock(func(changed func(), _ func() <-chan struct{}) {
			evictIndexPolicy.revision.Add(1)
			changed()
		})
	}
}

// EvictResult reports what an eviction pass did.
type EvictResult struct {
	// Evicted is the number of local values dropped.
	Evicted int
	// EvictedBytes is the recorded size of the dropped values.
	EvictedBytes int64
	// Requeued is the number of entries the caller kept, queued at the new
	// epoch.
	// This excludes Refused even when refusal retry moves those entries too.
	Requeued int
	// Refused counts decisions to keep a block because a marker awaits upload
	// or the remote does not report holding it, even if a later write fails.
	// These entries move to the new epoch when refusal retry is enabled;
	// indexed marked values instead retain their marker's clear reservation.
	Refused int
}

// evictEntry is a decoded eviction queue entry.
type evictEntry struct {
	// key is the entry's key, owned by the entry.
	key []byte
	// epoch is the pass that queued the entry.
	epoch uint64
	// hash is the block hash.
	hash *hash.Hash
	// size is the block size in bytes.
	size int64
}

// evictKey returns the eviction queue key of a block queued at epoch.
func evictKey(epoch uint64, h *hash.Hash) []byte {
	return append(evictEpochKey(epoch), h.MarshalString()...)
}

// evictEpochKey returns the prefix of the eviction queue keys queued at epoch,
// which sorts before all of them.
func evictEpochKey(epoch uint64) []byte {
	// Encode the epoch as fixed-width hex, so keys sort by epoch.
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], epoch)
	key := []byte(evictPrefix)
	key = hex.AppendEncode(key, stamp[:])
	return append(key, '/')
}

// parseEvictKey decodes an eviction queue key into an entry without its size.
func parseEvictKey(key []byte) (evictEntry, error) {
	// Split the key into its epoch and hash.
	rest := key[len(evictPrefix):]
	if len(rest) <= queueTimeLen || rest[queueTimeLen] != '/' {
		return evictEntry{}, errors.Errorf("malformed eviction queue key %q", key)
	}

	// Decode the epoch and the block hash.
	var stamp [8]byte
	if _, err := hex.Decode(stamp[:], rest[:queueTimeLen]); err != nil {
		return evictEntry{}, errors.Wrap(err, "parse eviction queue epoch")
	}
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(rest[queueTimeLen+1:])); err != nil {
		return evictEntry{}, errors.Wrap(err, "parse eviction queue hash")
	}
	return evictEntry{key: bytes.Clone(key), epoch: binary.BigEndian.Uint64(stamp[:]), hash: h}, nil
}

// scanEvictable calls cb with each eviction queue entry from the key from,
// oldest epoch first, until cb returns an error. A nil from starts at the
// oldest entry.
func scanEvictable(ctx context.Context, tx kvtx.Tx, from []byte, cb func(evictEntry) error) error {
	// Walk eviction entries from the previous batch boundary.
	it := tx.Iterate(ctx, []byte(evictPrefix), true, false)
	defer it.Close()
	if err := it.Seek(from); err != nil {
		return err
	}
	for valid := it.Valid(); valid; valid = it.Next() {
		entry, err := parseEvictKey(it.Key())
		if err != nil {
			return err
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		entry.size, _ = strconv.ParseInt(string(value), 10, 64)
		if err := cb(entry); err != nil {
			return err
		}
	}
	return it.Err()
}

// queueEvict queues a block at epoch and returns its signed byte delta.
// Indexed moves replace one canonical entry; a marker instead covers the
// value. Legacy mode deduplicates only the exact epoch/hash key.
func queueEvict(ctx context.Context, tx kvtx.Tx, epoch uint64, h *hash.Hash, size int64) (int64, error) {
	// Select one policy inside the transaction; legacy writes invalidate pointers.
	indexed := evictQueueIndex.Load() && evictRefusedRetry.Load()
	if indexed {
		return queueIndexedEvict(ctx, tx, epoch, h, size)
	}
	pointer, found, err := tx.Get(ctx, evictIndexKey(h))
	if err != nil {
		return 0, err
	}
	if found {
		marked, err := tx.Exists(ctx, markedKey(h))
		if err != nil {
			return 0, err
		}
		if !marked && len(pointer) != 0 {
			if err := tx.Delete(ctx, evictIndexKey(h)); err != nil {
				return 0, err
			}
		}
	}

	// Preserve the legacy epoch/hash insertion while the index is disabled.
	key := evictKey(epoch, h)
	found, err = tx.Exists(ctx, key)
	if err != nil || found {
		return 0, err
	}
	return size, tx.Set(ctx, key, []byte(strconv.FormatInt(size, 10)))
}

// readEvictableBytes reads the summed size of the eviction queue entries.
func readEvictableBytes(ctx context.Context, tx kvtx.Tx) (int64, error) {
	value, found, err := tx.Get(ctx, []byte(evictableBytesKey))
	if err != nil || !found {
		return 0, err
	}
	return strconv.ParseInt(string(value), 10, 64)
}

// addEvictableBytes adds delta to the summed size of the eviction queue
// entries, in the transaction that changed them.
func addEvictableBytes(ctx context.Context, tx kvtx.Tx, delta int64) error {
	if delta == 0 {
		return nil
	}
	total, err := readEvictableBytes(ctx, tx)
	if err != nil {
		return err
	}
	return tx.Set(ctx, []byte(evictableBytesKey), []byte(strconv.FormatInt(total+delta, 10)))
}

// EvictBytes returns how many uploaded bytes a pass drops to bring the
// resident bytes from past the high mark down to the low mark, or zero. It
// starts a pass only past the high mark; EvictLatch carries a cut-short pass on
// to the low mark.
func (s Status) EvictBytes() int64 {
	resident := s.ResidentBytes()
	if s.Policy.ResidentHigh <= 0 || resident <= s.Policy.ResidentHigh {
		return 0
	}
	return min(resident-s.Policy.ResidentLow, s.EvictableBytes)
}

// addEvictStatus projects committed eviction queue changes into the status:
// deltas commute, so concurrent transactions project in any order. A change
// of nothing wakes no watcher, so a budget watcher whose pass changed nothing
// waits for the next write.
func (s *Store) addEvictStatus(evictable, evicted int64) {
	if evictable == 0 && evicted == 0 {
		return
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.status.EvictableBytes += evictable
		s.status.EvictedBytes += evicted
		broadcast()
	})
}

// readEpoch reads the current eviction epoch, zero before the first pass.
func readEpoch(ctx context.Context, tx kvtx.Tx) (uint64, error) {
	value, found, err := tx.Get(ctx, []byte(epochKey))
	if err != nil || !found {
		return 0, err
	}
	return strconv.ParseUint(string(value), 10, 64)
}

// Evict runs one eviction pass, one at a time: it advances the epoch, then walks the queue
// oldest epoch first and drops local values until it has dropped maxBytes or
// reaches the new epoch. A block keep reports is queued at the new epoch
// instead. A nil keep keeps nothing.
//
// A block leaves the local values only while no marker awaits its upload and
// remote reports holding it, so the remote store always holds what was
// dropped. An entry whose value is already gone, such as a block the garbage
// collector swept, is deleted. Values are dropped before their entries, so an
// interrupted pass leaves only entries the next pass deletes. The pass takes
// no lock a write waits on, since asking remote can wait on the network.
//
// The volume shares values by hash across its block stores. Eviction relies on
// each placed store sealing its blocks with its own transform keys, so no other
// store holds the same hash.
//
// A non-nil admit paces the pass, as EvictAdmit describes.
//
// Eviction drops a local value only when no upload marker was observed and
// the remote confirms holding it. Refusal retry never bypasses those checks.
// With refusal retry enabled, an entry leaves the queue only after its local
// value is dropped, already absent, or covered by a marker whose clear
// reserves replacement coverage. Other refused or caller-kept entries move
// atomically to the new epoch. A failed queue commit leaves the old entry.
// An entry moved to the pass epoch is not visited again in that pass.
//
// A transaction the store reports busy runs again after a backoff, as
// kvtx.RetryBusy describes, each attempt its own short transaction; a pass
// ends on a busy store only after those retries.
func (s *Store) Evict(
	ctx context.Context,
	remote block.StoreOps,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	admit EvictAdmit,
) (EvictResult, error) {
	// Commit the pass ordered: it drops only values the remote holds, so a
	// crash that loses its commits leaves values the next pass drops again,
	// and its writes must not each wait on a flush that other writes share.
	// Its writes use background admission.
	ctx = block.WithBackground(kvtx.WithOrderedCommitContext(ctx))

	// Keep one queue policy for the pass. With retry on, fillMtx is a barrier
	// around the epoch advance only: a fill holds it from its queue
	// reservation through its value write and reserves under the epoch it
	// reads, so every fill that held it before has written its value, and
	// every later fill reserves under the new epoch, which this pass neither
	// visits nor deletes. No fill waits on the pass's remote probes, pacing or
	// slices. It is taken before the pass mutex, so a pass waiting for a fill
	// does not delay disable.
	var result EvictResult
	retryRefused := evictRefusedRetry.Load()

	// Serialize the pass with other passes and disabling uploads, and advance
	// the epoch, so entries this pass or a later fill queues sort after the
	// walk. An attempt the store reports busy releases both mutexes for its
	// backoff, as kvtx.RetryBusy describes, so no fill or disable waits out a
	// retry.
	var epoch uint64
	var release func()
	err := kvtx.RetryBusy(ctx, func() (err error) {
		var rel func()
		rel, epoch, err = s.advanceEpoch(ctx, retryRefused)
		release = rel
		return err
	})
	if err != nil {
		return result, err
	}
	defer release()

	// Evict in batches until the byte target or the new epoch, each from
	// where the last committed batch stopped, as evictFrom describes.
	var last int64
	for result.EvictedBytes < maxBytes {
		if admit != nil {
			if err := admit(ctx, result.EvictedBytes-last); err != nil {
				return result, err
			}
		}
		last = result.EvictedBytes
		next, done, err := s.evictBatch(ctx, remote, epoch, maxBytes, keep, retryRefused, s.evictFrom, &result)
		if err != nil {
			return result, err
		}
		s.evictFrom = next
		if done {
			return result, nil
		}
	}
	return result, nil
}

// advanceEpoch takes the fill barrier when retryRefused is set, then the pass
// mutex, and advances the eviction epoch in one transaction. It returns the
// pass mutex's release and the new epoch, with the fill barrier released; on
// an error it holds neither.
func (s *Store) advanceEpoch(ctx context.Context, retryRefused bool) (func(), uint64, error) {
	// Hold the fill barrier through the epoch advance only.
	if retryRefused {
		releaseFill, err := s.fillMtx.Lock(ctx)
		if err != nil {
			return nil, 0, err
		}
		defer releaseFill()
	}

	// Serialize the pass with other passes and disabling uploads.
	release, err := s.evictMtx.Lock(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Advance the epoch.
	var epoch uint64
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		prev, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		epoch = prev + 1
		return tx.Set(ctx, []byte(epochKey), []byte(strconv.FormatUint(epoch, 10)))
	})
	if err != nil {
		release()
		return nil, 0, errors.Wrap(err, "advance eviction epoch")
	}
	return release, epoch, nil
}

// evictBatch reads one batch of entries older than epoch from the key from,
// drops the values it may, and commits the entry changes. Returns the key the
// next batch starts from, and true when the walk reached the end of the older
// entries; the key is then the start of epoch, where later entries queue.
func (s *Store) evictBatch(
	ctx context.Context,
	remote block.StoreOps,
	epoch uint64,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	retryRefused bool,
	from []byte,
	result *EvictResult,
) ([]byte, bool, error) {
	// Read the batch with the markers of its blocks, up to the block limit the
	// last drop write set within the bounds in force, again after a backoff
	// while the store reports itself busy.
	bounds := loadEvictBatch()
	blocks := bounds.clamp(s.evictLimit)
	var (
		batch  []evictEntry
		marked []bool
		next   []byte
		done   bool
	)
	err := kvtx.RetryBusy(ctx, func() error {
		return kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
			// Walk the older entries up to the batch and byte limits; the walk is
			// done when it runs out of them.
			batch, marked, done = batch[:0], marked[:0], true
			var batchBytes int64
			next = nil
			err := scanEvictable(ctx, tx, from, func(entry evictEntry) error {
				// Stop at the new epoch or a full batch, where the next batch
				// starts. A batch takes its first entry whatever its size.
				full := len(batch) == blocks || (len(batch) != 0 && batchBytes+entry.size > bounds.MaxBytes)
				if entry.epoch >= epoch || full || result.EvictedBytes+batchBytes >= maxBytes {
					done, next = entry.epoch >= epoch, entry.key
					return errScanDone
				}

				// Take the entry with whether a marker awaits its upload.
				found, err := tx.Exists(ctx, markedKey(entry.hash))
				if err != nil {
					return err
				}
				batch, marked = append(batch, entry), append(marked, found)
				batchBytes += entry.size
				return nil
			})
			if errors.Is(err, errScanDone) {
				return nil
			}
			return err
		})
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "scan eviction queue")
	}
	if done {
		next = evictEpochKey(epoch)
	}
	if len(batch) == 0 {
		return next, true, nil
	}

	// Ask the local values and the remote which blocks they hold.
	refs := make([]*block.BlockRef, len(batch))
	for i, entry := range batch {
		refs[i] = block.NewBlockRef(entry.hash)
	}
	held, err := s.values.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, false, errors.Wrap(err, "check local values")
	}
	stored, err := remote.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, false, errors.Wrap(err, "check remote blocks")
	}

	// Decide each entry.
	var requeue []evictEntry
	var drop []*block.PutBatchEntry
	var evictedBytes int64
	var kept int
	for i, entry := range batch {
		switch {
		case !held[i]:
		case marked[i] || !stored[i]:
			result.Refused++
			if retryRefused {
				requeue = append(requeue, entry)
			}
		case keep != nil && keep(entry.hash):
			requeue = append(requeue, entry)
			kept++
		default:
			drop = append(drop, &block.PutBatchEntry{Ref: refs[i], Tombstone: true})
			evictedBytes += entry.size
		}
	}

	// Drop the values the remote holds in one write, not one per block: the
	// local writer is shared with every other write, so a write per block
	// bounds eviction to a few blocks a second under load. The write's time
	// sets the next batch's limit: the attempt that committed, not the waits
	// of a busy store before it.
	if len(drop) != 0 {
		var took time.Duration
		err := kvtx.RetryBusy(ctx, func() error {
			start := time.Now()
			_, err := s.values.PutBlockBatch(ctx, drop)
			took = time.Since(start)
			return err
		})
		if err != nil {
			return nil, false, errors.Wrap(err, "evict blocks")
		}
		s.evictLimit = nextEvictLimit(bounds, blocks, len(drop), took)
		result.Evicted += len(drop)
	}

	// Retry adds a lookup and insertion per refusal. Bound the writer hold
	// with smaller metadata transactions while preserving the legacy batch
	// boundary when retry is off. Each move and its byte change stay atomic.
	limit := len(batch)
	if retryRefused {
		limit = evictRetryCommitBlocks
	}
	for first := 0; first < len(batch); first += limit {
		// Partition the retained entries in the same key order as the scan.
		end := min(first+limit, len(batch))
		n := 0
		for n < len(requeue) && bytes.Compare(requeue[n].key, batch[end-1].key) <= 0 {
			n++
		}
		retained := requeue[:n]
		var delta int64
		err = kvtx.RetryBusy(ctx, func() error {
			return kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
				delta = 0
				retainedAt := 0
				for _, entry := range batch[first:end] {
					// Only a live scanned row owns the right to be moved by this pass.
					removed, found, err := deleteEvictEntry(ctx, tx, entry)
					if err != nil {
						return err
					}
					delta += removed
					if retainedAt < len(retained) && bytes.Equal(retained[retainedAt].key, entry.key) {
						retainedAt++
						if found {
							added, err := queueEvict(ctx, tx, epoch, entry.hash, entry.size)
							if err != nil {
								return err
							}
							delta += added
						}
					}
				}
				return addEvictableBytes(ctx, tx, delta)
			}, kvtx.RetryInvalidSnapshot)
		})
		if err != nil {
			return nil, false, errors.Wrap(err, "update eviction queue")
		}

		// Project each committed slice; drop-byte reporting keeps the original
		// boundary after all metadata for this scan batch commits.
		var reportedBytes int64
		if end == len(batch) {
			reportedBytes = evictedBytes
		}
		s.addEvictStatus(delta, reportedBytes)
		requeue = requeue[n:]
	}

	// Report the batch.
	result.EvictedBytes += evictedBytes
	result.Requeued += kept
	return next, done, nil
}
