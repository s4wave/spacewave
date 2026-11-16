package block_store_writeback

// Refusal retry tests ported from Spacewave master c03394ac8 (PR 1504). The
// pin bounds a batch by its adaptive block limit, which starts at
// evictBatchStartBlocks, where master uses a fixed evictBatchBlocks.

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// evictCommitStore fails a transaction that changes existing eviction entries.
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

// evictCommitTx discards a prepared queue move when its commit fails.
type evictCommitTx struct {
	kvtx.Tx
	// store controls when queue-changing commits start to fail.
	store *evictCommitStore
	// moved records whether this transaction deletes an eviction entry.
	moved bool
}

// Delete records an eviction queue change while staging the real deletion.
func (t *evictCommitTx) Delete(ctx context.Context, key []byte) error {
	t.moved = t.moved || bytes.HasPrefix(key, []byte(evictPrefix))
	return t.Tx.Delete(ctx, key)
}

// Commit fails a queue move after its writes are staged, before they commit.
func (t *evictCommitTx) Commit(ctx context.Context) error {
	// Refuse the selected queue transaction before it changes durable state.
	if t.moved && t.store.allow == 0 {
		return t.store.err
	}

	// Count only successful slices toward the injected failure point.
	err := t.Tx.Commit(ctx)
	if err == nil && t.moved {
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

// evictDropStore fails the tombstone write before it changes local values.
type evictDropStore struct {
	block.StoreOps
	// err is the injected write failure.
	err error
}

// PutBlockBatch returns the injected failure without writing the batch.
func (s *evictDropStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
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
	entries := make([]*block.PutBatchEntry, 0, evictBatchStartBlocks+1)
	for i := range evictBatchStartBlocks + 1 {
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
	if result.Refused != evictBatchStartBlocks || result.Evicted != 1 || result.Requeued != 0 {
		t.Fatalf("mixed pass = %+v", result)
	}
	result, err = s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Refused != evictBatchStartBlocks || result.Evicted != 0 {
		t.Fatalf("permanent refusal pass = %+v", result)
	}
}

// TestEvictRetryOnMixedBatch runs the scenario of
// TestEvictRetryOffKeepsPinBehavior with refusal retry on: refused entries
// move to the new epoch beside the kept ones, Requeued still counts only the
// kept ones, and the queue changes commit in slices of evictRetryCommitBlocks.
func TestEvictRetryOnMixedBatch(t *testing.T) {
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	f := newEvictMixScenario(t)
	result, err := f.store.Evict(t.Context(), f.remote, 1<<30, f.keep, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := EvictResult{Evicted: 70, EvictedBytes: f.sizes["held"], Requeued: 5, Refused: 25}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	if got, want := f.queue(t), f.want("missing", "marked", "kept"); !slices.Equal(got, want) {
		t.Fatalf("queue = %q, want %q", got, want)
	}
	var total int
	for _, kind := range evictMixKinds {
		total += kind.count
	}
	if wantMoves := (total + evictRetryCommitBlocks - 1) / evictRetryCommitBlocks; f.moves.moves != wantMoves {
		t.Fatalf("queue commits = %d, want %d slices", f.moves.moves, wantMoves)
	}
	status, _ := f.store.GetStatus()
	wantBytes := f.sizes["missing"] + f.sizes["marked"] + f.sizes["kept"]
	if status.EvictableBytes != wantBytes || status.EvictedBytes != f.sizes["held"] {
		t.Fatalf("status evictable = %d, evicted = %d", status.EvictableBytes, status.EvictedBytes)
	}
	f.checkLocal(t)
}

// TestEvictRetryNeedsCheck checks that with refusal retry on, a pass without a
// remote check fails before it changes anything, so an entry a checked pass
// refused and kept queued is never dropped on the queue's word; and that with
// retry off the unchecked pass drops it, as the evictor did before retry.
func TestEvictRetryNeedsCheck(t *testing.T) {
	// Fill an unmarked block the remote lacks, as a peer fill does.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	ctx := t.Context()
	s := newTestEnv(t).newStore(t, true)
	remote := newInmemBlockStore()
	data := []byte("unmarked fill the remote lacks")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Refill(ref, &block.StoredBlock{Data: data, RefsKnown: true})
	waitStatus(t, s, func(st Status) bool { return st.Refills == 1 })

	// A checked pass refuses it and keeps it queued.
	result, err := s.Evict(ctx, remote, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Refused: 1}) {
		t.Fatalf("checked pass = %+v, %v", result, err)
	}
	readEpochNow := func() uint64 {
		var epoch uint64
		if err := kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
			epoch, err = readEpoch(ctx, tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return epoch
	}
	epoch := readEpochNow()
	durable := &evictMixScenario{store: s}
	before := durable.queue(t)
	if len(before) != 3 {
		t.Fatalf("durable queue before the unchecked pass = %v, want one entry, the epoch and the byte sum", before)
	}

	// An unchecked pass fails before advancing the epoch, and the local bytes
	// and the durable queue keys, epoch and byte sum stay as they were.
	result, err = s.Evict(ctx, nil, 1<<30, nil, nil)
	if !errors.Is(err, ErrEvictRetryUnchecked) || result != (EvictResult{}) {
		t.Fatalf("unchecked pass under retry = %+v, %v", result, err)
	}
	if after := durable.queue(t); !slices.Equal(after, before) {
		t.Fatalf("durable queue after the unchecked pass = %v, want %v", after, before)
	}
	if got := readEpochNow(); got != epoch {
		t.Fatalf("unchecked pass advanced the epoch to %d from %d", got, epoch)
	}
	if got, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(got) != string(data) {
		t.Fatalf("local read after unchecked pass = %q, %v, %v", got, found, err)
	}
	if status, _ := s.GetStatus(); status.EvictableBytes != int64(len(data)) {
		t.Fatalf("queue after unchecked pass = %+v", status)
	}

	// With retry off the unchecked pass trusts the queue and drops the
	// block, as before retry.
	SetEvictRefusedRetry(false)
	result, err = s.Evict(ctx, nil, 1<<30, nil, nil)
	if err != nil || result != (EvictResult{Evicted: 1, EvictedBytes: int64(len(data))}) {
		t.Fatalf("unchecked pass with retry off = %+v, %v", result, err)
	}
	if _, found, err := s.values.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("local value after unchecked pass with retry off: found=%v, err=%v", found, err)
	}
}

// TestEvictRetryBusySecondSlice fails the second metadata slice of a retried
// batch busy once, after the first committed, with both slices changing the
// byte sum: the absent and dropped entries leave it. At the failure the
// durable keys and byte sum hold the first slice alone, the status projects
// the same bytes, and the dropped bytes are not reported yet; after the retry
// the queue, sums and counts are those of an undisturbed pass.
func TestEvictRetryBusySecondSlice(t *testing.T) {
	// Build the mixed batch, and fail its second queue commit busy once.
	SetEvictRefusedRetry(true)
	t.Cleanup(func() { SetEvictRefusedRetry(false) })
	f := newEvictMixScenario(t)
	f.moves.busy, f.moves.busyAfter = 1, 1

	// List the entries in scan order with their kinds, to predict the state
	// after the first slice.
	type queued struct {
		ref  *block.BlockRef
		kind string
		size int64
	}
	var order []queued
	for _, kind := range evictMixKinds {
		for i, ref := range f.refs[kind.name] {
			order = append(order, queued{ref, kind.name, int64(len(evictMixData(kind.name, i)))})
		}
	}
	slices.SortFunc(order, func(a, b queued) int {
		return strings.Compare(a.ref.GetHash().MarshalString(), b.ref.GetHash().MarshalString())
	})
	if len(order) <= evictRetryCommitBlocks {
		t.Fatalf("batch of %d entries fits one slice", len(order))
	}
	retained := map[string]bool{"missing": true, "marked": true, "kept": true}
	var wantMid []string
	var midBytes, firstDelta, secondDelta int64
	for i, entry := range order {
		switch {
		case i >= evictRetryCommitBlocks:
			wantMid = append(wantMid, fmt.Sprintf("%s=%d", evictKey(0, entry.ref.GetHash()), entry.size))
			midBytes += entry.size
			if !retained[entry.kind] {
				secondDelta -= entry.size
			}
		case retained[entry.kind]:
			wantMid = append(wantMid, fmt.Sprintf("%s=%d", evictKey(1, entry.ref.GetHash()), entry.size))
			midBytes += entry.size
		default:
			firstDelta -= entry.size
		}
	}
	if firstDelta == 0 || secondDelta == 0 {
		t.Fatalf("slice byte deltas %d and %d, want both nonzero", firstDelta, secondDelta)
	}
	wantMid = append(wantMid, epochKey+"=1", fmt.Sprintf("%s=%d", evictableBytesKey, midBytes))
	slices.Sort(wantMid)

	// At the busy failure check the projected state, and once its
	// transaction is done, before the retry, the durable state.
	var checked bool
	f.moves.afterBusy = func() {
		checked = true
		if got := f.queue(t); !slices.Equal(got, wantMid) {
			t.Errorf("queue at the busy slice = %q, want %q", got, wantMid)
		}
	}
	f.moves.onBusy = func() {
		status, _ := f.store.GetStatus()
		if status.EvictableBytes != midBytes || status.EvictedBytes != 0 {
			t.Errorf("status at the busy slice evictable = %d, evicted = %d, want %d and 0", status.EvictableBytes, status.EvictedBytes, midBytes)
		}
	}
	result, err := f.store.Evict(t.Context(), f.remote, 1<<30, f.keep, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !checked || f.moves.busied != 1 || f.moves.moves != 2 {
		t.Fatalf("busy checked = %v, busied = %d, commits = %d", checked, f.moves.busied, f.moves.moves)
	}

	// After the retry the pass matches an undisturbed one.
	want := EvictResult{Evicted: 70, EvictedBytes: f.sizes["held"], Requeued: 5, Refused: 25}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	if got, want := f.queue(t), f.want("missing", "marked", "kept"); !slices.Equal(got, want) {
		t.Fatalf("queue = %q, want %q", got, want)
	}
	status, _ := f.store.GetStatus()
	wantBytes := f.sizes["missing"] + f.sizes["marked"] + f.sizes["kept"]
	if status.EvictableBytes != wantBytes || status.EvictedBytes != f.sizes["held"] {
		t.Fatalf("status evictable = %d, evicted = %d", status.EvictableBytes, status.EvictedBytes)
	}
	f.checkLocal(t)
}
