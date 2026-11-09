package block_store_writeback

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// TestEvictAdmitPacesBatches checks that an eviction pass asks its admit
// before each batch with the bytes the last one dropped, and that the admit's
// error ends the pass after the batches it let through. The next passes walk
// the rest over several batches, each resuming where the last stopped: every
// entry is evicted or kept exactly once, and a kept entry waits for the pass
// after. Reading every block back refills them in batched steps, and the next
// pass evicts them again.
func TestEvictAdmitPacesBatches(t *testing.T) {
	// Open a placed store.
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)

	// Write more blocks than three batches evict, and keep every third.
	const total = 3*evictBatchStartBlocks + 10
	var size int64
	kept := make(map[string]bool)
	var refs []*block.BlockRef
	for i := range total {
		data := fmt.Appendf(nil, "block-%05d", i)
		size = int64(len(data))
		ref, _, err := s.PutBlock(ctx, data, leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
		if i%3 == 0 {
			kept[ref.GetHash().MarshalString()] = true
		}
	}

	// Upload them all.
	remote := newInmemBlockStore()
	uploadCtx, uploadCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s.Upload(uploadCtx, remote) }()
	for {
		status, changed := s.GetStatus()
		if status.Pending == 0 {
			break
		}
		<-changed
	}
	uploadCancel()
	<-done

	// Admit the first batch and refuse the second.
	var asked []int64
	stop := errors.New("paused")
	admit := func(_ context.Context, lastBytes int64) error {
		if asked = append(asked, lastBytes); len(asked) > 1 {
			return stop
		}
		return nil
	}
	result, err := s.Evict(ctx, remote, 1<<30, nil, admit)
	if !errors.Is(err, stop) {
		t.Fatalf("evict err = %v, want the admit's", err)
	}
	batchBytes := evictBatchStartBlocks * size
	if result.Evicted != evictBatchStartBlocks || result.EvictedBytes != batchBytes {
		t.Fatalf("evict = %+v, want one batch", result)
	}
	if len(asked) != 2 || asked[0] != 0 || asked[1] != batchBytes {
		t.Fatalf("admit asked with %v, want [0 %d]", asked, batchBytes)
	}

	// Walk the rest in one pass, keeping every third block. A kept block
	// must not come up again in this pass.
	seen := make(map[string]int)
	keep := func(h *hash.Hash) bool {
		key := h.MarshalString()
		seen[key]++
		return kept[key]
	}
	result, err = s.Evict(ctx, remote, 1<<30, keep, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rest := total - evictBatchStartBlocks; result.Evicted+result.Requeued != rest || len(seen) != rest {
		t.Fatalf("second pass = %+v over %d entries, want %d", result, len(seen), rest)
	}
	for key, n := range seen {
		if n != 1 {
			t.Fatalf("second pass walked %s %d times", key, n)
		}
	}

	// The next pass drops exactly the blocks the second kept.
	requeued := result.Requeued
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != requeued || result.Requeued != 0 {
		t.Fatalf("third pass = %+v, want %d evicted", result, requeued)
	}

	// Read every block back from the remote in one burst: the fills commit in
	// a few batched steps, restoring the values and the queue's bytes.
	status, _ := s.GetStatus()
	before := status.EvictableBytes
	for _, ref := range refs {
		stored, err := remote.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil {
			t.Fatalf("remote read %s: %v", ref.MarshalString(), err)
		}
		s.Refill(ref, stored)
	}
	waitStatus(t, s, func(st Status) bool { return st.Refills == int64(len(refs)) })
	held, err := s.values.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range held {
		if !ok {
			t.Fatalf("refill %s not stored", refs[i].MarshalString())
		}
	}
	status, _ = s.GetStatus()
	if want := before + int64(len(refs))*size; status.EvictableBytes != want || status.DroppedFills != 0 {
		t.Fatalf("after refills evictable=%d dropped=%d, want %d and 0", status.EvictableBytes, status.DroppedFills, want)
	}

	// The refilled blocks are queued for eviction again.
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result.Evicted != len(refs) {
		t.Fatalf("pass after the refills = %+v, %v, want %d evicted", result, err, len(refs))
	}
}

// countingValues counts the batch writes to the local values.
type countingValues struct {
	block.StoreOps
	writes int
}

// PutBlockBatch counts the write and stores the batch.
func (c *countingValues) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	c.writes++
	return c.StoreOps.PutBlockBatch(ctx, entries)
}

// countingMarkers counts the write transactions on the marker store.
type countingMarkers struct {
	kvtx.Store
	writes int
}

// NewTransaction counts a write transaction and opens it.
func (c *countingMarkers) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		c.writes++
	}
	return c.Store.NewTransaction(ctx, write)
}

// TestEvictBatchesByBytes checks that once the drop writes run fast, one
// eviction batch carries 8,192 blocks of 3 KiB, 24 MiB, in one drop write and
// one queue commit, where a batch of 1,024 blocks took eight of each; and that
// the batch limit follows the drop write's time, halving when a write runs
// long and staying within its bounds.
func TestEvictBatchesByBytes(t *testing.T) {
	// Open a placed store and write 8,192 distinct blocks of 3 KiB.
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	const total, size = evictBatchMaxBlocks, 3 << 10
	for i := range total {
		data := make([]byte, size)
		copy(data, fmt.Appendf(nil, "block-%05d", i))
		if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, s, newInmemBlockStore())

	// Evict them all with the limit fast writes reach, counting the writes.
	values := &countingValues{StoreOps: s.values}
	markers := &countingMarkers{Store: s.markers}
	s.values, s.markers = values, markers
	s.evictLimit = evictBatchMaxBlocks
	result, err := s.Evict(ctx, nil, 1<<40, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != total || result.EvictedBytes != total*size {
		t.Fatalf("evict = %+v, want %d blocks of %d bytes", result, total, size)
	}
	if values.writes != 1 {
		t.Fatalf("%d drop writes, want 1", values.writes)
	}
	// One write advances the epoch, the other commits the queue.
	if markers.writes != 2 {
		t.Fatalf("%d marker store writes, want the epoch and 1 queue commit", markers.writes)
	}

	// The limit aims the next write at evictWriteTarget, moving at most a
	// factor of two a batch, within its bounds.
	for _, c := range []struct {
		limit, dropped int
		elapsed        time.Duration
		want           int
	}{
		{1024, 1024, 5 * time.Second, 512},
		{1024, 1024, 250 * time.Millisecond, 2048},
		{8192, 8192, 2 * time.Second, 4096},
		{1024, 1024, 700 * time.Millisecond, 731},
		{128, 128, 10 * time.Second, evictBatchMinBlocks},
		{8192, 4096, 10 * time.Millisecond, evictBatchMaxBlocks},
		{1024, 0, time.Minute, 1024},
	} {
		if got := nextEvictLimit(EvictBatch{}.normalize(), c.limit, c.dropped, c.elapsed); got != c.want {
			t.Fatalf("nextEvictLimit(%d, %d, %v) = %d, want %d", c.limit, c.dropped, c.elapsed, got, c.want)
		}
	}
}

// slowValues makes each drop write cost perBlock a block, as a store much
// larger than its page cache does, and records each write's blocks and time.
type slowValues struct {
	block.StoreOps
	perBlock time.Duration
	writes   []int
	took     []time.Duration
}

// PutBlockBatch waits perBlock for each entry, then stores the batch.
func (v *slowValues) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	start := time.Now()
	time.Sleep(time.Duration(len(entries)) * v.perBlock)
	ok, err := v.StoreOps.PutBlockBatch(ctx, entries)
	v.writes, v.took = append(v.writes, len(entries)), append(v.took, time.Since(start))
	return ok, err
}

// TestLoweredHighMarkKeepsBatchesShort checks that lowering the resident high
// mark while the store runs, so a pass must drop nearly every resident byte,
// still drops them in batches whose drop writes each hold the writer near the
// write target, not one long write: the batch limit shrinks to the target at
// the measured cost a block. Lowering the batch bounds mid-pass, as a live
// tunable does, applies at the next batch.
func TestLoweredHighMarkKeepsBatchesShort(t *testing.T) {
	// Scale the target down so the test runs in well under a second, keeping
	// the same ratio of target to cost a block as 500 ms at 1 ms a block.
	const target, perBlock = 20 * time.Millisecond, 40 * time.Microsecond
	SetEvictBatch(EvictBatch{WriteTarget: target, MinBlocks: 16})
	t.Cleanup(func() { SetEvictBatch(EvictBatch{}) })

	// Write and upload 6,000 blocks of 1 KiB under a high mark past them.
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	const total, size = 6000, 1 << 10
	for i := range total {
		data := make([]byte, size)
		copy(data, fmt.Appendf(nil, "block-%05d", i))
		if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, s, newInmemBlockStore())
	s.SetPolicy(Policy{ResidentHigh: 1 << 40, ResidentLow: 1 << 39})
	if status, _ := s.GetStatus(); status.EvictBytes() != 0 {
		t.Fatalf("evict bytes = %d under a high mark past the store", status.EvictBytes())
	}

	// Lower the high mark to a tenth of the store, as the resident tunable
	// does: the pass is asked to drop nearly everything.
	s.SetPolicy(Policy{ResidentHigh: total * size / 10, ResidentLow: total * size / 20})
	status, _ := s.GetStatus()
	want := status.EvictBytes()
	if want < total*size*9/10 {
		t.Fatalf("evict bytes = %d after lowering the high mark, want most of %d", want, total*size)
	}

	// Run the pass on slow drop writes, lowering the most blocks a batch
	// takes to 64 after the eighth batch.
	values := &slowValues{StoreOps: s.values, perBlock: perBlock}
	s.values = values
	batches := 0
	admit := func(context.Context, int64) error {
		if batches++; batches == 9 {
			SetEvictBatch(EvictBatch{WriteTarget: target, MinBlocks: 16, MaxBlocks: 64})
		}
		return nil
	}
	result, err := s.Evict(ctx, nil, want, nil, admit)
	if err != nil {
		t.Fatal(err)
	}
	if result.EvictedBytes < want {
		t.Fatalf("evicted %d bytes, want %d", result.EvictedBytes, want)
	}

	t.Logf("drop writes of %v blocks took %v", values.writes, values.took)

	// The first write takes the open's limit of 1,024 blocks; from the
	// fourth on, each takes near the target, never several times it.
	if values.writes[0] != evictBatchStartBlocks {
		t.Fatalf("first drop write took %d blocks, want %d", values.writes[0], evictBatchStartBlocks)
	}
	for i, took := range values.took {
		if i >= 3 && took > 3*target {
			t.Fatalf("drop write %d of %d blocks held the writer %v, want under %v; writes %v", i, values.writes[i], took, 3*target, values.writes)
		}
	}
	for i, n := range values.writes[8:] {
		if n > 64 {
			t.Fatalf("drop write %d took %d blocks after the bound fell to 64", i+8, n)
		}
	}
}

// TestFillQueueBoundsAndDedupes checks that a block read twice before its
// write queues once, that a fill past the policy's byte bound is dropped and
// counted, and that the drain writes every queued block and lets them queue
// again.
func TestFillQueueBoundsAndDedupes(t *testing.T) {
	// Open a placed store whose fill queue holds three blocks of eight bytes,
	// and hold the drain off so the queue keeps what it takes.
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	s.SetPolicy(Policy{FillQueueBytes: 3 * 8})
	s.fills.draining = true

	// Read four distinct blocks from the remote, the first one twice.
	remote := newInmemBlockStore()
	var refs []*block.BlockRef
	for i := range 4 {
		ref, _, err := remote.PutBlock(ctx, fmt.Appendf(nil, "fill-%03d", i), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	fillRef := func(ref *block.BlockRef) {
		stored, err := remote.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil {
			t.Fatalf("remote read %s: %v", ref.MarshalString(), err)
		}
		s.CacheFill(ref, stored)
	}
	fillRef(refs[0])
	fillRef(refs[0])
	for _, ref := range refs[1:] {
		fillRef(ref)
	}

	// The duplicate queued nothing, and the fourth block passed the bound.
	if n := len(s.fills.pending); n != 3 {
		t.Fatalf("queued %d fills, want 3", n)
	}
	if status, _ := s.GetStatus(); status.DroppedFills != 1 {
		t.Fatalf("dropped %d fills, want 1", status.DroppedFills)
	}

	// The drain writes the three queued blocks and empties the queue.
	s.drainFills()
	held, err := s.local.GetBlockExistsBatch(ctx, refs[:3])
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range held {
		if !ok {
			t.Fatalf("fill %s not stored", refs[i].MarshalString())
		}
	}
	if n := len(s.fills.queued); n != 0 || s.fills.draining {
		t.Fatalf("after the drain %d blocks are queued, draining=%v; want none", n, s.fills.draining)
	}
}

// TestEvictChecksOnlyWhenAsked checks that a pass given a check keeps a
// queued block the check does not hold, and that a pass without one trusts
// the queue and drops it.
func TestEvictChecksOnlyWhenAsked(t *testing.T) {
	ctx := t.Context()
	source := newInmemBlockStore()
	ref, _, err := source.PutBlock(ctx, []byte("peer-block"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := source.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		t.Fatalf("source read: %v", err)
	}
	for _, checked := range []bool{true, false} {
		// Fill the block read from the source into a placed store.
		s := newTestEnv(t).newStore(t, true)
		s.CacheFill(ref, stored)
		s.drainFills()

		// A check that lacks the block keeps it; no check drops it.
		var check block.StoreOps
		if checked {
			check = newInmemBlockStore()
		}
		result, err := s.Evict(ctx, check, 1<<30, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		held, err := s.values.GetBlockExistsBatch(ctx, []*block.BlockRef{ref})
		if err != nil {
			t.Fatal(err)
		}
		if checked && (result.Refused != 1 || result.Evicted != 0 || !held[0]) {
			t.Fatalf("checked pass = %+v held=%v, want 1 refused and kept", result, held[0])
		}
		if !checked && (result.Evicted != 1 || held[0]) {
			t.Fatalf("unchecked pass = %+v held=%v, want 1 evicted", result, held[0])
		}
	}
}
