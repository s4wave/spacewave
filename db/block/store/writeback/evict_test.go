package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// evictCommitStore fails a transaction that changes eviction entries.
type evictCommitStore struct {
	kvtx.Store
	// err is the failure returned instead of committing queue changes.
	err error
	// allow permits this many queue-changing commits before the failure.
	allow int
}

// NewTransaction wraps the real transaction without changing its reads or writes.
func (s *evictCommitStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &evictCommitTx{Tx: tx, store: s}, nil
}

// evictCommitTx discards prepared queue changes when its commit fails.
type evictCommitTx struct {
	kvtx.Tx
	// store controls when queue-changing commits start to fail.
	store *evictCommitStore
	// changed records whether this transaction mutates an eviction entry.
	changed bool
}

// Delete records an eviction queue change while staging the real deletion.
func (t *evictCommitTx) Delete(ctx context.Context, key []byte) error {
	t.changed = t.changed || bytes.HasPrefix(key, []byte(evictPrefix))
	return t.Tx.Delete(ctx, key)
}

// Set records insertion as well as movement of an eviction queue entry.
func (t *evictCommitTx) Set(ctx context.Context, key, value []byte) error {
	t.changed = t.changed || bytes.HasPrefix(key, []byte(evictPrefix))
	return t.Tx.Set(ctx, key, value)
}

// Commit fails queue changes after their writes are staged, before they commit.
func (t *evictCommitTx) Commit(ctx context.Context) error {
	// Refuse the selected queue transaction before it changes durable state.
	if t.changed && t.store.allow == 0 {
		return t.store.err
	}

	// Count only successful slices toward the injected failure point.
	err := t.Tx.Commit(ctx)
	if err == nil && t.changed {
		t.store.allow--
	}
	return err
}

// TestEvictRefusedPartialCommitFailure preserves the byte sum and every
// local value across a restart between committed and failed metadata slices.
func TestEvictRefusedPartialCommitFailure(t *testing.T) {
	// Queue more refusals than one metadata slice can move.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	remote := newInmemBlockStore()
	var refs []*block.BlockRef
	var size int64
	for i := range evictRetryCommitBlocks + 1 {
		data := fmt.Appendf(nil, "partial-refusal-%05d", i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		size += int64(len(data))
		refs = append(refs, ref)
		s.Refill(ref, &block.StoredBlock{Data: data, RefsKnown: true})
	}
	waitStatus(t, s, func(st Status) bool { return st.Refills == int64(len(refs)) })

	// Let the first atomic slice commit, then fail the next one.
	failure := errors.New("second metadata slice failed")
	s.markers = &evictCommitStore{Store: s.markers, err: failure, allow: 1}
	result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
	if !errors.Is(err, failure) || result != (EvictResult{Refused: len(refs)}) {
		t.Fatalf("partial metadata commit = %+v, %v", result, err)
	}
	if s.evictFrom != nil {
		t.Fatal("partial metadata commit advanced the cursor")
	}

	// Check the two epochs and durable byte sum before recovery can repair it.
	err = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		counts := make(map[uint64]int)
		if err := scanEvictable(ctx, tx, nil, func(entry evictEntry) error {
			counts[entry.epoch]++
			return nil
		}); err != nil {
			return err
		}
		if len(counts) != 2 || counts[0] != 1 || counts[1] != evictRetryCommitBlocks {
			t.Fatalf("partially committed epochs = %v", counts)
		}
		total, err := readEvictableBytes(ctx, tx)
		if err == nil && total != size {
			t.Fatalf("partially committed bytes = %d, want %d", total, size)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reopen after the partial move and retry every missing remote copy safely.
	env.open(t)
	s = env.newStore(t, true)
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Refused: len(refs)}) {
		t.Fatalf("retry after partial commit = %+v, %v", result, err)
	}
	if status, _ := s.GetStatus(); status.EvictableBytes != size {
		t.Fatalf("queue after partial commit recovery = %+v", status)
	}
	for _, ref := range refs {
		if _, found, err := s.GetBlock(ctx, ref); err != nil || !found {
			t.Fatalf("retry lost a local copy: found=%v, err=%v", found, err)
		}
	}
}

// evictDropStore injects write failures or gates a value write.
type evictDropStore struct {
	block.StoreOps
	// err is the injected write failure.
	err error
	// put gates a value write when configured instead of returning err.
	put func(context.Context, []*block.PutBatchEntry) ([]bool, error)
}

// PutBlockBatch gates the write when configured, or returns the injected failure.
func (s *evictDropStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if s.put != nil {
		return s.put(ctx, entries)
	}
	return nil, s.err
}

// TestEvictRefusedDropFailure preserves master's refusal counts when the
// tombstone batch fails, under either retry policy.
func TestEvictRefusedDropFailure(t *testing.T) {
	// Fail a batch containing both a refusal and a remotely held block.
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			// Upload two blocks, then remove one remote copy.
			SetEvictRefusedRetry(retry)
			t.Cleanup(func() { SetEvictRefusedRetry(false) })
			ctx := t.Context()
			s := newTestEnv(t).newStore(t, true)
			remote := newInmemBlockStore()
			var refs []*block.BlockRef
			for _, data := range []string{"missing", "held"} {
				ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
				if err != nil {
					t.Fatal(err)
				}
				refs = append(refs, ref)
			}
			drain(t, s, remote)
			if err := remote.RmBlock(ctx, refs[0]); err != nil {
				t.Fatal(err)
			}

			// Refusal is decided before the tombstone write can fail.
			failure := errors.New("tombstone write failed")
			s.values = &evictDropStore{StoreOps: s.values, err: failure}
			result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
			if !errors.Is(err, failure) || result != (EvictResult{Refused: 1}) {
				t.Fatalf("failed tombstones = %+v, %v", result, err)
			}
			if s.evictFrom != nil {
				t.Fatal("failed tombstones advanced the cursor")
			}
		})
	}
}

// TestEvictRefusedCommitFailure keeps the old entry when its replacement
// fails to commit, including a restart after another value was dropped.
func TestEvictRefusedCommitFailure(t *testing.T) {
	// Both policies use master's result boundaries on metadata failures.
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			// Upload two blocks, then remove one remote copy and fail the queue commit.
			SetEvictRefusedRetry(retry)
			t.Cleanup(func() { SetEvictRefusedRetry(false) })
			ctx := t.Context()
			env := newTestEnv(t)
			s := env.newStore(t, true)
			remote := newInmemBlockStore()
			refs := make([]*block.BlockRef, 0, 2)
			for _, data := range []string{"missing remote copy", "held remote copy"} {
				ref, _, err := s.PutBlock(ctx, []byte(data), leafOpts)
				if err != nil {
					t.Fatal(err)
				}
				refs = append(refs, ref)
			}
			drain(t, s, remote)
			if err := remote.RmBlock(ctx, refs[0]); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("queue commit failed")
			s.markers = &evictCommitStore{Store: s.markers, err: failure}

			// Master counts refusal decisions before writes, successful tombstones
			// immediately, and bytes only after the metadata commit succeeds.
			result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
			if !errors.Is(err, failure) {
				t.Fatalf("evict error = %v, want injected failure", err)
			}
			if result.Evicted != 1 || result.EvictedBytes != 0 || result.Refused != 1 {
				t.Fatalf("failed queue commit result = %+v", result)
			}
			if s.evictFrom != nil {
				t.Fatal("failed queue commit advanced the cursor")
			}

			// Inspect both original epoch-zero keys before any retry can repair
			// them, along with the byte sum in the same persisted snapshot.
			err = kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
				seen := make(map[string]bool, len(refs))
				if err := scanEvictable(ctx, tx, nil, func(entry evictEntry) error {
					if entry.epoch != 0 {
						t.Fatalf("failed commit moved an entry to epoch %d", entry.epoch)
					}
					seen[entry.hash.MarshalString()] = true
					return nil
				}); err != nil {
					return err
				}
				if len(seen) != len(refs) {
					t.Fatalf("failed commit left %d entries, want %d", len(seen), len(refs))
				}
				for _, ref := range refs {
					if !seen[ref.GetHash().MarshalString()] {
						t.Fatal("failed commit lost an original queue entry")
					}
				}
				total, err := readEvictableBytes(ctx, tx)
				if err != nil {
					return err
				}
				if want := int64(len("missing remote copy") + len("held remote copy")); total != want {
					t.Fatalf("failed commit left queue bytes %d, want %d", total, want)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			// Reopen without the injector: the old queue entry survives, and the
			// already-dropped value is only cleaned up, never counted as another drop.
			env.open(t)
			s = env.newStore(t, true)
			result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.Refused != 1 || result.Evicted != 0 {
				t.Fatalf("restart after failed commit = %+v", result)
			}
			if data, found, err := s.GetBlock(ctx, refs[0]); err != nil || !found || string(data) != "missing remote copy" {
				t.Fatalf("missing remote copy lost on restart: %q, %v, %v", data, found, err)
			}
			var wantBytes int64
			if retry {
				wantBytes = int64(len("missing remote copy"))
			}
			if status, _ := s.GetStatus(); status.EvictableBytes != wantBytes {
				t.Fatalf("queue after recovery = %+v", status)
			}
		})
	}
}

// TestEvictRefusedRetry retains both refusal causes across a reopen, checks
// the remote again on every pass, and evicts only after each cause clears.
func TestEvictRefusedRetry(t *testing.T) {
	// Exercise both refusal modes with the opt-in behavior and its rollback.
	for _, retry := range []bool{false, true} {
		for _, marked := range []bool{false, true} {
			t.Run(fmt.Sprintf("retry=%t/marked=%t", retry, marked), func(t *testing.T) {
				// Queue one uploaded block, then make its remote copy absent or
				// rewrite it so an upload marker prevents eviction.
				SetEvictRefusedRetry(retry)
				t.Cleanup(func() { SetEvictRefusedRetry(false) })
				ctx := t.Context()
				env := newTestEnv(t)
				s := env.newStore(t, true)
				remote := newInmemBlockStore()
				data := []byte("refused local copy")
				ref, _, err := s.PutBlock(ctx, data, leafOpts)
				if err != nil {
					t.Fatal(err)
				}
				drain(t, s, remote)
				if marked {
					if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
						t.Fatal(err)
					}
				}
				if !marked {
					if err := remote.RmBlock(ctx, ref); err != nil {
						t.Fatal(err)
					}
				}

				// A refusal keeps the exact local bytes and marker state. A
				// retained entry stays counted without claiming a caller keep.
				result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if result.Refused != 1 || result.Evicted != 0 || result.Requeued != 0 {
					t.Fatalf("refusal = %+v", result)
				}
				if got, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(got) != string(data) {
					t.Fatalf("local read after refusal = %q, %v, %v", got, found, err)
				}
				if got := env.isMarked(t, ref); got != marked {
					t.Fatalf("marked = %v, want %v", got, marked)
				}
				var wantBytes int64
				if retry {
					wantBytes = int64(len(data))
				}
				if status, _ := s.GetStatus(); status.EvictableBytes != wantBytes {
					t.Fatalf("evictable = %d, want %d", status.EvictableBytes, wantBytes)
				}

				// Reopen the Volume before retrying. Even with no new upload,
				// a still-missing remote copy must remain readable locally.
				env.open(t)
				s = env.newStore(t, true)
				result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantRefused := 0
				if retry {
					wantRefused = 1
				}
				if result.Refused != wantRefused || result.Evicted != 0 {
					t.Fatalf("restart pass = %+v, want %d refusals", result, wantRefused)
				}
				if got, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(got) != string(data) {
					t.Fatalf("local read after restart retry = %q, %v, %v", got, found, err)
				}

				// Restore remote possession without synthesizing an upload for
				// the unmarked block; clear a real marker through Upload.
				if _, _, err := remote.PutBlock(ctx, data, leafOpts); err != nil {
					t.Fatal(err)
				}
				if marked {
					drain(t, s, remote)
				}
				result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantEvicted := 0
				if retry || marked {
					wantEvicted = 1
				}
				if result.Evicted != wantEvicted || result.Refused != 0 {
					t.Fatalf("cleared cause = %+v, want %d evictions", result, wantEvicted)
				}
				if got, found, err := remote.GetBlock(ctx, ref); err != nil || !found || string(got) != string(data) {
					t.Fatalf("remote read after eviction = %q, %v, %v", got, found, err)
				}
			})
		}
	}
}

// TestEvictPermanentRefusalDoesNotStarve crosses a batch boundary of refused
// entries at a one-byte target and still reaches a remotely held value.
func TestEvictPermanentRefusalDoesNotStarve(t *testing.T) {
	// Queue only unmarked fills; every entry but the last lacks a remote copy.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	remote := newInmemBlockStore()
	entries := make([]*block.PutBatchEntry, 0, evictBatchBlocks+1)
	for i := range evictBatchBlocks + 1 {
		data := fmt.Appendf(nil, "permanent-refusal-%05d", i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
	}

	// Order the queue exactly as a pass sees it, making the sole evictable
	// block sort last so an early refusal cannot mask starvation.
	slices.SortFunc(entries, func(a, b *block.PutBatchEntry) int {
		return strings.Compare(a.Ref.GetHash().MarshalString(), b.Ref.GetHash().MarshalString())
	})
	last := entries[len(entries)-1]
	if _, _, err := remote.PutBlock(ctx, last.Data, leafOpts); err != nil {
		t.Fatal(err)
	}

	// Let the normal refill producer land every local value and queue entry.
	for _, entry := range entries {
		s.Refill(entry.Ref, &block.StoredBlock{Data: entry.Data, RefsKnown: true})
	}
	waitStatus(t, s, func(st Status) bool { return st.Refills == int64(len(entries)) })

	// Each refused entry is counted once while the held block is evicted;
	// another pass returns despite the permanently absent remote copies.
	result, err := s.Evict(ctx, remote, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Refused != evictBatchBlocks || result.Evicted != 1 || result.Requeued != 0 {
		t.Fatalf("mixed pass = %+v", result)
	}
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Refused != evictBatchBlocks || result.Evicted != 0 {
		t.Fatalf("permanent refusal pass = %+v", result)
	}
}

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

// TestDroppedClearAfterFillKeepsEvictionCoverage reproduces a dropped upload
// whose cache fill deduplicates against an older entry before marked refusal.
func TestDroppedClearAfterFillKeepsEvictionCoverage(t *testing.T) {
	// Upload once, then leave a new marker beside the old eviction entry.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	env := newTestEnv(t)
	s := env.newStore(t, true)
	remote := newInmemBlockStore()
	data := []byte("filled after absent upload read")
	ref, _, err := s.PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s, remote)
	if _, _, err := s.PutBlock(ctx, data, leafOpts); err != nil {
		t.Fatal(err)
	}

	// Let upload observe absence, then restore the value without a re-put.
	if err := s.local.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	head, err := s.scanHead(ctx, batchLimitsOf(nil), nil)
	if err != nil || len(head.entries) != 1 {
		t.Fatalf("scan = %v, %v", head, err)
	}
	if err := s.uploadChunk(ctx, remote, head.entries); err != nil || !head.entries[0].dropped {
		t.Fatalf("upload = %v, dropped=%v", err, head.entries[0].dropped)
	}
	if err := s.writeFills(ctx, []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsKnown: true}}}); err != nil {
		t.Fatal(err)
	}

	// Refusal keeps the value pending until the dropped upload clears its marker.
	result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Refused: 1}) {
		t.Fatalf("marked refusal = %+v, %v", result, err)
	}
	if status, _ := s.GetStatus(); status.PendingBytes != int64(len(data)) {
		t.Fatalf("pending value after refusal = %+v", status)
	}

	// The dropped clear removes the marker but preserves eviction coverage.
	if err := s.clear(ctx, head.entries); err != nil {
		t.Fatal(err)
	}
	if env.isMarked(t, ref) {
		t.Fatal("dropped clear retained the marker")
	}
	env.open(t)
	s = env.newStore(t, true)
	if status, _ := s.GetStatus(); status.PendingBytes != 0 || status.EvictableBytes != int64(len(data)) {
		t.Fatalf("reopened coverage = %+v", status)
	}
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Evicted: 1, EvictedBytes: int64(len(data))}) {
		t.Fatalf("eviction after dropped clear = %+v, %v", result, err)
	}
}

// TestFillQueueFailureCannotStrandValue fails the queue commit before a fill
// can expose local bytes, then checks both cleanup and a later successful fill.
func TestFillQueueFailureCannotStrandValue(t *testing.T) {
	// Fail the metadata reservation for a cache fill and a value-only refill.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	for _, refill := range []bool{false, true} {
		t.Run(fmt.Sprintf("refill=%t", refill), func(t *testing.T) {
			// Start with remote bytes and no local value or upload marker.
			ctx := t.Context()
			env := newTestEnv(t)
			s := env.newStore(t, true)
			remote := newInmemBlockStore()
			data := []byte("queue failure must precede value write")
			ref, _, err := remote.PutBlock(ctx, data, leafOpts)
			if err != nil {
				t.Fatal(err)
			}
			batch := []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsKnown: true}, refill: refill}}
			failure := errors.New("fill queue commit failed")
			s.markers = &evictCommitStore{Store: s.markers, err: failure}

			// Refuse the queue commit and attempt to strand an untracked value.
			if err := s.writeFills(ctx, batch); !errors.Is(err, failure) {
				t.Fatalf("failed fill = %v", err)
			}
			if found, err := s.values.GetBlockExists(ctx, ref); err != nil || found {
				t.Fatalf("value after queue failure = %v, %v", found, err)
			}
			env.open(t)
			s = env.newStore(t, true)
			if status, _ := s.GetStatus(); status.EvictableBytes != 0 {
				t.Fatalf("failed fill queue persisted: %+v", status)
			}

			// A value-write failure leaves only a surplus queue entry to clean.
			values := s.values
			if refill {
				s.values = &evictDropStore{StoreOps: values, err: failure}
				if err := s.writeFills(ctx, batch); !errors.Is(err, failure) {
					t.Fatalf("failed value write = %v", err)
				}
				s.values = values
				result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
				if err != nil || result != (EvictResult{}) {
					t.Fatalf("clean surplus queue = %+v, %v", result, err)
				}
			}

			// Refill successfully, then try to evict with the remote copy absent.
			if err := s.writeFills(ctx, batch); err != nil {
				t.Fatal(err)
			}
			if err := remote.RmBlock(ctx, ref); err != nil {
				t.Fatal(err)
			}
			env.open(t)
			s = env.newStore(t, true)
			result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
			if err != nil || result != (EvictResult{Refused: 1}) {
				t.Fatalf("missing remote refusal = %+v, %v", result, err)
			}
			if got, found, err := s.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
				t.Fatalf("last copy after failed eviction = %q, %v, %v", got, found, err)
			}
		})
	}
}

// TestFillReservationExcludesEviction attempts to clean a reserved queue entry
// while the fill has not yet exposed its local value.
func TestFillReservationExcludesEviction(t *testing.T) {
	// Hold the value write after its queue reservation has committed.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	remote := newInmemBlockStore()
	data := []byte("reserved fill awaiting its value")
	ref, _, err := remote.PutBlock(ctx, data, leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	values := s.values
	s.values = &evictDropStore{StoreOps: values, put: func(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
		// Expose the reservation/write boundary without holding the KV writer.
		close(entered)
		select {
		case <-release:
			return values.PutBlockBatch(ctx, entries)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	filled := make(chan error, 1)
	go func() {
		filled <- s.writeFills(ctx, []fill{{ref: ref, stored: &block.StoredBlock{Data: data, RefsKnown: true}, refill: true}})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fill did not reach the reserved value write")
	}

	// A pass must wait for the fill rather than delete its absent-value entry.
	passCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := s.Evict(passCtx, remote, 1<<30, nil, nil); !errors.Is(err, context.Canceled) || !errors.Is(passCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("eviction entered unfinished fill: %v", err)
	}
	if status, _ := s.GetStatus(); status.EvictableBytes != int64(len(data)) {
		t.Fatalf("reservation after waiting pass = %+v", status)
	}

	// Finish the value write, then consume the still-present queue entry.
	close(release)
	select {
	case err := <-filled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("fill did not finish after release")
	}

	// Remove the gate before the eviction writes its tombstone batch.
	s.values = values
	result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Evicted: 1, EvictedBytes: int64(len(data))}) {
		t.Fatalf("eviction after completed fill = %+v, %v", result, err)
	}
}

// TestMarkedRefusalDuplicateEpochCounterexample checks the candidate against
// a marked hash queued in two epochs, with a byte target between its entries.
func TestMarkedRefusalDuplicateEpochCounterexample(t *testing.T) {
	// Choose queue order A, marked H, B so the first pass stops before H.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	s := newIndexTestStore(t, newTestEnv(t))
	SetEvictQueueIndex(true)
	t.Cleanup(func() { SetEvictQueueIndex(false) })
	remote := newInmemBlockStore()
	type value struct {
		ref  *block.BlockRef
		data []byte
	}
	values := make([]value, 0, 3)
	for i := range 3 {
		data := fmt.Appendf(nil, "duplicate-epoch-%d", i)
		ref, _, err := s.PutBlock(ctx, data, leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, value{ref: ref, data: data})
	}
	slices.SortFunc(values, func(a, b value) int { return bytes.Compare(evictKey(0, a.ref.GetHash()), evictKey(0, b.ref.GetHash())) })
	drain(t, s, remote)

	// Advance the epoch by evicting only A, leaving H and B in epoch zero.
	result, err := s.Evict(ctx, remote, 1, nil, nil)
	if err != nil || result.Evicted != 1 {
		t.Fatalf("first pass = %+v, %v", result, err)
	}
	h := values[1]
	if _, _, err := s.PutBlock(ctx, h.data, leafOpts); err != nil {
		t.Fatal(err)
	}
	drain(t, s, remote)
	if _, _, err := s.PutBlock(ctx, h.data, leafOpts); err != nil {
		t.Fatal(err)
	}

	// Marker creation already removed H; the second one-byte pass evicts B.
	result, err = s.Evict(ctx, remote, 1, nil, nil)
	if err != nil || result.Refused != 0 || result.Evicted != 1 {
		t.Fatalf("second pass = %+v, %v", result, err)
	}
	status, _ := s.GetStatus()
	t.Logf("refused=%d pending=%d evictable=%d actual=%d", result.Refused, status.PendingBytes, status.EvictableBytes, len(h.data))
	if status.EvictableBytes != 0 {
		t.Fatalf("refused marked hash remains double-counted: %+v", status)
	}
}
