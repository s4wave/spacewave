package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// evictMoveCountStore counts the committed transactions that delete eviction
// queue entries.
type evictMoveCountStore struct {
	kvtx.Store
	// moves is the number of committed queue-changing transactions.
	moves int
}

// NewTransaction wraps the real transaction to count its queue changes.
func (s *evictMoveCountStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &evictMoveCountTx{Tx: tx, store: s}, nil
}

// evictMoveCountTx records whether it deletes an eviction queue entry.
type evictMoveCountTx struct {
	kvtx.Tx
	// store counts the commit.
	store *evictMoveCountStore
	// moved records whether this transaction deletes an eviction entry.
	moved bool
}

// Delete records an eviction queue change while staging the real deletion.
func (t *evictMoveCountTx) Delete(ctx context.Context, key []byte) error {
	t.moved = t.moved || bytes.HasPrefix(key, []byte(evictPrefix))
	return t.Tx.Delete(ctx, key)
}

// Commit counts a committed queue change.
func (t *evictMoveCountTx) Commit(ctx context.Context) error {
	err := t.Tx.Commit(ctx)
	if err == nil && t.moved {
		t.store.moves++
	}
	return err
}

// evictMixKinds are the decision kinds of evictMixScenario, with their counts.
var evictMixKinds = []struct {
	name  string
	count int
}{{"held", 70}, {"missing", 20}, {"marked", 5}, {"kept", 5}, {"gone", 2}}

// evictMixScenario is one batch holding every eviction decision.
type evictMixScenario struct {
	store  *Store
	remote block.StoreOps
	moves  *evictMoveCountStore
	// refs and sizes hold each kind's blocks and summed size, by kind name.
	refs  map[string][]*block.BlockRef
	sizes map[string]int64
	// keep keeps the kept kind's blocks.
	keep func(*hash.Hash) bool
}

// evictMixData returns the data of block i of kind.
func evictMixData(kind string, i int) []byte {
	return fmt.Appendf(nil, "mix-%s-%03d", kind, i)
}

// newEvictMixScenario uploads blocks of every decision kind into one batch of
// more than evictRetryCommitBlocks entries: held blocks the remote confirms,
// missing blocks the remote lost, marked blocks rewritten after upload, kept
// blocks the caller keeps, and gone blocks whose local value is already absent.
func newEvictMixScenario(t *testing.T) *evictMixScenario {
	// Upload every block, queueing its entry at epoch zero.
	ctx := t.Context()
	f := &evictMixScenario{
		store:  newTestEnv(t).newStore(t, true),
		remote: newInmemBlockStore(),
		refs:   make(map[string][]*block.BlockRef),
		sizes:  make(map[string]int64),
	}
	for _, kind := range evictMixKinds {
		for i := range kind.count {
			data := evictMixData(kind.name, i)
			ref, _, err := f.store.PutBlock(ctx, data, leafOpts)
			if err != nil {
				t.Fatal(err)
			}
			f.refs[kind.name] = append(f.refs[kind.name], ref)
			f.sizes[kind.name] += int64(len(data))
		}
	}
	drain(t, f.store, f.remote)

	// Give each kind its cause after the upload queued its entry.
	for _, ref := range f.refs["missing"] {
		if err := f.remote.RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	for i := range f.refs["marked"] {
		if _, _, err := f.store.PutBlock(ctx, evictMixData("marked", i), leafOpts); err != nil {
			t.Fatal(err)
		}
	}
	var gone []*block.PutBatchEntry
	for _, ref := range f.refs["gone"] {
		gone = append(gone, &block.PutBatchEntry{Ref: ref, Tombstone: true})
	}
	if _, err := f.store.values.PutBlockBatch(ctx, gone); err != nil {
		t.Fatal(err)
	}
	kept := make(map[string]bool)
	for _, ref := range f.refs["kept"] {
		kept[ref.GetHash().MarshalString()] = true
	}
	f.keep = func(h *hash.Hash) bool { return kept[h.MarshalString()] }

	// Count the pass's queue commits.
	f.moves = &evictMoveCountStore{Store: f.store.markers}
	f.store.markers = f.moves
	return f
}

// queue returns the eviction queue keys with their sizes, the epoch and the
// evictable byte sum, as one sorted listing of the durable eviction state.
func (f *evictMixScenario) queue(t *testing.T) []string {
	var out []string
	err := kvtx.RunTransaction(t.Context(), false, f.store.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		if err := scanEvictable(ctx, tx, nil, func(entry evictEntry) error {
			out = append(out, fmt.Sprintf("%s=%d", entry.key, entry.size))
			return nil
		}); err != nil {
			return err
		}
		for _, key := range []string{epochKey, evictableBytesKey} {
			value, _, err := tx.Get(ctx, []byte(key))
			if err != nil {
				return err
			}
			out = append(out, key+"="+string(value))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// want returns the listing queue reports after one pass, epoch 1, that queued
// the given kinds there.
func (f *evictMixScenario) want(kinds ...string) []string {
	var out []string
	var total int64
	for _, kind := range kinds {
		for i, ref := range f.refs[kind] {
			out = append(out, fmt.Sprintf("%s=%d", evictKey(1, ref.GetHash()), len(evictMixData(kind, i))))
		}
		total += f.sizes[kind]
	}
	out = append(out, epochKey+"=1", fmt.Sprintf("%s=%d", evictableBytesKey, total))
	slices.Sort(out)
	return out
}

// checkLocal checks that only the held and gone kinds lost their local values.
func (f *evictMixScenario) checkLocal(t *testing.T) {
	for kind, want := range map[string]bool{"held": false, "missing": true, "marked": true, "kept": true, "gone": false} {
		for _, ref := range f.refs[kind] {
			if _, found, err := f.store.GetBlock(t.Context(), ref); err != nil || found != want {
				t.Fatalf("%s local value found = %v, %v, want %v", kind, found, err, want)
			}
		}
	}
}

// TestEvictRetryOffKeepsPinBehavior checks a pass with refusal retry off, the
// default, against the eviction state and counts the evictor wrote before
// refusal retry existed: refused entries leave the queue, only the caller's
// kept entries move to the new epoch and Requeued counts them, and the
// batch's queue changes commit in one transaction. This file uses nothing
// refusal retry added, so the same test runs against the evictor before it.
func TestEvictRetryOffKeepsPinBehavior(t *testing.T) {
	f := newEvictMixScenario(t)
	result, err := f.store.Evict(t.Context(), f.remote, 1<<30, f.keep, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := EvictResult{Evicted: 70, EvictedBytes: f.sizes["held"], Requeued: 5, Refused: 25}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	if got, want := f.queue(t), f.want("kept"); !slices.Equal(got, want) {
		t.Fatalf("queue = %q, want %q", got, want)
	}
	if f.moves.moves != 1 {
		t.Fatalf("queue commits = %d, want one for the batch", f.moves.moves)
	}
	status, _ := f.store.GetStatus()
	if status.EvictableBytes != f.sizes["kept"] || status.EvictedBytes != f.sizes["held"] {
		t.Fatalf("status evictable = %d, evicted = %d", status.EvictableBytes, status.EvictedBytes)
	}
	f.checkLocal(t)
}
