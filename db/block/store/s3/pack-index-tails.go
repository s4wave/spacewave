package block_store_s3

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// tailQueueBytes bounds the index tail bytes awaiting their write. A tail
// past it is dropped: the row is a cache, and the pack's next cold read
// reads its tail from the bucket again.
const tailQueueBytes = 64 << 20

// tailBatchRows bounds the tails one drain step writes in a transaction.
const tailBatchRows = 256

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

// drainTails writes the queued tails in transactions of up to tailBatchRows
// until none are left. Each row is a cache of bytes the bucket holds, so its
// commit is ordered rather than flushed, and a failed step drops its tails.
func (x *packIndex) drainTails() {
	ctx := kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(context.Background()))
	for {
		// Take the next step, or end the drain on an empty queue. A tail
		// stays queued, and readable, until its step is written.
		x.mtx.Lock()
		q := &x.tails
		if len(q.pending) == 0 {
			q.draining = false
			x.mtx.Unlock()
			return
		}
		step := make(map[string][]byte, min(len(q.pending), tailBatchRows))
		for id, data := range q.pending {
			if len(step) == tailBatchRows {
				break
			}
			step[id] = data
		}
		x.mtx.Unlock()

		// Write the step in one transaction.
		_ = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return x.store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			for id, data := range step {
				if err := tx.Set(ctx, []byte(indexTailPrefix+id), data); err != nil {
					return err
				}
			}
			return nil
		})

		// Unqueue the step, written or dropped.
		x.mtx.Lock()
		x.forgetQueuedTailsLocked(mapKeys(step))
		x.mtx.Unlock()
	}
}

// mapKeys returns the keys of m.
func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
