package block_store_writeback

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// countKey holds the upload queue's kept count: the entries it counts, their
// summed size, and the walk that starts it on a store that never kept one.
//
// A walk starts with its end just past the queue's last key, so every entry
// it starts with sorts below it. A queue key is covered once the walk is
// done, or while it runs when the key is at or below its cursor, the last key
// it counted, or at or past its end, as the writes since it started are.
// Every transaction that inserts or deletes a queue entry adjusts the count
// for a covered key alone, in the same transaction, and each walk slice
// counts the uncovered entries just past the cursor and moves the cursor over
// them, in one transaction, until it reaches the end. So the walk's length is
// the backlog it started with, whatever is written meanwhile, and an entry a
// clock stepped back sorts below the end and is counted by the walk.
//
// Invariant: in every committed state, the count plus the queue's uncovered
// entries is the queue's size, so once the walk is done the count is the
// queue's size. It holds through crashes, restarts and discarded
// transactions, since the count changes only with the entries it counts.
// QueueCount.tla checks it, with mutants that adjust the count for a key past
// the cursor, persist the cursor apart from its slice, and end the walk below
// an entry it started with.
//
// The open that starts or resumes a walk writes its owner, and a slice counts
// only for the walk's owner, so a slice of a store whose mount was replaced
// counts nothing into a count its successor read.
//
// A writer that changes the queue without the count, such as a build of this
// package from before it after a rollback, or a tool that writes MarkerKeys
// into an opened store, leaves the count off; SetRecountAtOpen resets it, and
// a clear that empties the queue resets it to zero.
const countKey = "queue-count"

// recountAtOpen makes an open drop the kept count and walk the queue again.
var recountAtOpen atomic.Bool

// SetRecountAtOpen makes stores opened while on is set drop the counts they
// keep of their upload queues and count them again in the background: the
// runtime switch for a count that a writer without it left off.
func SetRecountAtOpen(on bool) {
	recountAtOpen.Store(on)
}

// readOnlyOpen makes an open read its markers without writing them.
var readOnlyOpen atomic.Bool

// SetReadOnlyOpen makes stores opened while on is set read their kept state in
// a read transaction: they drop no former marker layout, take no ownership of
// the queue count, and start no walk of the queue, so the open writes nothing.
// It is for a store that only serves reads and never uploads.
func SetReadOnlyOpen(on bool) {
	readOnlyOpen.Store(on)
}

// queueCount is the kept count of the upload queue.
type queueCount struct {
	// n is the number of covered queue entries.
	n int
	// bytes is their summed size.
	bytes int64
	// done reports that the walk reached the queue's end, covering every key.
	done bool
	// cursor is the last key the walk counted while it runs, nil before the
	// first.
	cursor []byte
	// end is the first key past the queue's entries when the walk started,
	// while it runs.
	end []byte
	// owner names the open that runs the walk, while it runs.
	owner uint64
}

// covers reports whether the count holds the queue entry at key.
func (c queueCount) covers(key []byte) bool {
	return c.done ||
		c.cursor != nil && bytes.Compare(key, c.cursor) <= 0 ||
		bytes.Compare(key, c.end) >= 0
}

// countDelta is a change of the kept count, which the status follows once the
// transaction that made it commits.
type countDelta struct {
	n     int
	bytes int64
}

// add adds an entry of size bytes to the count when it covers key, and
// returns the change.
func (c *queueCount) add(key []byte, size int64) countDelta {
	if !c.covers(key) {
		return countDelta{}
	}
	c.n++
	c.bytes += size
	return countDelta{n: 1, bytes: size}
}

// remove takes an entry of size bytes off the count when it covers key, and
// returns the change.
func (c *queueCount) remove(key []byte, size int64) countDelta {
	if !c.covers(key) {
		return countDelta{}
	}
	c.n--
	c.bytes -= size
	return countDelta{n: -1, bytes: -size}
}

// plus returns the sum of two changes.
func (d countDelta) plus(o countDelta) countDelta {
	return countDelta{n: d.n + o.n, bytes: d.bytes + o.bytes}
}

// encode returns the count's value: n and bytes in decimal and "done", or
// "walk" and the owner, then the cursor and the end on lines of their own.
// Queue keys hold no newline.
func (c queueCount) encode() []byte {
	// Write the numbers, and done for a walk that ended.
	out := strconv.AppendInt(nil, int64(c.n), 10)
	out = append(out, ' ')
	out = strconv.AppendInt(out, c.bytes, 10)
	if c.done {
		return append(out, " done"...)
	}

	// Write the walk's owner, cursor and end.
	out = append(out, " walk "...)
	out = strconv.AppendUint(out, c.owner, 10)
	out = append(out, '\n')
	out = append(out, c.cursor...)
	out = append(out, '\n')
	return append(out, c.end...)
}

// decodeQueueCount parses a value encode wrote, reporting false for one it
// cannot parse.
func decodeQueueCount(value []byte) (queueCount, bool) {
	// Split the numbers and state from the keys.
	head, keys, walking := bytes.Cut(value, []byte("\n"))
	fields := bytes.Fields(head)
	if len(fields) < 3 {
		return queueCount{}, false
	}
	n, err := strconv.Atoi(string(fields[0]))
	if err != nil {
		return queueCount{}, false
	}
	size, err := strconv.ParseInt(string(fields[1]), 10, 64)
	if err != nil {
		return queueCount{}, false
	}

	// Read the walk's state.
	c := queueCount{n: n, bytes: size}
	switch {
	case string(fields[2]) == "done" && len(fields) == 3:
		c.done = true
		return c, !walking
	case string(fields[2]) != "walk" || len(fields) != 4 || !walking:
		return queueCount{}, false
	}

	// Read the walk's owner, cursor and end.
	owner, err := strconv.ParseUint(string(fields[3]), 10, 64)
	cursor, end, ok := bytes.Cut(keys, []byte("\n"))
	if err != nil || !ok || len(end) == 0 {
		return queueCount{}, false
	}
	c.owner, c.end = owner, bytes.Clone(end)
	if len(cursor) != 0 {
		c.cursor = bytes.Clone(cursor)
	}
	return c, true
}

// readQueueCount reads the kept count, reporting false when it is absent or
// unreadable, which an open replaces with a new walk.
func readQueueCount(ctx context.Context, tx kvtx.Tx) (queueCount, bool, error) {
	value, found, err := tx.Get(ctx, []byte(countKey))
	if err != nil || !found {
		return queueCount{}, false, err
	}
	c, ok := decodeQueueCount(value)
	return c, ok, nil
}

// writeQueueCount writes the kept count.
func writeQueueCount(ctx context.Context, tx kvtx.Tx, c queueCount) error {
	return tx.Set(ctx, []byte(countKey), c.encode())
}

// queueLast returns the last queue key in tx, nil for an empty queue.
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
