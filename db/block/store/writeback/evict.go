package block_store_writeback

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"strconv"
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

// EvictPrefix, EpochKey and EvictableBytesKey are evictPrefix, epochKey and
// evictableBytesKey, for tools that read the store's keys offline.
const (
	EvictPrefix       = evictPrefix
	EpochKey          = epochKey
	EvictableBytesKey = evictableBytesKey
)

// EvictAdmit is called before each eviction batch with the bytes the previous
// batch dropped, zero before the first. It may wait, to pace a pass against the
// disk's other writers; its error ends the pass.
type EvictAdmit func(ctx context.Context, lastBytes int64) error

// EvictStep runs one eviction batch for the caller of EvictSteps, which may
// hold a lock over the batch alone: it calls batch at most once, with the
// store the batch checks as Evict's check describes, and returns batch's
// error. A step that returns nil without calling batch ends the pass after the
// batches before it, which then reports Stopped. EvictSteps calls admit before
// step, so a step holds nothing while the pass is paced.
type EvictStep func(ctx context.Context, batch func(check block.StoreOps) error) error

const (
	// evictBatchBlocks is the entries the first eviction batch of an open
	// reads and commits.
	evictBatchBlocks = 256
	// evictBatchMin and evictBatchMax bound the entries of a later batch,
	// which the writer hold of the last one sizes.
	evictBatchMin = 32
	evictBatchMax = 16384
	// evictHoldHigh and evictHoldLow bound the writer hold of an eviction
	// batch: a batch that held the writer longer shrinks the next to at
	// least half, toward evictHoldTarget, and one that held it shorter
	// doubles it. The values store's writer is shared with every foreground
	// write, which cannot preempt a holder and so waits out the whole hold,
	// while each batch waits behind every foreground writer for its turn, so
	// a batch is as large as its hold allows.
	evictHoldHigh   = 100 * time.Millisecond
	evictHoldLow    = 25 * time.Millisecond
	evictHoldTarget = 50 * time.Millisecond
)

// evictHoldHook observes each batch committed in one write with its entries
// and writer hold, for tests; nil in production.
var evictHoldHook func(entries int, hold time.Duration)

// valueEditor is a values store whose key-value store also holds the marker
// store, such as a Volume, so one write transaction drops values and changes
// the eviction queue together.
type valueEditor interface {
	// EditValues runs fn in one write transaction with the transaction's
	// blocks and a view of the object store storeID, and commits both.
	EditValues(ctx context.Context, storeID string, fn func(ctx context.Context, blocks block.StoreOps, tx kvtx.Tx) error, retry kvtx.RetryPredicate) error
}

// nextEvictBatch sizes the next eviction batch from the last one, which held
// the writer for hold over n entries, as evictHoldHigh describes, within
// evictBatchMin and evictBatchMax.
func nextEvictBatch(n int, hold time.Duration) int {
	next := n
	switch {
	case hold > evictHoldHigh:
		next = min(n/2, int(int64(n)*int64(evictHoldTarget)/int64(hold)))
	case hold < evictHoldLow:
		next = 2 * n
	}
	return min(max(next, evictBatchMin), evictBatchMax)
}

// EvictResult reports what an eviction pass did.
type EvictResult struct {
	// Evicted is the number of local values dropped.
	Evicted int
	// EvictedBytes is the recorded size of the dropped values.
	EvictedBytes int64
	// Requeued is the number of entries the caller kept, queued at the new
	// epoch.
	Requeued int
	// Refused is the number of blocks kept because the remote store may not
	// hold them: a marker awaits upload, or a checked remote does not report
	// them.
	Refused int
	// Batches is the number of batches committed.
	Batches int
	// Hold is the longest writer hold of a batch committed in one write.
	Hold time.Duration
	// Stopped is set when the step ended the pass before its target.
	Stopped bool
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

// ParseEvictKey decodes an eviction queue key, relative to the store's object
// store, into the epoch that queued it and the block hash, as an eviction
// pass does: an error fails the pass.
func ParseEvictKey(key []byte) (uint64, *hash.Hash, error) {
	// Refuse a key outside the prefix, which a pass never sees.
	if !bytes.HasPrefix(key, []byte(evictPrefix)) {
		return 0, nil, errors.Errorf("malformed eviction queue key %q", key)
	}
	entry, err := parseEvictKey(key)
	if err != nil {
		return 0, nil, err
	}
	return entry.epoch, entry.hash, nil
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

// queueEvict sets a block's eviction queue entry at epoch, returning the bytes
// it added to the queue: zero when the entry was already there.
func queueEvict(ctx context.Context, tx kvtx.Tx, epoch uint64, h *hash.Hash, size int64) (int64, error) {
	key := evictKey(epoch, h)
	found, err := tx.Exists(ctx, key)
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
// resident bytes from past the high mark down to the low mark, or zero.
//
// While Counting, the walk that starts the queue's kept count has yet to
// reach part of the queue, by an amount unknown until it ends, so a pass takes
// the queue as past the high mark and drops every uploaded byte: holding
// eviction until the walk ends would let the disk fill for its length, and an
// uploaded block dropped early costs only its refill. The walk runs once per
// store, and over a short queue ends within the open's first moments.
func (s Status) EvictBytes() int64 {
	resident := s.ResidentBytes()
	switch {
	case s.Policy.ResidentHigh <= 0:
		return 0
	case s.Counting:
		return s.EvictableBytes
	case resident <= s.Policy.ResidentHigh:
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
	return ParseEpoch(value)
}

// ParseEpoch decodes the stored eviction epoch, the value of EpochKey, as a
// pass reads it: an error fails the pass. A store without the key is at
// epoch zero.
func ParseEpoch(value []byte) (uint64, error) {
	return strconv.ParseUint(string(value), 10, 64)
}

// PassEpoch returns the epoch a pass advances to from the stored epoch prev.
func PassEpoch(prev uint64) uint64 {
	return prev + 1
}

// EvictWalks reports whether a pass at epoch walks an entry queued at
// entryEpoch: the pass stops at the first entry queued at its epoch or later.
func EvictWalks(entryEpoch, epoch uint64) bool {
	return entryEpoch < epoch
}

// evictAction is what a pass does with one walked eviction entry.
type evictAction int

const (
	// evictDelete deletes the entry of a value already gone.
	evictDelete evictAction = iota
	// evictRefuse deletes the entry and keeps the value: a marker awaits its
	// upload, or the remote does not hold it.
	evictRefuse
	// evictRequeue keeps the value and queues the entry at the pass's epoch.
	evictRequeue
	// evictDrop drops the value and deletes the entry.
	evictDrop
)

// decideEvict decides one walked entry from whether the local values hold
// the block, a marker awaits its upload, the remote holds it (true when the
// pass asks no remote), and keep, called only when nothing else decides,
// keeps it.
func decideEvict(held, marked, stored bool, keep func() bool) evictAction {
	switch {
	case !held:
		return evictDelete
	case marked || !stored:
		return evictRefuse
	case keep():
		return evictRequeue
	}
	return evictDrop
}

// EvictDrops reports whether a pass drops the value of a walked entry, as
// decideEvict decides it.
func EvictDrops(held, marked, stored bool, keep func() bool) bool {
	return decideEvict(held, marked, stored, keep) == evictDrop
}

// Evict runs one eviction pass, one at a time: it advances the epoch, then walks the queue
// oldest epoch first and drops local values until it has dropped maxBytes or
// reaches the new epoch. A block keep reports is queued at the new epoch
// instead. A nil keep keeps nothing.
//
// A block leaves the local values only while no marker awaits its upload.
// With a nil check the queue itself is the proof the remote holds the block,
// which the caller asserts: every entry was written in the transaction that
// cleared an uploaded block's marker, or after a fill read the block from the
// remote, so the pass asks the remote nothing. A caller whose fills may queue
// blocks the remote lacks, such as a peer's block filled without an upload
// marker, passes the remote as check, and a block leaves only while check
// reports holding it. Asking costs a remote index probe per batch, which can
// wait on the pack index writer and on bucket reads, so a pass over a large
// queue that asks can run for days. An entry whose value is already gone, such
// as a block the garbage collector swept, is deleted. Values are dropped with
// their entries in one transaction where the values store holds the queue,
// else before them, so an interrupted pass leaves only entries the next pass
// deletes. Each batch reads and commits on its own, so the pass holds no lock
// across batches, and each sizes the next by its writer hold, as
// evictHoldHigh describes.
//
// The volume shares values by hash across its block stores. Eviction relies on
// each placed store sealing its blocks with its own transform keys, so no other
// store holds the same hash.
//
// A non-nil admit paces the pass, as EvictAdmit describes.
func (s *Store) Evict(
	ctx context.Context,
	check block.StoreOps,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	admit EvictAdmit,
) (EvictResult, error) {
	return s.EvictSteps(ctx, maxBytes, keep, admit, func(_ context.Context, batch func(block.StoreOps) error) error {
		return batch(check)
	})
}

// EvictSteps runs one eviction pass as Evict does, with each batch run by
// step, which names the store the batch checks and may end the pass between
// batches, as EvictStep describes.
func (s *Store) EvictSteps(
	ctx context.Context,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	admit EvictAdmit,
	step EvictStep,
) (EvictResult, error) {
	// Commit the pass ordered: it drops only values the remote holds, so a
	// crash that loses its commits leaves values the next pass drops again,
	// and its writes must not each wait on a flush that other writes share.
	// Its writes wait behind every foreground writer.
	ctx = kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(ctx))

	// Serialize the pass with other passes.
	var result EvictResult
	release, err := s.evictMtx.Lock(ctx)
	if err != nil {
		return result, err
	}
	defer release()

	// Advance the epoch, so entries this pass queues sort after the walk.
	var epoch uint64
	err = kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		prev, err := readEpoch(ctx, tx)
		if err != nil {
			return err
		}
		epoch = PassEpoch(prev)
		return tx.Set(ctx, []byte(epochKey), []byte(strconv.FormatUint(epoch, 10)))
	})
	if err != nil {
		return result, errors.Wrap(err, "advance eviction epoch")
	}

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
		var next []byte
		var done, ran bool
		err := step(ctx, func(check block.StoreOps) error {
			var err error
			ran = true
			next, done, err = s.evictBatch(ctx, check, epoch, maxBytes, keep, s.evictFrom, &result)
			return err
		})
		if err != nil {
			return result, err
		}
		if !ran {
			result.Stopped = true
			return result, nil
		}
		s.evictFrom = next
		if done {
			return result, nil
		}
	}
	return result, nil
}

// evictBatch reads one batch of entries older than epoch from the key from,
// drops the values it may, and commits the entry changes. Returns the key the
// next batch starts from, and true when the walk reached the end of the older
// entries; the key is then the start of epoch, where later entries queue.
func (s *Store) evictBatch(
	ctx context.Context,
	check block.StoreOps,
	epoch uint64,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	from []byte,
	result *EvictResult,
) ([]byte, bool, error) {
	// Read the batch with the markers of its blocks, as many entries as the
	// last batch's writer hold sized.
	size := s.evictBatchSize
	if size == 0 {
		size = evictBatchBlocks
	}
	var batch []evictEntry
	var marked []bool
	var next []byte
	var done bool
	err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		// Walk the older entries up to the batch and byte limits; the walk is
		// done when it runs out of them.
		batch, marked, done = batch[:0], marked[:0], true
		var batchBytes int64
		next = nil
		err := scanEvictable(ctx, tx, from, func(entry evictEntry) error {
			// Stop at the new epoch or a full batch, where the next batch
			// starts.
			if !EvictWalks(entry.epoch, epoch) || len(batch) == size || result.EvictedBytes+batchBytes >= maxBytes {
				done, next = !EvictWalks(entry.epoch, epoch), entry.key
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
	if err != nil {
		return nil, false, errors.Wrap(err, "scan eviction queue")
	}
	if done {
		next = evictEpochKey(epoch)
	}
	if len(batch) == 0 {
		return next, true, nil
	}

	// Ask the local values which blocks they hold, and check which the remote
	// holds when the caller asks; otherwise the queue entry shows it, as Evict
	// describes.
	refs := make([]*block.BlockRef, len(batch))
	for i, entry := range batch {
		refs[i] = block.NewBlockRef(entry.hash)
	}
	held, err := s.values.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, false, errors.Wrap(err, "check local values")
	}
	var stored []bool
	if check != nil {
		stored, err = check.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return nil, false, errors.Wrap(err, "check remote blocks")
		}
	}

	// Decide each entry.
	var requeue []evictEntry
	var drop []*block.PutBatchEntry
	var evictedBytes int64
	for i, entry := range batch {
		kept := func() bool { return keep != nil && keep(entry.hash) }
		switch decideEvict(held[i], marked[i], stored == nil || stored[i], kept) {
		case evictDelete:
		case evictRefuse:
			result.Refused++
		case evictRequeue:
			requeue = append(requeue, entry)
		default:
			drop = append(drop, &block.PutBatchEntry{Ref: refs[i], Tombstone: true})
			evictedBytes += entry.size
		}
	}

	// Drop the values the remote holds, delete the walked entries and queue
	// the kept ones at the new epoch, in one write where the values store
	// holds the queue: the writer is shared with every other write, and each
	// turn waits behind every foreground writer. A refused block's next upload
	// queues it again. The size of the next batch follows the writer hold.
	var delta int64
	editQueue := func(ctx context.Context, tx kvtx.Tx) error {
		delta = 0
		for _, entry := range batch {
			if err := tx.Delete(ctx, entry.key); err != nil {
				return err
			}
			delta -= entry.size
		}
		for _, entry := range requeue {
			added, err := queueEvict(ctx, tx, epoch, entry.hash, entry.size)
			if err != nil {
				return err
			}
			delta += added
		}
		return addEvictableBytes(ctx, tx, delta)
	}
	editor, joint := s.values.(valueEditor)
	if joint {
		var start time.Time
		err = editor.EditValues(ctx, s.markersID, func(ctx context.Context, blocks block.StoreOps, tx kvtx.Tx) error {
			start = time.Now()
			if len(drop) != 0 {
				if _, err := blocks.PutBlockBatch(ctx, drop); err != nil {
					return err
				}
			}
			return editQueue(ctx, tx)
		}, kvtx.RetryInvalidSnapshot)
		if err == nil {
			hold := time.Since(start)
			result.Hold = max(result.Hold, hold)
			if evictHoldHook != nil {
				evictHoldHook(len(batch), hold)
			}
			if len(batch) == size {
				s.evictBatchSize = nextEvictBatch(size, hold)
			}
		} else if !errors.Is(err, kvtx.ErrTxTooBig) && !errors.Is(err, block.ErrAtomicPublicationUnsupported) {
			return nil, false, errors.Wrap(err, "evict blocks")
		}
	}
	if !joint || err != nil {
		if err := s.evictApart(ctx, drop, editQueue); err != nil {
			return nil, false, err
		}
	}
	result.Evicted += len(drop)

	// Report the batch.
	s.addEvictStatus(delta, evictedBytes)
	result.EvictedBytes += evictedBytes
	result.Requeued += len(requeue)
	result.Batches++
	return next, done, nil
}

// evictApart drops the values in drop, then runs editQueue on the eviction
// queue, in two writes: for a values store that does not hold the queue or
// cannot commit atomically, or a batch too big for one transaction. Values go
// first, so an interrupted batch leaves only entries the next pass deletes.
// Each write splits as its store needs, and the queue's reads take no writer
// lock.
func (s *Store) evictApart(ctx context.Context, drop []*block.PutBatchEntry, editQueue func(context.Context, kvtx.Tx) error) error {
	// Drop the values in one write, not one per block: a write per block
	// bounds eviction to a few blocks a second under load.
	if len(drop) != 0 {
		if _, err := s.values.PutBlockBatch(ctx, drop); err != nil {
			return errors.Wrap(err, "evict blocks")
		}
	}

	// Change the queue.
	err := kvtx.RunOptimisticTransaction(ctx, s.writeTx, editQueue, kvtx.RetryInvalidSnapshot)
	return errors.Wrap(err, "update eviction queue")
}
