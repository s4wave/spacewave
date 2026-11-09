package block_store_writeback

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// slowWrites delays each read in a write transaction on the markers while
// armed, standing in for a disk under load, and records the longest time one
// held the writer.
type slowWrites struct {
	kvtx.Store
	armed   atomic.Bool
	writes  atomic.Int32
	longest atomic.Int64
}

// NewTransaction slows the reads of a write transaction while armed.
func (w *slowWrites) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := w.Store.NewTransaction(ctx, write)
	if err != nil || !write || !w.armed.Load() {
		return tx, err
	}
	w.writes.Add(1)
	return &slowReadTx{Tx: tx, w: w, start: time.Now()}, nil
}

// slowReadTx delays each Get and records its hold when it ends.
type slowReadTx struct {
	kvtx.Tx
	w     *slowWrites
	start time.Time
	once  sync.Once
}

// Get reads the key after a delay.
func (t *slowReadTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	time.Sleep(time.Millisecond)
	return t.Tx.Get(ctx, key)
}

// Commit commits and records the hold.
func (t *slowReadTx) Commit(ctx context.Context) error {
	t.ended()
	return t.Tx.Commit(ctx)
}

// Discard discards and records the hold.
func (t *slowReadTx) Discard() {
	t.ended()
	t.Tx.Discard()
}

// ended records how long the transaction held the writer.
func (t *slowReadTx) ended() {
	t.once.Do(func() {
		held := int64(time.Since(t.start))
		for {
			prev := t.w.longest.Load()
			if held <= prev || t.w.longest.CompareAndSwap(prev, held) {
				return
			}
		}
	})
}

// TestClearBoundsTheWriterHold checks that a clear of a large upload batch
// holds the store's writer in short transactions, each bounded in entries and
// in time, and leaves the queue and status as one transaction would.
func TestClearBoundsTheWriterHold(t *testing.T) {
	// Bound each clear transaction to 512 entries and a 40 ms hold.
	SetClearBatch(ClearBatch{Hold: 40 * time.Millisecond, MaxBlocks: 512})
	t.Cleanup(func() { SetClearBatch(ClearBatch{}) })

	// Queue more blocks than one clear transaction removes.
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	const blocks = 512 + 300
	for i := range blocks {
		if _, _, err := s.PutBlock(ctx, fmt.Appendf(nil, "clear-%05d", i), leafOpts); err != nil {
			t.Fatal(err)
		}
	}

	// Upload the whole queue as one batch.
	head, err := s.scanHead(ctx, batchLimitsOf(&sizedStore{limits: limitsFor(blocks)}), nil)
	if err != nil || len(head.entries) != blocks {
		t.Fatalf("scan = %d blocks, %v", len(head.entries), err)
	}
	remote := newInmemBlockStore()
	if err := s.uploadChunk(ctx, remote, head.entries); err != nil {
		t.Fatal(err)
	}

	// Clear it on a slow disk: every read under the writer takes a
	// millisecond, so one transaction for the batch would hold the writer
	// for over a second.
	markers := &slowWrites{Store: s.markers}
	s.markers = markers
	markers.armed.Store(true)
	if err := s.clear(ctx, head.entries); err != nil {
		t.Fatal(err)
	}
	markers.armed.Store(false)

	// Each transaction let the writer go within a bounded time near the
	// hold, a small part of the 0.8 s one transaction for the batch would
	// have held it, and the 512-entry bound split the batch.
	if longest := time.Duration(markers.longest.Load()); longest > 150*time.Millisecond {
		t.Fatalf("a clear transaction held the writer %v over %d transactions", longest, markers.writes.Load())
	}
	if writes := markers.writes.Load(); writes < blocks/60 {
		t.Fatalf("the clear ran %d transactions, want at least %d at 40 ms of 1 ms reads", writes, blocks/60)
	}

	// The queue and the status read as cleared.
	if n, size := countQueue(t, s.markers); n != 0 || size != 0 {
		t.Fatalf("queue holds %d entries of %d bytes, want none", n, size)
	}
	status, _ := s.GetStatus()
	if status.Pending != 0 || status.PendingBytes != 0 || status.Uploaded != blocks || !status.Oldest.IsZero() {
		t.Fatalf("status pending %d (%d bytes), uploaded %d, oldest %v; want 0, 0, %d, zero", status.Pending, status.PendingBytes, status.Uploaded, status.Oldest, blocks)
	}
}

// limitsFor returns batch limits that hold the given number of small blocks.
func limitsFor(blocks int) block.BatchLimits {
	return block.BatchLimits{Bytes: int64(blocks) << 10, Blocks: blocks, Workers: 1}
}

// TestSetClearBatchBounds checks that SetClearBatch keeps the default of each
// zero or negative field and lowers a field past its most.
func TestSetClearBatchBounds(t *testing.T) {
	t.Cleanup(func() { SetClearBatch(ClearBatch{}) })
	cases := []struct {
		set, want ClearBatch
	}{
		{ClearBatch{}, ClearBatch{Hold: clearHoldDefault, MaxBlocks: clearBlocksDefault}},
		{ClearBatch{Hold: -time.Second, MaxBlocks: -1}, ClearBatch{Hold: clearHoldDefault, MaxBlocks: clearBlocksDefault}},
		{ClearBatch{Hold: 150 * time.Millisecond, MaxBlocks: 2048}, ClearBatch{Hold: 150 * time.Millisecond, MaxBlocks: 2048}},
		{ClearBatch{Hold: 5 * time.Second, MaxBlocks: 1 << 20}, ClearBatch{Hold: clearHoldMax, MaxBlocks: clearBlocksMax}},
	}
	for _, c := range cases {
		SetClearBatch(c.set)
		if got := loadClearBatch(); got != c.want {
			t.Fatalf("SetClearBatch(%+v) applies %+v, want %+v", c.set, got, c.want)
		}
	}
}
