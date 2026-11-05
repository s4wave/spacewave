package block_store_s3

import (
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

// drainTails writes the extents the queued tails hold into their blocks' h
// rows, in transactions of up to tailBatchRows tails paced by tailStepPause,
// until none are left. A
// row is upgraded only while it names the tail's pack and holds no extent, so
// a row a later pack took keeps naming that pack. The tails themselves are
// not stored: a row with its extent reads its block without the tail. Each
// row is derived from bytes the bucket holds, so the commit is ordered rather
// than flushed, and a failed step drops its tails.
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

		// Upgrade the step's rows in one transaction.
		started := time.Now()
		_ = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return x.store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			for id, data := range step {
				num, ok := nums[id]
				if !ok {
					continue
				}
				if err := upgradeHashRows(ctx, tx, id, num, data); err != nil {
					return err
				}
			}
			return nil
		})

		// Unqueue the step, written or dropped, then pause before the next.
		x.mtx.Lock()
		x.forgetQueuedTailsLocked(mapKeys(step))
		x.mtx.Unlock()
		time.Sleep(min(max(tailStepPause, 2*time.Since(started)), tailStepPauseMax))
	}
}

// upgradeHashRows writes the extent of each block the tail of pack id lists
// into its h row, where the row names the pack as num and holds no extent.
// The pack's size, which the tail's offsets need, comes from its entry row.
func upgradeHashRows(ctx context.Context, tx kvtx.Tx, id string, num uint32, tail []byte) error {
	// Read the pack's size.
	data, found, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
	if err != nil || !found {
		return err
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil {
		return nil
	}

	// Upgrade each row that names this pack without an extent.
	return packfile.ScanIndexTail(tail, entry.GetSizeBytes(), func(ie *kvfile.IndexEntry) error {
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

		// Write the extent into the row.
		row.off, row.size, row.extent = ie.GetOffset(), uint32(ie.GetSize()), true //nolint:gosec // checked against MaxUint32 above.
		return tx.Set(ctx, rowKey, row.marshal())
	})
}

// mapKeys returns the keys of m.
func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
