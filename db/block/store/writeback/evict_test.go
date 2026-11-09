package block_store_writeback

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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
