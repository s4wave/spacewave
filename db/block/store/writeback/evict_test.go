package block_store_writeback

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
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
	const total = 3*evictBatchBlocks + 10
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
	batchBytes := evictBatchBlocks * size
	if result.Evicted != evictBatchBlocks || result.EvictedBytes != batchBytes {
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
	if rest := total - evictBatchBlocks; result.Evicted+result.Requeued != rest || len(seen) != rest {
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

// countingKV counts the write transactions opened on a key-value store.
type countingKV struct {
	kvtx.Store
	writes atomic.Int64
}

// SupportsAtomicCommit reports the inner store's atomic commits.
func (c *countingKV) SupportsAtomicCommit() bool { return true }

// NewTransaction counts a write transaction.
func (c *countingKV) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		c.writes.Add(1)
	}
	return c.Store.NewTransaction(ctx, write)
}

// putUploaded writes n blocks to s and uploads them to a new remote, which
// it returns with their refs.
func putUploaded(t *testing.T, s *Store, n int) (block.StoreOps, []*block.BlockRef) {
	t.Helper()
	var refs []*block.BlockRef
	for i := range n {
		ref, _, err := s.PutBlock(t.Context(), fmt.Appendf(nil, "block-%06d", i), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	remote := newInmemBlockStore()
	drain(t, s, remote)
	return remote, refs
}

// TestEvictCommitsEachBatchOnce checks that a pass over a Volume's values
// drops each batch's values and changes the queue in one write transaction,
// that the next batch grows while a batch holds the writer briefly, and that
// the pass leaves no value or entry behind.
func TestEvictCommitsEachBatchOnce(t *testing.T) {
	// Open a placed store over a Volume whose writes are counted.
	ctx := t.Context()
	kv := &countingKV{Store: store_kvtx_inmem.NewStore()}
	vol, err := volume_kvtx.NewVolume(ctx, "test/writeback", store_kvkey.NewDefaultKVKey(), kv, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	s, err := NewStore(ctx, ownedStore{vol}, vol, markers, testMarkersID, true)
	if err != nil {
		t.Fatal(err)
	}
	_, refs := putUploaded(t, s, 4*evictBatchBlocks)

	// Evict them all: the epoch takes one write and each batch one more, and
	// the batches grow from evictBatchBlocks, so fewer than four walk them.
	before := kv.writes.Load()
	evictHoldHook = func(n int, hold time.Duration) { t.Logf("batch of %d entries held the writer %v", n, hold) }
	defer func() { evictHoldHook = nil }()
	result, err := s.Evict(ctx, nil, 1<<30, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Evicted != len(refs) || result.Batches < 2 || result.Batches > 3 {
		t.Fatalf("evict = %+v, want %d evicted in growing batches", result, len(refs))
	}
	if writes := kv.writes.Load() - before; writes != int64(1+result.Batches) {
		t.Fatalf("pass opened %d write transactions, want %d", writes, 1+result.Batches)
	}
	if result.Hold <= 0 || result.Hold > time.Second {
		t.Fatalf("longest hold = %v, want a measured hold under a second", result.Hold)
	}

	// No value and no entry remains.
	held, err := vol.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range held {
		if ok {
			t.Fatalf("evicted %s still on disk", refs[i].MarshalString())
		}
	}
	if status, _ := s.GetStatus(); status.EvictableBytes != 0 {
		t.Fatalf("evictable bytes = %d, want 0", status.EvictableBytes)
	}
}

// TestEvictStepEndsPass checks that a step that declines a batch ends the
// pass after the batches before it, reporting Stopped, and that the next pass
// resumes where it stopped.
func TestEvictStepEndsPass(t *testing.T) {
	// Upload more blocks than one batch evicts.
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	remote, refs := putUploaded(t, s, evictBatchBlocks+10)

	// Run one batch, then decline the next.
	var steps int
	result, err := s.EvictSteps(ctx, 1<<30, nil, nil, func(_ context.Context, batch func(block.StoreOps) error) error {
		if steps++; steps > 1 {
			return nil
		}
		return batch(remote)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stopped || result.Batches != 1 || result.Evicted != evictBatchBlocks {
		t.Fatalf("stopped pass = %+v, want one batch then Stopped", result)
	}

	// The next pass evicts the rest.
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result.Stopped || result.Evicted != len(refs)-evictBatchBlocks {
		t.Fatalf("next pass = %+v/%v, want the other %d evicted", result, err, len(refs)-evictBatchBlocks)
	}
}

// TestNextEvictBatch checks that a batch that held the writer past
// evictHoldHigh at least halves the next, toward evictHoldTarget, that one
// under evictHoldLow doubles it, and that the size stays within its bounds.
func TestNextEvictBatch(t *testing.T) {
	cases := []struct {
		n    int
		hold time.Duration
		want int
	}{
		{1024, 0, 2048},
		{1024, evictHoldLow - 1, 2048},
		{1024, evictHoldLow, 1024},
		{1024, evictHoldHigh, 1024},
		{1024, evictHoldHigh + 1, 511},
		{1024, 400 * time.Millisecond, 128},
		{1024, 10 * time.Second, evictBatchMin},
		{evictBatchMin, 10 * time.Second, evictBatchMin},
		{evictBatchMax, 0, evictBatchMax},
	}
	for _, c := range cases {
		if got := nextEvictBatch(c.n, c.hold); got != c.want {
			t.Errorf("nextEvictBatch(%d, %v) = %d, want %d", c.n, c.hold, got, c.want)
		}
	}
}
