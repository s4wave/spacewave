package block_store_s3

import (
	"bytes"
	"context"
	"math"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// tailQueueBytes bounds the index tail bytes awaiting the drain. A tail past
// it is dropped: it is not stored, its blocks' h rows keep only their pack, and
// the pack's next cold read reads its tail from the bucket again. A stored tail
// leaves the queue at its row's write, so the bound counts only tails not yet
// stored.
const tailQueueBytes = 64 << 20

// tailBatchRows bounds the tails one drain step upgrades in a transaction.
// Each tail rewrites the rows of its pack's blocks, scattered over the index,
// so a small step keeps each transaction's writes and flush short.
const tailBatchRows = 16

// tailStepPause is the least pause after a drain step. The pause is twice
// the step's own time when that is longer, up to tailStepPauseMax, so the
// drain slows as the disk it shares with player writes slows, and players
// never queue behind it.
const tailStepPause = 100 * time.Millisecond

// tailStepPauseMax bounds the pause after a slow step, so a step that waited
// long for the index's writer does not stall the drain as long again.
const tailStepPauseMax = time.Second

// tailRowsOn turns on storing each tail a pack reader loads from the bucket in
// its t/ row; see SetIndexTailRows.
var tailRowsOn atomic.Bool

func init() {
	tailRowsOn.Store(true)
}

// SetIndexTailRows turns storing loaded index tails on or off for every
// PackStore. On, the drain writes each tail a pack reader loaded from the
// bucket to the pack's t/ row before it upgrades the pack's h rows, so the
// pack's next load after a restart, or after its index was reclaimed, reads
// the row and not the bucket. Off, as before, tails are queued for the h row
// upgrade alone and a dropped or drained tail is read from the bucket again.
// Rows already stored are read either way.
//
// Invariant: a stored tail is the exact bytes of an immutable pack that
// durable storage holds. Only Set, which the pack reader calls with the tail
// it read from the bucket, stores one; the row is written only while the
// pack's entry row exists, and removeRows deletes it with the pack. A pack id
// is never reused, so a row never describes other bytes. The reader still
// validates a stored tail like a fetched one, against the pack's size and
// block count, and on failure reads the bucket and stores that tail over it.
func SetIndexTailRows(on bool) {
	tailRowsOn.Store(on)
}

// queuedTail is an index tail awaiting the drain.
type queuedTail struct {
	// data is the tail.
	data []byte
	// store records that the tail came from the bucket and is to be stored in
	// its t/ row; a writer's or the indexer's tail is not.
	store bool
}

// tailQueue holds index tails awaiting their write, one per pack. The fields
// are guarded by packIndex.mtx.
type tailQueue struct {
	// pending maps a pack id to its tail.
	pending map[string]queuedTail
	// stored holds the packs whose tail the drain stored in its t/ row and
	// whose h rows it has not yet upgraded; their tails are read back from
	// the rows.
	stored map[string]struct{}
	// bytes is the summed size of pending.
	bytes int
	// draining records that a drain goroutine runs.
	draining bool
}

// queueTail queues the index tail of a pack for its write and starts the
// drain when none runs. A pack already queued keeps its first tail, which
// is the same bytes. store asks the drain to keep the tail in its t/ row.
func (x *packIndex) queueTail(packID string, data []byte, store bool) {
	// Skip a queued pack, and drop a tail past the byte bound.
	x.mtx.Lock()
	q := &x.tails
	_, queued := q.pending[packID]
	_, stored := q.stored[packID]
	if queued || stored || q.bytes+len(data) > tailQueueBytes {
		x.mtx.Unlock()
		return
	}

	// Queue it, starting the drain this tail found idle.
	if q.pending == nil {
		q.pending = make(map[string]queuedTail)
	}
	q.pending[packID] = queuedTail{data: data, store: store}
	q.bytes += len(data)
	drain := !q.draining
	q.draining = true
	x.mtx.Unlock()
	if drain {
		go x.drainTails()
	}
}

// queuedTailData returns the tail of a pack awaiting its write.
func (x *packIndex) queuedTailData(packID string) ([]byte, bool) {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	t, ok := x.tails.pending[packID]
	return t.data, ok
}

// forgetQueuedTailsLocked drops the queued tails of removed packs. The caller
// holds mtx.
func (x *packIndex) forgetQueuedTailsLocked(ids []string) {
	q := &x.tails
	for _, id := range ids {
		if t, ok := q.pending[id]; ok {
			q.bytes -= len(t.data)
			delete(q.pending, id)
		}
		delete(q.stored, id)
	}
}

// tailTurnBudget bounds the time one drain transaction holds the index's
// writer turn. A foreground writer that queued behind it waits at most this
// long, plus the commit.
const tailTurnBudget = 50 * time.Millisecond

// tailUpgrade is an h row a drain step rewrites with its extent.
type tailUpgrade struct {
	// key is the row key.
	key []byte
	// old is the row value read before the writer turn.
	old []byte
	// value is the row value with the extent.
	value []byte
}

// drainTails stores the queued bucket tails in their t/ rows and writes the
// extents the queued tails hold into their blocks' h rows, a step of up to
// tailBatchRows tails at a time paced by tailStepPause, until none are left.
// Storing goes first, since it frees the queue's memory and saves a restart
// the tails' bucket reads; the upgrade then reads each stored tail back from
// its row. A row is upgraded only while it names the tail's pack and holds no
// extent, so a row a later pack took keeps naming that pack.
//
// An upgrade step reads its rows in a read transaction, which takes no writer
// turn, and then writes them in transactions that each hold the turn for at
// most tailTurnBudget or indexScanBatchKeys rows. A write rereads each row,
// which the read left in the page cache, and writes it only while it holds the
// value read, so a row another writer changed meanwhile keeps that writer's
// value.
//
// Each row is derived from bytes the bucket holds, so every commit is ordered
// rather than flushed, and a failed step drops its work: a tail not stored is
// read from the bucket again, and a row not upgraded reads through its pack.
func (x *packIndex) drainTails() {
	ctx := kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(context.Background()))
	for {
		started := time.Now()
		if !x.storeTailStep(ctx) && !x.upgradeTailStep(ctx) {
			return
		}
		time.Sleep(min(max(tailStepPause, 2*time.Since(started)), tailStepPauseMax))
	}
}

// storeTailStep writes up to tailBatchRows queued bucket tails to their t/
// rows in one transaction and moves them from the queue to the stored set. It
// reports false when tail rows are off or no tail awaits storing. A failed
// write leaves the tails queued for the upgrade alone.
func (x *packIndex) storeTailStep(ctx context.Context) bool {
	// Leave every tail to the upgrade while tail rows are off.
	if !tailRowsOn.Load() {
		return false
	}

	// Take the step.
	x.mtx.Lock()
	step := make(map[string][]byte, tailBatchRows)
	for id, t := range x.tails.pending {
		if len(step) == tailBatchRows {
			break
		}
		if t.store {
			step[id] = t.data
		}
	}
	x.mtx.Unlock()
	if len(step) == 0 {
		return false
	}

	// Write each tail while its pack's entry row exists, so a pack removed
	// meanwhile leaves no row, holding the writer turn for at most
	// tailTurnBudget; a tail past it stays queued for the next step.
	written := make(map[string]struct{}, len(step))
	err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return x.store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		clear(written)
		held := time.Now()
		for id, data := range step {
			if len(written) != 0 && time.Since(held) >= tailTurnBudget {
				return nil
			}
			written[id] = struct{}{}
			_, found, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
			if err != nil {
				return err
			}
			if found {
				if err := tx.Set(ctx, []byte(indexTailPrefix+id), data); err != nil {
					return err
				}
			}
		}
		return nil
	})

	// Move a stored tail of a pack still queued to the stored set, whose
	// upgrade reads it from its row; on failure, leave the step to the
	// upgrade alone.
	x.mtx.Lock()
	q := &x.tails
	for id := range written {
		t, ok := q.pending[id]
		if !ok {
			continue
		}
		if err != nil {
			t.store = false
			q.pending[id] = t
			continue
		}
		q.bytes -= len(t.data)
		delete(q.pending, id)
		if q.stored == nil {
			q.stored = make(map[string]struct{})
		}
		q.stored[id] = struct{}{}
	}
	x.mtx.Unlock()
	return true
}

// upgradeTailStep writes the extents of up to tailBatchRows tails into their
// blocks' h rows and unqueues them, written or dropped. It reports false, and
// ends the drain, when no tail is left.
func (x *packIndex) upgradeTailStep(ctx context.Context) bool {
	// End the drain on an empty queue.
	x.mtx.Lock()
	q := &x.tails
	if len(q.pending) == 0 && len(q.stored) == 0 {
		q.draining = false
		x.mtx.Unlock()
		return false
	}

	// Take the next step with each tail's pack number. A tail stays readable,
	// queued or in its row, until its step is written; a stored tail's data
	// is nil and is read from its row.
	step := make(map[string][]byte, tailBatchRows)
	nums := make(map[string]uint32, tailBatchRows)
	take := func(id string, data []byte) {
		step[id] = data
		if row, ok := x.rows[id]; ok {
			nums[id] = row.num
		}
	}
	for id, t := range q.pending {
		if len(step) == tailBatchRows {
			break
		}
		take(id, t.data)
	}
	for id := range q.stored {
		if len(step) == tailBatchRows {
			break
		}
		take(id, nil)
	}
	x.mtx.Unlock()

	// Read the rows the step upgrades, then write them in short turns.
	var plan []tailUpgrade
	err := kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return x.store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		plan = plan[:0]
		for id, data := range step {
			num, ok := nums[id]
			if !ok {
				continue
			}
			if data == nil {
				var found bool
				var err error
				data, found, err = tx.Get(ctx, []byte(indexTailPrefix+id))
				if err != nil {
					return err
				}
				if !found {
					continue
				}
			}
			var err error
			if plan, err = planTailUpgrades(ctx, tx, id, num, data, plan); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		_ = x.writeTailUpgrades(ctx, plan)
	}

	// Unqueue the step, written or dropped.
	x.mtx.Lock()
	x.forgetQueuedTailsLocked(mapKeys(step))
	x.mtx.Unlock()
	return true
}

// writeTailUpgrades writes plan in transactions that each hold the writer
// turn for at most tailTurnBudget or indexScanBatchKeys rows, writing a row
// only while it still holds the value the plan read.
func (x *packIndex) writeTailUpgrades(ctx context.Context, plan []tailUpgrade) error {
	for len(plan) != 0 {
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return x.store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			held := time.Now()
			for i, u := range plan {
				if i == indexScanBatchKeys || (i != 0 && time.Since(held) >= tailTurnBudget) {
					plan = plan[i:]
					return nil
				}
				value, ok, err := tx.Get(ctx, u.key)
				if err != nil {
					return err
				}
				if ok && bytes.Equal(value, u.old) {
					if err := tx.Set(ctx, u.key, u.value); err != nil {
						return err
					}
				}
			}
			plan = nil
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// planTailUpgrades appends to plan the h row of each block the tail of pack id
// lists that names the pack as num and holds no extent, with its extent. The
// pack's size, which the tail's offsets need, comes from its entry row.
func planTailUpgrades(ctx context.Context, tx kvtx.Tx, id string, num uint32, tail []byte, plan []tailUpgrade) ([]tailUpgrade, error) {
	// Read the pack's size.
	data, found, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
	if err != nil || !found {
		return plan, err
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil {
		return plan, nil
	}

	// Plan each row that names this pack without an extent.
	err = packfile.ScanIndexTail(tail, entry.GetSizeBytes(), func(ie *kvfile.IndexEntry) error {
		// Skip an extent a row cannot hold.
		if ie.GetSize() > math.MaxUint32 {
			return nil
		}

		// Read the row, leaving one that names another pack or has its extent.
		rowKey := hashRowKey(ie.GetKey())
		value, ok, err := tx.Get(ctx, rowKey)
		if err != nil || !ok {
			return err
		}
		row, ok := parseHashValue(value)
		if !ok || row.num != num || row.extent {
			return nil
		}

		// Plan the extent's write.
		row.off, row.size, row.extent = ie.GetOffset(), uint32(ie.GetSize()), true //nolint:gosec // checked against MaxUint32 above.
		plan = append(plan, tailUpgrade{key: rowKey, old: bytes.Clone(value), value: row.marshal()})
		return nil
	})
	return plan, err
}

// mapKeys returns the keys of m.
func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
