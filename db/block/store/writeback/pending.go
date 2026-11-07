package block_store_writeback

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// pendingKey holds the queue's count: the number of queue entries, their
// summed size, and the first and last queue keys. Once established it is
// exact in every committed state, since every transaction that adds or
// removes a queue entry changes it in the same transaction, so an open reads
// it instead of counting the queue.
//
// Invariant: while pendingKey exists, it equals what a count of the queue
// in the same snapshot produces. queueMarks adds only the entries it creates,
// clear removes only the entries it deletes, and SetEnabled empties both. A
// writer that changes the queue without it, such as an older build of this
// package, leaves it stale; the first and last keys catch such a writer's
// appends and head clears at open, which then counts the queue again. A
// clear in the middle it misses leaves the count high, never low, so the
// upload never sleeps with entries queued, and a clear that empties the queue
// sets it to zero.
const pendingKey = "queue-pending"

// pendingDeltaKey holds the entries queued since an open that found no valid
// count, while the open's count runs. The count reads only entries written
// before the open, and nothing removes an entry until it ends, so the count
// plus this delta is the queue's count when it ends.
const pendingDeltaKey = "queue-pending-delta"

// recountAtOpen makes an open count the queue even when it holds a valid
// count, then keep the new one.
var recountAtOpen atomic.Bool

// SetRecountAtOpen makes stores opened after it count their upload queues
// again rather than trust the counts they keep, while on is set: the runtime
// switch back to counting at every open.
func SetRecountAtOpen(on bool) {
	recountAtOpen.Store(on)
}

// queueCount is the count of the upload queue.
type queueCount struct {
	// n is the number of queue entries.
	n int
	// bytes is their summed size.
	bytes int64
	// first and last are the first and last queue keys, nil when the queue is
	// empty or for a delta.
	first, last []byte
}

// encode returns the count's value: n and bytes in decimal, then the two keys,
// on lines of their own. Queue keys hold no newline.
func (c queueCount) encode() []byte {
	// Write the numbers, then each key on its line.
	out := strconv.AppendInt(nil, int64(c.n), 10)
	out = append(out, ' ')
	out = strconv.AppendInt(out, c.bytes, 10)
	out = append(out, '\n')
	out = append(out, c.first...)
	out = append(out, '\n')
	return append(out, c.last...)
}

// decodeQueueCount parses a value encode wrote.
func decodeQueueCount(value []byte) (queueCount, error) {
	// Split the numbers from the two keys.
	parts := bytes.SplitN(value, []byte("\n"), 3)
	if len(parts) != 3 {
		return queueCount{}, errors.New("malformed upload queue count")
	}
	nums := bytes.Fields(parts[0])
	if len(nums) != 2 {
		return queueCount{}, errors.New("malformed upload queue count")
	}

	// Parse the numbers and keep the keys.
	n, err := strconv.Atoi(string(nums[0]))
	if err != nil {
		return queueCount{}, errors.Wrap(err, "parse upload queue count")
	}
	size, err := strconv.ParseInt(string(nums[1]), 10, 64)
	if err != nil {
		return queueCount{}, errors.Wrap(err, "parse upload queue size")
	}
	c := queueCount{n: n, bytes: size}
	if len(parts[1]) != 0 {
		c.first = bytes.Clone(parts[1])
	}
	if len(parts[2]) != 0 {
		c.last = bytes.Clone(parts[2])
	}
	return c, nil
}

// readQueueCount reads the count at key, reporting false when it is absent. A
// value it cannot parse reads as absent too, so a write that keeps the count
// never fails on it and the next open counts the queue again.
func readQueueCount(ctx context.Context, tx kvtx.Tx, key string) (queueCount, bool, error) {
	value, found, err := tx.Get(ctx, []byte(key))
	if err != nil || !found {
		return queueCount{}, false, err
	}
	c, err := decodeQueueCount(value)
	return c, err == nil, nil
}

// writeQueueCount writes c at key.
func writeQueueCount(ctx context.Context, tx kvtx.Tx, key string, c queueCount) error {
	return tx.Set(ctx, []byte(key), c.encode())
}

// addQueued adds the entries a transaction created to the queue's count, or to
// the delta of an open's count while that runs. first and last are the least
// and greatest keys created. With neither key present it changes nothing.
func addQueued(ctx context.Context, tx kvtx.Tx, n int, size int64, first, last []byte) error {
	// Add to the count, widening its ends.
	c, found, err := readQueueCount(ctx, tx, pendingKey)
	if err != nil {
		return err
	}
	if found {
		c.n += n
		c.bytes += size
		if c.first == nil || bytes.Compare(first, c.first) < 0 {
			c.first = bytes.Clone(first)
		}
		if c.last == nil || bytes.Compare(last, c.last) > 0 {
			c.last = bytes.Clone(last)
		}
		return writeQueueCount(ctx, tx, pendingKey, c)
	}

	// Or to the delta while the open counts.
	d, found, err := readQueueCount(ctx, tx, pendingDeltaKey)
	if err != nil || !found {
		return err
	}
	d.n += n
	d.bytes += size
	return writeQueueCount(ctx, tx, pendingDeltaKey, d)
}

// queueEnds returns the first and last live queue keys in tx, nil for an empty
// queue. floor is at or before every live entry, or nil.
func queueEnds(ctx context.Context, tx kvtx.Tx, floor []byte) (first, last []byte, err error) {
	// Read the first entry from the floor.
	err = scanQueue(ctx, tx, floor, func(item queueItem) error {
		first = bytes.Clone(item.key)
		return errScanDone
	})
	if err != nil && !errors.Is(err, errScanDone) {
		return nil, nil, err
	}
	if first == nil {
		return nil, nil, nil
	}

	// Read the last entry from the end.
	last, err = queueLast(ctx, tx)
	return first, last, err
}

// queueLast returns the last live queue key in tx, nil for an empty queue.
func queueLast(ctx context.Context, tx kvtx.Tx) ([]byte, error) {
	// Seek to the end of the queue, newest first.
	it := tx.Iterate(ctx, []byte(queuePrefix), true, true)
	defer it.Close()
	if err := it.Seek(nil); err != nil {
		return nil, err
	}
	if !it.Valid() {
		return nil, it.Err()
	}
	return bytes.Clone(it.Key()), nil
}
