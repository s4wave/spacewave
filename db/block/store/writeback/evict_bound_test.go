package block_store_writeback

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// fakePacks is a remote of packs for bounded eviction tests. The local index
// it answers LocatePacks from places every block in the first pack that
// holds it, unless index names another, whatever the remote holds;
// VerifyPack answers for the remote, which may have lost, cut short or
// changed a pack.
type fakePacks struct {
	// remote holds the block values.
	remote block.StoreOps
	// packs maps each pack id to its blocks.
	packs map[string][]*block.BlockRef
	// lie maps a pack id to the reason its fetch fails.
	lie map[string]string
	// index maps a block hash to the pack the local index places it in,
	// overriding the first pack that holds it.
	index map[string]string
	// fetches counts VerifyPack calls by pack.
	fetches map[string]int
	// afterVerify runs after a pack verifies, when set.
	afterVerify func(id string)
	// ident maps a pack id to its identity; a pack without one has none.
	ident map[string]string
}

// newFakePacks returns an empty remote.
func newFakePacks(remote block.StoreOps) *fakePacks {
	return &fakePacks{remote: remote, packs: make(map[string][]*block.BlockRef), lie: make(map[string]string), index: make(map[string]string), fetches: make(map[string]int), ident: make(map[string]string)}
}

// packOf returns the pack id the index places h in.
func (f *fakePacks) packOf(h *hash.Hash) (string, int64) {
	if id, ok := f.index[h.MarshalString()]; ok {
		return id, int64(64 * len(f.packs[id]))
	}
	for id, refs := range f.packs {
		var size int64
		found := false
		for _, ref := range refs {
			size += 64
			if ref.GetHash().CompareHash(h) {
				found = true
			}
		}
		if found {
			return id, size
		}
	}
	return "", 0
}

// LocatePacks names each block's pack from the index.
func (f *fakePacks) LocatePacks(_ context.Context, hashes []*hash.Hash) ([]EvictPackLocation, error) {
	out := make([]EvictPackLocation, len(hashes))
	for i, h := range hashes {
		id, size := f.packOf(h)
		out[i] = EvictPackLocation{ID: id, Size: size}
	}
	return out, nil
}

// VerifyPack answers for the remote.
func (f *fakePacks) VerifyPack(ctx context.Context, id string, _ int64) ([]EvictVerifiedValue, string, error) {
	f.fetches[id]++
	if reason := f.lie[id]; reason != "" {
		return nil, reason, nil
	}
	var values []EvictVerifiedValue
	for _, ref := range f.packs[id] {
		data, found, err := f.remote.GetBlock(ctx, ref)
		if err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "missing", nil
		}
		values = append(values, EvictVerifiedValue{Hash: ref.GetHash(), Size: int64(len(data))})
	}
	if f.afterVerify != nil {
		f.afterVerify(id)
	}
	return values, "", nil
}

// PackIdentity names the pack by the test's identity.
func (f *fakePacks) PackIdentity(_ context.Context, id string) (string, error) {
	return f.ident[id], nil
}

// staticBudget returns a budget source with one budget, or none.
func staticBudget(b *EvictBudget) func() (EvictBudget, bool) {
	return func() (EvictBudget, bool) {
		if b == nil {
			return EvictBudget{}, false
		}
		return *b, true
	}
}

// boundEnv is a placed store with uploaded blocks in packs.
type boundEnv struct {
	env   *testEnv
	s     *Store
	packs *fakePacks
	refs  []*block.BlockRef
	size  int64
	// deletes is the remote delete path the bound reports, or "".
	deletes string
}

// newBoundEnv writes n blocks of 11 bytes, uploads them, and places them in
// packs of perPack blocks, named p0, p1 and on.
func newBoundEnv(t *testing.T, n, perPack int) *boundEnv {
	t.Helper()
	ctx := t.Context()
	e := &boundEnv{env: newTestEnv(t)}
	e.s = e.env.newStore(t, true)
	remote := newInmemBlockStore()
	for i := range n {
		data := fmt.Appendf(nil, "block-%05d", i)
		e.size = int64(len(data))
		ref, _, err := e.s.PutBlock(ctx, data, leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		e.refs = append(e.refs, ref)
	}
	drain(t, e.s, remote)
	e.packs = newFakePacks(remote)
	for i, ref := range e.refs {
		id := fmt.Sprintf("p%d", i/perPack)
		e.packs.packs[id] = append(e.packs.packs[id], ref)
	}
	return e
}

// held counts the blocks the local values hold.
func (e *boundEnv) held(t *testing.T) int {
	t.Helper()
	held, err := e.s.values.GetBlockExistsBatch(t.Context(), e.refs)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ok := range held {
		if ok {
			n++
		}
	}
	return n
}

// bound returns the bound of a pass under budget.
func (e *boundEnv) bound(budget *EvictBudget) EvictBound {
	return EvictBound{
		Budget:        staticBudget(budget),
		Verifier:      e.packs,
		RemoteDeletes: func() string { return e.deletes },
	}
}

// evict runs a bounded pass under budget.
func (e *boundEnv) evict(t *testing.T, budget *EvictBudget, maxBytes int64) EvictResult {
	t.Helper()
	result, err := e.s.EvictBounded(t.Context(), e.bound(budget), maxBytes, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestBoundedEvictNoBudget checks that a pass with no valid budget drops
// nothing: no budget, an empty id, a negative size. NoBudgetNoDrop.
func TestBoundedEvictNoBudget(t *testing.T) {
	e := newBoundEnv(t, 8, 4)
	for _, b := range []*EvictBudget{nil, {ID: "", Bytes: 1 << 20}, {ID: "b1", Bytes: -1}} {
		result := e.evict(t, b, 1<<30)
		if result.Evicted != 0 || e.held(t) != len(e.refs) {
			t.Fatalf("budget %+v: pass = %+v, held %d of %d; want nothing dropped", b, result, e.held(t), len(e.refs))
		}
	}
	if len(e.packs.fetches) != 0 {
		t.Fatalf("fetched %v without a budget", e.packs.fetches)
	}
}

// TestBoundedEvictWithinBudget checks that the dropped bytes never pass the
// budget, that a pass in flight stops when it is spent, that the same id
// rewritten larger does not refill it, and that a new id grants anew.
// DroppedWithinBudget and SameIdNoRefill.
func TestBoundedEvictWithinBudget(t *testing.T) {
	e := newBoundEnv(t, 12, 4)
	budget := &EvictBudget{ID: "b1", Bytes: 5 * e.size}
	result := e.evict(t, budget, 1<<30)
	if result.EvictedBytes > budget.Bytes || result.EvictedBytes != 5*e.size {
		t.Fatalf("first pass = %+v, want exactly %d bytes", result, budget.Bytes)
	}
	if got := len(e.refs) - e.held(t); int64(got)*e.size != result.EvictedBytes {
		t.Fatalf("%d values gone, pass reports %d bytes", got, result.EvictedBytes)
	}
	status, _ := e.s.GetStatus()
	if !status.Bound.BudgetKnown || status.Bound.BudgetLeft != 0 {
		t.Fatalf("status bound = %+v, want 0 left", status.Bound)
	}

	// The same id, rewritten with more bytes, drops nothing more.
	budget.Bytes = 100 * e.size
	if result := e.evict(t, budget, 1<<30); result.Evicted != 0 {
		t.Fatalf("same id rewritten: pass = %+v, want nothing", result)
	}

	// A new id grants its bytes.
	if result := e.evict(t, &EvictBudget{ID: "b2", Bytes: 2 * e.size}, 1<<30); result.EvictedBytes != 2*e.size {
		t.Fatalf("new id: pass = %+v, want %d bytes", result, 2*e.size)
	}
	if got := len(e.refs) - e.held(t); got != 7 {
		t.Fatalf("%d values gone after both budgets, want 7", got)
	}
}

// TestBoundedEvictRemoteDeletes checks that a pass drops nothing and fetches
// nothing while a remote delete path is enabled, names the path, and drops
// again once the path is off. A pack it verified could otherwise be deleted
// by compaction or reclaim after its local values were gone.
func TestBoundedEvictRemoteDeletes(t *testing.T) {
	e := newBoundEnv(t, 8, 4)
	budget := &EvictBudget{ID: "b1", Bytes: 1 << 20}
	e.deletes = "compaction"
	result := e.evict(t, budget, 1<<30)
	if result.Evicted != 0 || result.Halted != "compaction" || e.held(t) != len(e.refs) {
		t.Fatalf("pass = %+v, held %d of %d; want nothing dropped, halted on compaction", result, e.held(t), len(e.refs))
	}
	if len(e.packs.fetches) != 0 {
		t.Fatalf("fetched %v while a delete path was enabled", e.packs.fetches)
	}

	// With the path off, the same budget drops.
	e.deletes = ""
	if result := e.evict(t, budget, 1<<30); result.Evicted != len(e.refs) || result.Halted != "" {
		t.Fatalf("pass after the path is off = %+v, want every block dropped", result)
	}
}

// TestBoundedEvictTwoPacks checks a value held in two packs whose local index
// names the second: fetching the first proves nothing about the copy reads
// use, so the value does not drop ahead of its entry, and when the indexed
// pack fails it stays held.
func TestBoundedEvictTwoPacks(t *testing.T) {
	e := newBoundEnv(t, 5, 4)
	b4 := e.refs[4]
	e.packs.packs["p0"] = append(e.packs.packs["p0"], b4)
	e.packs.index[b4.GetHash().MarshalString()] = "p1"
	e.packs.lie["p1"] = "missing"
	result := e.evict(t, &EvictBudget{ID: "b1", Bytes: 1 << 20}, 1<<30)
	held, err := e.s.values.GetBlockExistsBatch(t.Context(), e.refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range held {
		if want := i == 4; ok != want {
			t.Fatalf("block %d held=%v after %+v; want only block 4 held", i, ok, result)
		}
	}
	if e.packs.fetches["p1"] != 1 {
		t.Fatalf("indexed pack fetched %d times, want once", e.packs.fetches["p1"])
	}
}

// sliceValues records the largest tombstone write.
type sliceValues struct {
	block.StoreOps
	largest int
}

// PutBlockBatch records the size of a tombstone batch.
func (v *sliceValues) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if len(entries) != 0 && entries[0].Tombstone {
		v.largest = max(v.largest, len(entries))
	}
	return v.StoreOps.PutBlockBatch(ctx, entries)
}

// TestBoundedEvictSlices checks that a pack holding more droppable values
// than the batch limit is tombstoned in writes of at most the limit, and that
// every value still drops.
func TestBoundedEvictSlices(t *testing.T) {
	const limit = 2
	SetEvictBatch(EvictBatch{MinBlocks: limit, MaxBlocks: limit})
	t.Cleanup(func() { SetEvictBatch(EvictBatch{}) })
	e := newBoundEnv(t, 8, 8)
	values := &sliceValues{StoreOps: e.s.values}
	e.s.values = values
	result := e.evict(t, &EvictBudget{ID: "b1", Bytes: 1 << 20}, 1<<30)
	if result.Evicted != 8 || e.held(t) != 0 {
		t.Fatalf("pass = %+v, held %d; want all 8 dropped", result, e.held(t))
	}
	if values.largest == 0 || values.largest > limit {
		t.Fatalf("largest tombstone write = %d, want 1 to %d", values.largest, limit)
	}
}

// crashValues fails the drop write after storing it, as a crash between the
// tombstones and the queue commit leaves the store.
type crashValues struct {
	block.StoreOps
	crash bool
}

// PutBlockBatch stores the batch, then fails it once when crash is set.
func (v *crashValues) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	out, err := v.StoreOps.PutBlockBatch(ctx, entries)
	if err == nil && v.crash {
		v.crash = false
		return nil, errors.New("crash after tombstones")
	}
	return out, err
}

// TestBoundedEvictCrashAtBudget checks that a crash between the tombstones
// and the queue commit, at the budget's boundary, never lets the next pass
// under the same id drop past the budget: the reservation committed before
// the tombstones. BudgetDurable.
func TestBoundedEvictCrashAtBudget(t *testing.T) {
	e := newBoundEnv(t, 8, 4)
	budget := &EvictBudget{ID: "b1", Bytes: 4 * e.size}
	values := &crashValues{StoreOps: e.s.values, crash: true}
	e.s.values = values
	if _, err := e.s.EvictBounded(t.Context(), e.bound(budget), 1<<30, nil, nil); err == nil {
		t.Fatal("pass did not fail on the crash")
	}
	crashed := len(e.refs) - e.held(t)

	// Reopen over the same metadata and evict again under the same id.
	e.s = e.env.newStore(t, true)
	e.evict(t, budget, 1<<30)
	if gone := len(e.refs) - e.held(t); int64(gone)*e.size > budget.Bytes {
		t.Fatalf("%d values gone (%d at the crash), past the budget of %d bytes", gone, crashed, budget.Bytes)
	}
}

// TestBoundedEvictRemoteLies checks that a pack the remote lost, cut short,
// or holds with other bytes drops nothing, while the local index says it
// holds every block, that the failure is counted under its label, and that
// the pass moves on to the next pack and fetches the bad one once.
// DropVerified.
func TestBoundedEvictRemoteLies(t *testing.T) {
	for _, reason := range []string{"missing", "short", "hash", "index", "oversize"} {
		t.Run(reason, func(t *testing.T) {
			e := newBoundEnv(t, 8, 4)
			e.packs.lie["p0"] = reason
			result := e.evict(t, &EvictBudget{ID: "b1", Bytes: 1 << 20}, 1<<30)
			held, err := e.s.values.GetBlockExistsBatch(t.Context(), e.refs)
			if err != nil {
				t.Fatal(err)
			}
			for i, ok := range held {
				if bad := i < 4; bad != ok {
					t.Fatalf("block %d of pack p%d held=%v after %+v", i, i/4, ok, result)
				}
			}
			if e.packs.fetches["p0"] != 1 {
				t.Fatalf("bad pack fetched %d times in the pass", e.packs.fetches["p0"])
			}
			status, _ := e.s.GetStatus()
			if status.Bound.Refused[reason] != 4 {
				t.Fatalf("refused = %v, want 4 under %q", status.Bound.Refused, reason)
			}
		})
	}
}

// TestBoundedEvictKeepsLastingRefusal checks that a pack refused for a
// lasting reason is not fetched again while its identity stays, that its
// entries are still refused and counted, that a changed identity fetches it
// again and lets it verify, which deletes the kept row, and that a fetch
// failure or a pack with no identity is fetched on every pass.
func TestBoundedEvictKeepsLastingRefusal(t *testing.T) {
	cases := []struct {
		reason, ident string
		kept          bool
	}{{"hash", "a", true}, {"entry", "a", true}, {"fetch", "a", false}, {"hash", "", false}}
	for _, c := range cases {
		t.Run(c.reason+"/"+c.ident, func(t *testing.T) {
			ctx := t.Context()
			e := newBoundEnv(t, 8, 4)
			e.packs.lie["p0"], e.packs.ident["p0"] = c.reason, c.ident
			budget := &EvictBudget{ID: "b1", Bytes: 1 << 20}
			e.evict(t, budget, 1<<30)
			e.evict(t, budget, 1<<30)
			want := 2
			if c.kept {
				want = 1
			}
			status, _ := e.s.GetStatus()
			if e.packs.fetches["p0"] != want || e.held(t) != 4 || status.Bound.Refused[c.reason] != 8 {
				t.Fatalf("two passes: %d fetches, want %d; %d held; refused %v", e.packs.fetches["p0"], want, e.held(t), status.Bound.Refused)
			}
			if !c.kept {
				return
			}

			// The bucket's objects change and the pack verifies.
			delete(e.packs.lie, "p0")
			e.packs.ident["p0"] = "b"
			e.evict(t, budget, 1<<30)
			_, found, err := e.s.keptRefusal(ctx, "p0", "b")
			if err != nil || found || e.packs.fetches["p0"] != 2 || e.held(t) != 0 {
				t.Fatalf("after the change: row %v, %v; %d fetches; %d held", found, err, e.packs.fetches["p0"], e.held(t))
			}
		})
	}
}

// TestBoundedEvictStaleEntry checks that a pass drops a pack's values ahead
// of their queue entries, deletes those entries and subtracts their sizes in
// the same commit, and that a later pass finds nothing left to walk.
func TestBoundedEvictStaleEntry(t *testing.T) {
	e := newBoundEnv(t, 4, 4)
	budget := &EvictBudget{ID: "b1", Bytes: 1 << 20}

	// A pass whose target is one block takes one entry and drops the pack.
	result := e.evict(t, budget, 1)
	if result.Evicted != 4 || e.held(t) != 0 {
		t.Fatalf("pass = %+v, held %d; want the whole pack", result, e.held(t))
	}
	status, _ := e.s.GetStatus()
	if status.EvictableBytes != 0 || status.Bound.DroppedAhead != 3 || e.queued(t) != 0 {
		t.Fatalf("evictable = %d, dropped ahead = %d, queued %d; want 0, 3 and 0", status.EvictableBytes, status.Bound.DroppedAhead, e.queued(t))
	}
	left := status.Bound.BudgetLeft

	// The next pass finds nothing to walk.
	result = e.evict(t, budget, 1<<30)
	status, _ = e.s.GetStatus()
	if result.Evicted != 0 || result.EvictedBytes != 0 || status.Bound.BudgetLeft != left || status.EvictableBytes != 0 {
		t.Fatalf("second pass = %+v, left %d, evictable %d; want nothing, %d left, 0", result, status.Bound.BudgetLeft, status.EvictableBytes, left)
	}
}

// TestBoundedEvictAheadWithoutEntry checks that a value of a verified pack
// with no queue entry, as one held from before the queue, stays held, and
// that the pass leaves evictable-bytes the summed size of the queue's
// entries, here none.
func TestBoundedEvictAheadWithoutEntry(t *testing.T) {
	e := newBoundEnv(t, 4, 4)
	e.unqueue(t, e.refs[3])
	budget := &EvictBudget{ID: "b1", Bytes: 1 << 20}

	// A pass over the first entry drops the pack's queued values only.
	result := e.evict(t, budget, 1)
	if result.Evicted != 3 || e.held(t) != 1 {
		t.Fatalf("pass = %+v, held %d; want three dropped and one held", result, e.held(t))
	}
	held, err := e.s.values.GetBlockExistsBatch(t.Context(), e.refs[3:])
	if err != nil || !held[0] {
		t.Fatalf("the value without an entry was dropped: %v", err)
	}
	status, _ := e.s.GetStatus()
	if status.EvictableBytes != 0 || status.Bound.DroppedAhead != 2 {
		t.Fatalf("evictable = %d, dropped ahead = %d; want 0 and 2", status.EvictableBytes, status.Bound.DroppedAhead)
	}
	var stored int64
	err = kvtx.RunTransaction(t.Context(), false, e.s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		stored, err = readEvictableBytes(ctx, tx)
		return err
	})
	if err != nil || stored != 0 {
		t.Fatalf("stored evictable bytes = %d, %v; want 0", stored, err)
	}
}

// queued counts the eviction queue's entries.
func (e *boundEnv) queued(t *testing.T) int {
	t.Helper()
	n := 0
	err := kvtx.RunTransaction(t.Context(), false, e.s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
		n = 0
		return scanEvictable(ctx, tx, nil, func(evictEntry) error {
			n++
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// unqueue deletes ref's eviction queue entries and their bytes, leaving the
// value held with no entry.
func (e *boundEnv) unqueue(t *testing.T, ref *block.BlockRef) {
	t.Helper()
	var delta int64
	err := kvtx.RunTransaction(t.Context(), true, e.s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
		var keys [][]byte
		delta = 0
		err := scanEvictable(ctx, tx, nil, func(entry evictEntry) error {
			if entry.hash.CompareHash(ref.GetHash()) {
				keys = append(keys, entry.key)
				delta -= entry.size
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return errors.New("no queue entry to delete")
		}
		for _, key := range keys {
			if err := tx.Delete(ctx, key); err != nil {
				return err
			}
		}
		return addEvictableBytes(ctx, tx, delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	e.s.addEvictStatus(delta, 0)
}

// putRaceValues puts a block again just before the drop write, as a writer
// racing the pass does after its markers were read.
type putRaceValues struct {
	block.StoreOps
	race func()
}

// PutBlockBatch runs race once before a tombstone batch.
func (v *putRaceValues) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if race := v.race; race != nil && len(entries) != 0 && entries[0].Tombstone {
		v.race = nil
		race()
	}
	return v.StoreOps.PutBlockBatch(ctx, entries)
}

// TestBoundedEvictPutRacesDrop checks a put of a block landing between the
// verify and the tombstone: the value drops, its remote copy was verified
// from this batch's bytes; the upload finds the value gone, clears the
// marker and queues nothing; a read fills it from the remote.
func TestBoundedEvictPutRacesDrop(t *testing.T) {
	ctx := t.Context()
	e := newBoundEnv(t, 1, 1)
	ref := e.refs[0]
	data, _, err := e.packs.remote.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	values := &putRaceValues{StoreOps: e.s.values}
	values.race = func() {
		if _, _, err := e.s.PutBlock(ctx, data, leafOpts); err != nil {
			t.Error(err)
		}
	}
	e.s.values = values
	if result := e.evict(t, &EvictBudget{ID: "b1", Bytes: 1 << 20}, 1<<30); result.Evicted != 1 {
		t.Fatalf("pass = %+v, want the block dropped", result)
	}
	if !e.env.isMarked(t, ref) {
		t.Fatal("the racing put left no marker")
	}

	// The upload finds the value gone: it clears the marker, queues nothing.
	drain(t, e.s, e.packs.remote)
	if e.env.isMarked(t, ref) {
		t.Fatal("marker left after the upload")
	}
	status, _ := e.s.GetStatus()
	if status.Pending != 0 || status.EvictableBytes != 0 {
		t.Fatalf("after the upload status = %+v, want nothing pending or queued", status)
	}

	// A read fills it from the remote.
	stored, err := e.packs.remote.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		t.Fatalf("remote read: %v", err)
	}
	e.s.Refill(ref, stored)
	waitStatus(t, e.s, func(st Status) bool { return st.Refills == 1 })
	if e.held(t) != 1 {
		t.Fatal("refill not stored")
	}
}
