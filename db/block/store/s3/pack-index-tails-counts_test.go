package block_store_s3

import (
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

// tailCountsSince returns the tail counts gained since before.
func tailCountsSince(before IndexTailCounts) IndexTailCounts {
	now := ReadIndexTailCounts()
	return IndexTailCounts{
		Queued:       now.Queued - before.Queued,
		Dropped:      now.Dropped - before.Dropped,
		Held:         now.Held - before.Held,
		Drained:      now.Drained - before.Drained,
		RowsUpgraded: now.RowsUpgraded - before.RowsUpgraded,
	}
}

// TestIndexTailCounts counts a tail through its drain, holds the next one
// out without touching its rows, queues it again once the hold lifts, and
// counts a tail past the queue's bound as dropped.
func TestIndexTailCounts(t *testing.T) {
	SetIndexExtentWrites(false)
	t.Cleanup(func() { SetIndexExtentWrites(true) })
	t.Cleanup(func() { SetIndexTailsHeld(false) })
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := &countingKV{Store: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	s := newIndexedStore(t, client, kv)

	// A written pack queues its tail, which drains and upgrades its rows.
	before := ReadIndexTailCounts()
	adoptBlocks(ctx, t, s, "own", 5)
	waitFor(t, "the tail drains", func() bool { return tailsIdle(s) && extentRows(t, kv) == 5 })
	if got := tailCountsSince(before); got != (IndexTailCounts{Queued: 1, Drained: 1, RowsUpgraded: 5}) {
		t.Fatalf("counts after one drain = %+v", got)
	}

	// Held, the next pack's tail is counted and left out, and no row of it
	// is written twice.
	SetIndexTailsHeld(true)
	before = ReadIndexTailCounts()
	sets := kv.hashSets.Load()
	refs := adoptBlocks(ctx, t, s, "held", 3)
	if !tailsIdle(s) {
		t.Fatal("a tail was queued while tails are held")
	}
	if got := tailCountsSince(before); got != (IndexTailCounts{Held: 1}) {
		t.Fatalf("counts while held = %+v", got)
	}
	if got := kv.hashSets.Load() - sets; got != int64(len(refs)) {
		t.Fatalf("h row writes while held = %d, want one per block, %d", got, len(refs))
	}
	if got := extentRows(t, kv); got != 5 {
		t.Fatalf("extent rows while held = %d, want the first pack's 5", got)
	}

	// Lifted, the held pack's tail queues again on its next read and drains.
	SetIndexTailsHeld(false)
	entry := entryOf(ctx, t, s, 3)
	before = ReadIndexTailCounts()
	if err := s.index.Set(ctx, entry.GetId(), tailOf(ctx, t, client, entry)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the lifted tail drains", func() bool { return tailsIdle(s) && extentRows(t, kv) == 8 })
	if got := tailCountsSince(before); got != (IndexTailCounts{Queued: 1, Drained: 1, RowsUpgraded: 3}) {
		t.Fatalf("counts after the hold lifts = %+v", got)
	}
	checkExtents(ctx, t, s, client, entry.GetId(), refs)

	// A tail past the queue's bound is dropped and counted.
	before = ReadIndexTailCounts()
	s.index.queueTail("too-large", make([]byte, tailQueueBytes+1))
	if got := tailCountsSince(before); got != (IndexTailCounts{Dropped: 1}) {
		t.Fatalf("counts after an oversize tail = %+v", got)
	}
}
