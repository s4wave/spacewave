package block_store_s3

import (
	"bytes"
	"context"
	"math"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// tailQueueBytes bounds the index tail bytes awaiting the drain. A tail past
// it is dropped: its blocks' h rows keep only their pack, and the pack's next
// cold read reads its tail from the bucket again.
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

// tailQueue holds index tails awaiting their write, one per pack. The fields
// are guarded by packIndex.mtx.
type tailQueue struct {
	// pending maps a pack id to its tail.
	pending map[string][]byte
	// bytes is the summed size of pending.
	bytes int
	// draining records that a drain goroutine runs.
	draining bool
}

// queueTail queues the index tail of a pack for its write and starts the
// drain when none runs. A pack already queued keeps its first tail, which
// is the same bytes.
func (x *packIndex) queueTail(packID string, data []byte) {
	// Skip a queued pack, and drop a tail past the byte bound.
	x.mtx.Lock()
	q := &x.tails
	if _, ok := q.pending[packID]; ok || q.bytes+len(data) > tailQueueBytes {
		x.mtx.Unlock()
		return
	}

	// Queue it, starting the drain this tail found idle.
	if q.pending == nil {
		q.pending = make(map[string][]byte)
	}
	q.pending[packID] = data
	q.bytes += len(data)
	drain := !q.draining
	q.draining = true
	x.mtx.Unlock()
	if drain {
		go x.drainTails()
	}
}

// queuedTail returns the tail of a pack awaiting its write.
func (x *packIndex) queuedTail(packID string) ([]byte, bool) {
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	data, ok := x.tails.pending[packID]
	return data, ok
}

// forgetQueuedTailsLocked drops the queued tails of removed packs. The caller
// holds mtx.
func (x *packIndex) forgetQueuedTailsLocked(ids []string) {
	q := &x.tails
	for _, id := range ids {
		if data, ok := q.pending[id]; ok {
			q.bytes -= len(data)
			delete(q.pending, id)
		}
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

// drainTails writes the extents the queued tails hold into their blocks' h
// rows, a step of up to tailBatchRows tails at a time paced by tailStepPause,
// until none are left. A row is upgraded only while it names the tail's pack
// and holds no extent, so a row a later pack took keeps naming that pack.
//
// A step reads its rows in a read transaction, which takes no writer turn, and
// then writes them in transactions that each hold the turn for at most
// tailTurnBudget or indexScanBatchKeys rows. A write rereads each row, which
// the read left in the page cache, and writes it only while it holds the value
// read, so a row another writer changed meanwhile keeps that writer's value.
//
// The tails themselves are not stored: a row with its extent reads its block
// without the tail. Each row is derived from bytes the bucket holds, so the
// commit is ordered rather than flushed, and a failed step drops its tails.
func (x *packIndex) drainTails() {
	ctx := kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(context.Background()))
	for {
		// Take the next step with each tail's pack number, or end the drain on
		// an empty queue. A tail stays queued, and readable, until its step is
		// written.
		x.mtx.Lock()
		q := &x.tails
		if len(q.pending) == 0 {
			q.draining = false
			x.mtx.Unlock()
			return
		}
		n := min(len(q.pending), tailBatchRows)
		step := make(map[string][]byte, n)
		nums := make(map[string]uint32, n)
		for id, data := range q.pending {
			if len(step) == tailBatchRows {
				break
			}
			step[id] = data
			if row, ok := x.rows[id]; ok {
				nums[id] = row.num
			}
		}
		x.mtx.Unlock()

		// Read the rows the step upgrades, then write them in short turns.
		started := time.Now()
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

		// Unqueue the step, written or dropped, then pause before the next.
		x.mtx.Lock()
		x.forgetQueuedTailsLocked(mapKeys(step))
		x.mtx.Unlock()
		time.Sleep(min(max(tailStepPause, 2*time.Since(started)), tailStepPauseMax))
	}
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
