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

// Bounds of one eviction batch. A batch reads at most the store's current
// block limit of entries, and stops before an entry that would take it past
// the batch byte bound, evictBatchBytes by default. The limit starts at
// evictBatchStartBlocks on each open and follows the time the batch's drop
// write takes, as nextEvictLimit describes, between the batch's least and
// most blocks, evictBatchMinBlocks and evictBatchMaxBlocks by default.
//
// The drop write holds the store's writer for a time that grows with the
// blocks it drops, not their bytes: each tombstone is a row delete that may
// rebalance its B-tree page and read its neighbors. On a store much larger
// than the page cache that costs from a quarter of a millisecond to several
// milliseconds a block, so a fixed count large enough to move many bytes
// would hold the writer for seconds on a slow disk.
const (
	evictBatchStartBlocks = 1024
	evictBatchMinBlocks   = 128
	evictBatchMaxBlocks   = 8192
	evictBatchBytes       = 32 << 20
)

// evictRetryCommitBlocks bounds each metadata write when refusals are retried.
// The scan and remote probe keep their bulk size; a queue move always deletes
// its old key, inserts its replacement and adjusts bytes in one transaction.
const evictRetryCommitBlocks = 64

// ErrEvictRetryUnchecked is returned by a pass with refusal retry on and a nil
// check: a refused entry stays queued under retry, and a pass without a check
// would drop it on the queue's word alone.
var ErrEvictRetryUnchecked = errors.New("eviction with refusal retry needs a remote check")

// evictRefusedRetry selects whether passes retain refused entries.
// The default preserves the earlier refusal-dropping behavior until enabled.
var evictRefusedRetry atomic.Bool

// SetEvictRefusedRetry selects refusal retention for passes started afterward.
// When on, a refused entry moves to the new epoch for a later pass to recheck.
// Turning it off restores refusal-dropping behavior without enabling eviction.
func SetEvictRefusedRetry(on bool) {
	evictRefusedRetry.Store(on)
}

// evictWriteTarget is the time a batch's drop write aims to take by default,
// half the longest a background write may hold the writer, so a batch whose
// blocks cost twice the last batch's still releases it within a second.
const evictWriteTarget = 500 * time.Millisecond

// EvictBatch bounds the eviction batches of every store in the process, as
// SetEvictBatch sets them. A zero field keeps its default.
type EvictBatch struct {
	// MinBlocks is the least block limit a batch shrinks to, 128 by default.
	MinBlocks int
	// MaxBlocks is the most blocks a batch takes, 8192 by default.
	MaxBlocks int
	// MaxBytes is the most bytes a batch takes past its first entry, 32 MiB
	// by default.
	MaxBytes int64
	// WriteTarget is the time a batch's drop write aims to take, 500 ms by
	// default. The next batch's block limit scales to it.
	WriteTarget time.Duration
}

// evictBatchConfig holds the bounds SetEvictBatch set, nil for the defaults.
var evictBatchConfig atomic.Pointer[EvictBatch]

// SetEvictBatch sets the bounds of the eviction batches of every store in the
// process. A pass running reads them at its next batch, which clamps the
// store's block limit into the new bounds, so lowering them applies within a
// batch. A zero field keeps its default, a negative one too, and a least
// block limit past the most is lowered to it.
func SetEvictBatch(b EvictBatch) {
	b = b.normalize()
	evictBatchConfig.Store(&b)
}

// loadEvictBatch returns the bounds in force.
func loadEvictBatch() EvictBatch {
	if b := evictBatchConfig.Load(); b != nil {
		return *b
	}
	return EvictBatch{}.normalize()
}

// normalize fills the defaults of the zero or negative fields and keeps
// MinBlocks at most MaxBlocks.
func (b EvictBatch) normalize() EvictBatch {
	if b.MinBlocks <= 0 {
		b.MinBlocks = evictBatchMinBlocks
	}
	if b.MaxBlocks <= 0 {
		b.MaxBlocks = evictBatchMaxBlocks
	}
	if b.MaxBytes <= 0 {
		b.MaxBytes = evictBatchBytes
	}
	if b.WriteTarget <= 0 {
		b.WriteTarget = evictWriteTarget
	}
	b.MinBlocks = min(b.MinBlocks, b.MaxBlocks)
	return b
}

// clamp returns limit within the batch's block bounds.
func (b EvictBatch) clamp(limit int) int {
	return min(max(limit, b.MinBlocks), b.MaxBlocks)
}

// nextEvictLimit returns the block limit of the batch after one that dropped
// dropped blocks under limit in elapsed: the count that would take b's write
// target at the same cost a block, moving at most a factor of two from limit
// and kept within b's block bounds. A batch that dropped nothing measured
// nothing and keeps limit. elapsed includes the write's wait for the writer
// turn, so a busy writer shrinks the batches too.
func nextEvictLimit(b EvictBatch, limit, dropped int, elapsed time.Duration) int {
	// Keep the limit when the batch measured nothing.
	if dropped == 0 {
		return limit
	}

	// Scale the batch to the target at its cost a block, within the bounds.
	elapsed = max(elapsed, time.Millisecond)
	next := int(int64(dropped) * int64(b.WriteTarget) / int64(elapsed))
	next = min(max(next, limit/2), limit*2)
	return b.clamp(next)
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
	// or a checked remote does not report holding it, even if a later write
	// fails. These entries move to the new epoch when refusal retry is enabled.
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
// resident bytes from past the high mark down to the low mark, or zero. It
// starts a pass only past the high mark; EvictLatch carries a cut-short pass on
// to the low mark.
//
// While Counting, the walk that starts the queue's kept count has yet to
// reach part of the queue, so the resident bytes are short by the blocks
// awaiting upload it has not counted. A pass still takes the marks, on the
// bytes counted so far: a store whose counted resident bytes are under the
// high mark evicts nothing while they stay there, which can last until the
// walk ends, and one past it on them evicts toward the low mark on them. The
// pending bytes the walk counts later start the next pass. Dropping every
// uploaded byte while Counting could empty the local copies of a store whose
// walk runs long.
func (s Status) EvictBytes() int64 {
	resident := s.ResidentBytes()
	switch {
	case s.Policy.ResidentHigh <= 0:
		return 0
	case resident <= s.Policy.ResidentHigh:
		return 0
	}
	return min(resident-s.Policy.ResidentLow, s.EvictableBytes)
}

// EvictLatch carries an eviction loop from the high mark down to the low mark
// across passes: once the resident bytes cross the high mark, Bytes keeps
// returning the bytes down to the low mark after a pass cut short leaves them
// between the marks, until they reach the low mark, no uploaded byte is left
// to evict, or either mark changes. Without it a pass that ends early, such as
// on a busy database, starts again only once the resident bytes cross the high
// mark again. The latch decides only when a pass starts; each pass still
// commits in the same short batches. The zero value is released.
type EvictLatch struct {
	// held is set while a pass started at the high mark has yet to reach
	// the low mark.
	held bool
	// high and low are the marks the latch was set under.
	high, low int64
}

// Bytes returns how many uploaded bytes the next pass drops: EvictBytes when
// positive, which sets the latch, or else, while the latch holds under the same
// marks, the bytes from the resident bytes down to the low mark. Otherwise it
// releases the latch and returns zero.
func (l *EvictLatch) Bytes(s Status) int64 {
	// Set the latch when the store is past the high mark.
	if want := s.EvictBytes(); want > 0 {
		l.held, l.high, l.low = true, s.Policy.ResidentHigh, s.Policy.ResidentLow
		return want
	}

	// Keep evicting toward the low mark while the latch holds under the same
	// marks and there is something to drop.
	var want int64
	if l.held && s.Policy.ResidentHigh == l.high && s.Policy.ResidentLow == l.low {
		want = min(s.ResidentBytes()-s.Policy.ResidentLow, s.EvictableBytes)
	}
	if want <= 0 {
		l.held, want = false, 0
	}
	return want
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
// as a block the garbage collector swept, is deleted. Values are dropped before
// their entries, so an interrupted pass leaves only entries the next pass
// deletes. Each batch reads and commits on its own, so the pass holds no lock
// across batches. A transaction the store reports busy runs again after a
// backoff, as retryBusy describes, each attempt its own short transaction; a
// pass ends on a busy store only after those retries.
//
// The volume shares values by hash across its block stores. Eviction relies on
// each placed store sealing its blocks with its own transform keys, so no other
// store holds the same hash.
//
// A non-nil admit paces the pass, as EvictAdmit describes.
//
// Eviction drops a local value only when no upload marker was observed and,
// with a check, the remote confirms holding it. Refusal retry never bypasses
// those checks. With refusal retry enabled, an entry leaves the queue only
// after its local value is dropped or already absent. A refused or caller-kept
// entry moves atomically to the new epoch. A failed queue commit leaves the old
// entry. An entry moved to the pass epoch is not visited again in that pass.
// With refusal retry enabled a nil check returns ErrEvictRetryUnchecked
// before the pass changes anything.
func (s *Store) Evict(
	ctx context.Context,
	check block.StoreOps,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	admit EvictAdmit,
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

	// Keep one refusal policy for the entire pass, including all its batches.
	// Under retry a refused entry stays queued, so only a checked pass may
	// read the queue.
	retryRefused := evictRefusedRetry.Load()
	if retryRefused && check == nil {
		return result, ErrEvictRetryUnchecked
	}

	// Advance the epoch, so entries this pass queues sort after the walk.
	epoch, err := s.advanceEvictEpoch(ctx)
	if err != nil {
		return result, err
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
		next, done, err := s.evictBatch(ctx, check, epoch, maxBytes, keep, retryRefused, s.evictFrom, &result)
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

// advanceEvictEpoch advances the eviction epoch and returns the new one, so
// the entries a pass queues sort after its walk.
func (s *Store) advanceEvictEpoch(ctx context.Context) (uint64, error) {
	var epoch uint64
	err := retryBusy(ctx, func() error {
		return kvtx.RunTransaction(ctx, true, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			prev, err := readEpoch(ctx, tx)
			if err != nil {
				return err
			}
			epoch = prev + 1
			return tx.Set(ctx, []byte(epochKey), []byte(strconv.FormatUint(epoch, 10)))
		})
	})
	if err != nil {
		return 0, errors.Wrap(err, "advance eviction epoch")
	}
	return epoch, nil
}

// evictScan is one batch of eviction queue entries older than the pass
// epoch, read in one snapshot with whether a marker awaits each upload.
type evictScan struct {
	// batch holds the entries in key order.
	batch []evictEntry
	// marked reports for each entry whether a marker awaits its upload.
	marked []bool
	// next is the key the batch after this one starts from.
	next []byte
	// done reports the walk reached the end of the older entries; next is
	// then the start of the pass epoch.
	done bool
	// bounds and limit are the batch bounds and block limit in force.
	bounds EvictBatch
	limit  int
}

// scanEvictBatch reads one batch of entries older than epoch from the key
// from, up to the block limit the last drop write set within the bounds in
// force, and stops before the batch's bytes and evicted reach maxBytes.
func (s *Store) scanEvictBatch(ctx context.Context, epoch uint64, maxBytes, evicted int64, from []byte) (evictScan, error) {
	// Read the batch in one snapshot, at the limit in force.
	scan := evictScan{bounds: loadEvictBatch()}
	scan.limit = scan.bounds.clamp(s.evictLimit)
	err := retryBusy(ctx, func() error {
		return kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
			// Walk the older entries up to the batch and byte limits; the walk is
			// done when it runs out of them.
			scan.batch, scan.marked, scan.done = scan.batch[:0], scan.marked[:0], true
			var batchBytes int64
			scan.next = nil
			err := scanEvictable(ctx, tx, from, func(entry evictEntry) error {
				// Stop at the new epoch or a full batch, where the next batch
				// starts. A batch takes its first entry whatever its size.
				full := len(scan.batch) == scan.limit || (len(scan.batch) != 0 && batchBytes+entry.size > scan.bounds.MaxBytes)
				if entry.epoch >= epoch || full || evicted+batchBytes >= maxBytes {
					scan.done, scan.next = entry.epoch >= epoch, entry.key
					return errScanDone
				}

				// Take the entry with whether a marker awaits its upload.
				found, err := tx.Exists(ctx, markedKey(entry.hash))
				if err != nil {
					return err
				}
				scan.batch, scan.marked = append(scan.batch, entry), append(scan.marked, found)
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
		return evictScan{}, errors.Wrap(err, "scan eviction queue")
	}
	if scan.done {
		scan.next = evictEpochKey(epoch)
	}
	return scan, nil
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
	retryRefused bool,
	from []byte,
	result *EvictResult,
) ([]byte, bool, error) {
	// Read the batch with the markers of its blocks.
	scan, err := s.scanEvictBatch(ctx, epoch, maxBytes, result.EvictedBytes, from)
	if err != nil {
		return nil, false, err
	}
	batch, marked, next, done := scan.batch, scan.marked, scan.next, scan.done
	bounds, limit := scan.bounds, scan.limit
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
	var kept int
	for i, entry := range batch {
		switch {
		case !held[i]:
		case marked[i] || (stored != nil && !stored[i]):
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
		err := retryBusy(ctx, func() error {
			start := time.Now()
			_, err := s.values.PutBlockBatch(ctx, drop)
			took = time.Since(start)
			return err
		})
		if err != nil {
			return nil, false, errors.Wrap(err, "evict blocks")
		}
		s.evictLimit = nextEvictLimit(bounds, limit, len(drop), took)
		result.Evicted += len(drop)
	}

	// Delete the walked entries and queue the kept ones at the new epoch,
	// reading without the writer lock. With refusal retry off a refused
	// block's next upload queues it again. Retry adds a lookup and insertion
	// per refusal, so it bounds the writer hold with smaller metadata
	// transactions, while retry off keeps the one transaction a batch. Each
	// move and its byte change stay atomic.
	commit := len(batch)
	if retryRefused {
		commit = evictRetryCommitBlocks
	}
	for first := 0; first < len(batch); first += commit {
		// Partition the retained entries in the same key order as the scan.
		end := min(first+commit, len(batch))
		n := 0
		for n < len(requeue) && bytes.Compare(requeue[n].key, batch[end-1].key) <= 0 {
			n++
		}
		retained := requeue[:n]
		var delta int64
		err = retryBusy(ctx, func() error {
			return kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
				delta = 0
				for _, entry := range batch[first:end] {
					if err := tx.Delete(ctx, entry.key); err != nil {
						return err
					}
					delta -= entry.size
				}
				for _, entry := range retained {
					added, err := queueEvict(ctx, tx, epoch, entry.hash, entry.size)
					if err != nil {
						return err
					}
					delta += added
				}
				return addEvictableBytes(ctx, tx, delta)
			}, kvtx.RetryInvalidSnapshot)
		})
		if err != nil {
			return nil, false, errors.Wrap(err, "update eviction queue")
		}

		// Project each committed slice; drop-byte reporting keeps the batch
		// boundary, after all metadata for this scan batch commits.
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
