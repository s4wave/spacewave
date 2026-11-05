package block_store_fillcache

import (
	"github.com/s4wave/spacewave/db/block"
)

// fillBatchBlocks bounds the entries one write transaction takes, so a read
// that queues behind the transaction waits for a bounded write.
const fillBatchBlocks = 256

// fillBatchBytes bounds the block bytes one write transaction takes.
const fillBatchBytes = 8 << 20

// fillQueue holds the entries no write transaction has taken. It does no
// locking: the Cache guards it with its broadcast.
type fillQueue struct {
	// pending are the queued entries, oldest first.
	pending []*block.PutBatchEntry
	// bytes is the summed block size of pending.
	bytes int64
	// limit bounds bytes.
	limit int64
	// queued holds the hash of each block pending or being written, so a block
	// read again before its write lands is not queued twice.
	queued map[string]struct{}
}

// newFillQueue builds a queue holding at most limit block bytes.
func newFillQueue(limit int64) *fillQueue {
	return &fillQueue{limit: limit, queued: make(map[string]struct{})}
}

// entryKey keys an entry's block in queued.
func entryKey(entry *block.PutBatchEntry) string {
	return string(entry.Ref.GetHash().GetHash())
}

// push queues the entry and reports whether it did. It queues nothing when the
// block is already queued, or when the entry would pass the byte bound, which
// it reports as full. A dropped entry loses nothing: the next read of the block
// queues it again.
func (q *fillQueue) push(entry *block.PutBatchEntry) (queued, full bool) {
	// Skip a block already queued, and refuse one past the byte bound.
	key := entryKey(entry)
	if _, ok := q.queued[key]; ok {
		return false, false
	}
	if q.bytes+int64(len(entry.Data)) > q.limit {
		return false, true
	}

	// Queue the entry and count its bytes.
	q.queued[key] = struct{}{}
	q.pending = append(q.pending, entry)
	q.bytes += int64(len(entry.Data))
	return true, false
}

// take removes the next write transaction's entries from the queue. The blocks
// stay in queued until done releases them.
func (q *fillQueue) take() []*block.PutBatchEntry {
	// Count the leading entries that fit one batch.
	n, size := 0, 0
	for n < len(q.pending) && n < fillBatchBlocks {
		if n != 0 && size+len(q.pending[n].Data) > fillBatchBytes {
			break
		}
		size += len(q.pending[n].Data)
		n++
	}

	// Cut them off the queue.
	batch := q.pending[:n:n]
	q.pending = q.pending[n:]
	q.bytes -= int64(size)
	return batch
}

// done releases the blocks of a written batch for queueing again.
func (q *fillQueue) done(batch []*block.PutBatchEntry) {
	for _, entry := range batch {
		delete(q.queued, entryKey(entry))
	}
}

// idle reports that nothing is pending or being written.
func (q *fillQueue) idle() bool {
	return len(q.queued) == 0
}
