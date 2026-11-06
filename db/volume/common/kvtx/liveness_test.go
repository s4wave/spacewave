package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// testClock is a clock a test moves by hand.
type testClock struct {
	// mtx guards now.
	mtx sync.Mutex
	// now is the time the clock reads.
	now time.Time
}

// Now returns the clock's time.
func (c *testClock) Now() time.Time {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.now
}

// Add moves the clock forward by d.
func (c *testClock) Add(d time.Duration) {
	c.mtx.Lock()
	c.now = c.now.Add(d)
	c.mtx.Unlock()
}

// newLivenessVolume opens an empty Volume, adopted from birth, with the
// collector's frees set and a clock the test moves.
func newLivenessVolume(t *testing.T, frees bool) (*Volume, *testClock) {
	t.Helper()
	return openLivenessVolume(t, store_kvtx_inmem.NewStore(), frees)
}

// openLivenessVolume opens a Volume over store, as newLivenessVolume does.
func openLivenessVolume(t *testing.T, store *store_kvtx_inmem.Store, frees bool) (*Volume, *testClock) {
	t.Helper()
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v, err := NewVolume(t.Context(), "test/liveness", store_kvkey.NewDefaultKVKey(), store, &store_kvtx.Config{CollectorFrees: frees}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v.clock = clk.Now
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	return v, clk
}

// openUnadoptedVolume opens a Volume over store, which predates adoption,
// with the collector's frees on and no background adoption, so the test
// drives the adoption itself.
func openUnadoptedVolume(t *testing.T, store *store_kvtx_inmem.Store) (*Volume, *testClock) {
	t.Helper()
	v, clk := openLivenessVolume(t, store, false)
	v.collector = v.newCollector(true, 0)
	return v, clk
}

// testTree is a set of blocks a test builds bottom up, each named.
type testTree map[string]*block.PutBatchEntry

// add builds the block name naming the blocks children name, which add
// built before, and returns its ref.
func (tr testTree) add(t *testing.T, name string, children ...string) *block.BlockRef {
	t.Helper()
	refs := make([]*block.BlockRef, 0, len(children))
	data := []byte("block " + name)
	for _, child := range children {
		refs = append(refs, tr[child].Ref)
		data = append(data, tr[child].Ref.MarshalString()...)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	tr[name] = &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	return ref
}

// entries returns the blocks names name.
func (tr testTree) entries(names ...string) []*block.PutBatchEntry {
	out := make([]*block.PutBatchEntry, len(names))
	for i, name := range names {
		out[i] = tr[name]
	}
	return out
}

// collectAll runs collection passes until one reports no more work.
func collectAll(t *testing.T, v *Volume) block_gc.CollectResult {
	t.Helper()
	var total block_gc.CollectResult
	for range 100 {
		res, err := v.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		total.Netted += res.Netted
		total.Freed += res.Freed
		total.Kept += res.Kept
		total.Leaked += res.Leaked
		total.Lapsed += res.Lapsed
		total.Underflows += res.Underflows
		if !res.More {
			return total
		}
	}
	t.Fatal("collection never finished")
	return total
}

// requireStored checks that each named block of tr is stored or gone.
func requireStored(t *testing.T, v *Volume, tr testTree, want bool, names ...string) {
	t.Helper()
	for _, name := range names {
		_, found, err := v.GetBlock(t.Context(), tr[name].Ref)
		if err != nil {
			t.Fatal(err)
		}
		if found != want {
			t.Fatalf("block %s stored=%v, want %v", name, found, want)
		}
	}
}

// requireAudit checks that an audit finds no live block queued and leaked
// blocks.
func requireAudit(t *testing.T, v *Volume, leaked uint64) {
	t.Helper()
	res, err := v.Audit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.LiveQueued != 0 || res.Leaked != leaked || res.Undercount != 0 || res.Unlisted != 0 {
		t.Fatalf("audit live queued %d (sample %v) leaked %d undercount %d (sample %v) unlisted %d, want 0, %d, 0 and 0",
			res.LiveQueued, res.LiveQueuedSample, res.Leaked, res.Undercount, res.UndercountSample, res.Unlisted, leaked)
	}
}

// TestLivenessRunThrough stores a tree, swaps the root to a second tree that
// shares a subtree, collects, and checks that the grace period keeps the old
// tree, that afterwards only what the live root reaches is stored, and that a
// hole under a dead page frees nothing below it while the audit reports it.
func TestLivenessRunThrough(t *testing.T) {
	// Store R1 -> C -> {E, F}, F -> G under main.
	v, clk := newLivenessVolume(t, true)
	ctx := t.Context()
	tr := testTree{}
	tr.add(t, "G")
	tr.add(t, "F", "G")
	tr.add(t, "E")
	tr.add(t, "C", "E", "F")
	r1 := tr.add(t, "R1", "C")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("G", "F", "E", "C", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
		t.Fatal(err)
	}

	// Move main to R2 -> C2 -> {F}, sharing F.
	tr.add(t, "C2", "F")
	r2 := tr.add(t, "R2", "C2")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("C2", "R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
		t.Fatal(err)
	}

	// Inside the grace period nothing is freed.
	if res := collectAll(t, v); res.Freed != 0 {
		t.Fatalf("freed %d inside the grace period", res.Freed)
	}
	requireStored(t, v, tr, true, "R1", "C", "E", "F", "G", "R2", "C2")

	// After it, the old tree's own blocks go and the shared subtree stays.
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 3 || res.Underflows != 0 {
		t.Fatalf("freed %d with %d underflows, want 3 and none", res.Freed, res.Underflows)
	}
	requireStored(t, v, tr, false, "R1", "C", "E")
	requireStored(t, v, tr, true, "R2", "C2", "F", "G")
	requireAudit(t, v, 0)

	// Store R3 -> D -> {H} under side, then lose D with its child list.
	tr.add(t, "H")
	tr.add(t, "D", "H")
	r3 := tr.add(t, "R3", "D")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "D", "R3"), []block.NamedRoot{{Name: "side", Ref: r3}}); err != nil {
		t.Fatal(err)
	}
	punchHole(t, v, tr["D"].Ref)

	// Releasing side frees R3, and the cascade stops at the hole: H stays,
	// and the audit reports it leaked.
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "side", Previous: r3}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 1 || res.Leaked != 1 {
		t.Fatalf("freed %d leaked %d, want 1 and 1", res.Freed, res.Leaked)
	}
	requireStored(t, v, tr, false, "R3")
	requireStored(t, v, tr, true, "H", "R2", "C2", "F", "G")
	requireAudit(t, v, 1)

	// A released chain frees whole in one pass, though its cascade queues
	// children on both sides of the walk's position.
	names := []string{"L0"}
	tr.add(t, "L0")
	for i := 1; i < 32; i++ {
		names = append(names, "L"+strconv.Itoa(i))
		tr.add(t, names[i], names[i-1])
	}
	top := tr[names[len(names)-1]].Ref
	if err := v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: "chain", Ref: top}}); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "chain", Previous: top}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	res, err := v.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Freed != uint64(len(names)) || res.More {
		t.Fatalf("one pass freed %d of the %d-block chain, more %v", res.Freed, len(names), res.More)
	}
	requireStored(t, v, tr, false, names...)
}

// punchHole loses ref from the store with its child list and count, as
// damage to the store or a partial copy does.
func punchHole(t *testing.T, v *Volume, ref *block.BlockRef) {
	t.Helper()
	err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
		if err := p.rec.DeleteChildren(t.Context(), ref); err != nil {
			return err
		}
		if err := p.rec.SetCount(t.Context(), ref, 0); err != nil {
			return err
		}
		return p.blocks.RmBlock(t.Context(), ref)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLivenessAdversarial tries to make the collector free a live block: a
// reader pinning an old root during a swap and a reader whose lease lapses,
// two writers moving one root, a root moved away and back within the grace
// period, a release and a birth that share a child in different intervals,
// and a Volume that predates adoption. It frees no live block in any.
func TestLivenessAdversarial(t *testing.T) {
	t.Run("reader pin", func(t *testing.T) {
		// Store R1 -> E under main and pin it as a reader that saw it.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		unpin, err := v.PinBucketRoot(ctx, r1)
		if err != nil {
			t.Fatal(err)
		}

		// Swap main away; the pin, renewed by its live holder, keeps the old
		// tree past the grace period.
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// The pin released, the old tree goes after the grace period.
		unpin()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("lapsed lease", func(t *testing.T) {
		// A pin whose process died lapses; the collector releases it, and
		// frees the old tree only a grace period later.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PinBucketRoot(ctx, r1); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		forgetPins(t, v)
		clk.Add(rootLease + time.Second)
		if res := collectAll(t, v); res.Lapsed != 1 || res.Freed != 0 {
			t.Fatalf("lapsed %d freed %d, want 1 and 0", res.Lapsed, res.Freed)
		}
		requireStored(t, v, tr, true, "R1")
		clk.Add(block_gc.DefaultGrace)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("freed %d after the lapsed pin's grace, want 1", res.Freed)
		}
	})

	t.Run("two writers", func(t *testing.T) {
		// Writer B moves main from R1 while writer A still expects R1.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		r3 := tr.add(t, "R3")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}

		// A loses: its change is refused whole and stores nothing.
		err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r1}})
		if !errors.Is(err, block.ErrRootMoved) {
			t.Fatalf("stale move = %v, want ErrRootMoved", err)
		}
		requireStored(t, v, tr, false, "R3")

		// The winner's root survives collection; the old root goes.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R2")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)
	})

	t.Run("moved away and back", func(t *testing.T) {
		// main moves R1 to R2 and back to R1 inside the grace period; a pass
		// that nets only the first move must not free R1.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace / 2)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r1, Previous: r2}}); err != nil {
			t.Fatal(err)
		}

		// Net only the first move: R1 is queued and kept by the later record.
		clk.Add(block_gc.DefaultGrace/2 + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Kept == 0 {
			t.Fatalf("freed %d kept %d, want 0 freed and R1 kept", res.Freed, res.Kept)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// Once the move back nets, R2 goes and R1's tree stays.
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E")
		requireStored(t, v, tr, false, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("release and birth in different intervals", func(t *testing.T) {
		// R1 -> C -> F loses main; just inside its grace period a new R2
		// names F under another root. Netting the release alone frees R1 and
		// C, and F, whose new holder is in a later record, stays.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "F")
		tr.add(t, "C", "F")
		r1 := tr.add(t, "R1", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("F", "C", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		r2 := tr.add(t, "R2", "F")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "other", Ref: r2}}); err != nil {
			t.Fatal(err)
		}

		// Net the release only.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "C")
		requireStored(t, v, tr, true, "F", "R2")

		// Once the birth nets, F keeps its new holder.
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "F", "R2")
		requireAudit(t, v, 0)
	})

	t.Run("re-put of a queued block", func(t *testing.T) {
		// R -> X loses main, and just inside its grace period a writer puts X
		// again, as a writer that puts one tree across several batches does
		// for a block it already stored. Netting the release frees R and
		// queues X, which the re-put keeps until it ages, so a parent the
		// writer puts later still finds X.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}

		// Net the release only.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R")
		requireStored(t, v, tr, true, "X")

		// The writer's parent names X under main, and X stays.
		p := tr.add(t, "P", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "X")
		requireAudit(t, v, 0)
	})

	t.Run("shared child in one batch", func(t *testing.T) {
		// R -> {A, B}, A -> X, B -> X are born in one batch, and side holds
		// A too. X has two parents born beside it, so it is counted twice.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		a := tr.add(t, "A", "X")
		tr.add(t, "B", "X")
		r := tr.add(t, "R", "A", "B")
		roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "side", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "A", "B", "R"), roots); err != nil {
			t.Fatal(err)
		}

		// Releasing main frees R and B; A and X stay under side.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "B")
		requireStored(t, v, tr, true, "A", "X")
		requireAudit(t, v, 0)

		// Releasing side frees the rest.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "side", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "A", "X")
	})

	t.Run("stage", func(t *testing.T) {
		// A stage builds C, then P -> C; main adopts P, and the stage's
		// release leaves P's tree to main. A second build the stage releases
		// unadopted goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		tr.add(t, "Q")
		for _, name := range []string{"C", "P", "Q"} {
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}

		// Inside the stage's life nothing is freed; after its release and
		// the grace period only the unadopted build goes.
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P", "Q")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P")
		requireStored(t, v, tr, false, "Q")
		requireAudit(t, v, 0)
	})

	t.Run("underflow in one free slice", func(t *testing.T) {
		// A -> B under main, with B also queued as if a holder had vanished
		// from its count. Releasing main queues A, and one slice takes A and
		// then B: freeing A takes B off the queue as an underflow, keeping it
		// with one holder, and the slice must not then free B.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		for i := 0; ; i++ {
			child := "U" + strconv.Itoa(i)
			tr.add(t, child)
			tr.add(t, "A", child)
			if bytes.Compare(block_gc.BlockKey(tr["A"].Ref), block_gc.BlockKey(tr[child].Ref)) < 0 {
				tr["B"] = tr[child]
				break
			}
		}
		a := tr["A"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("B", "A"), []block.NamedRoot{{Name: "main", Ref: a}}); err != nil {
			t.Fatal(err)
		}
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			return p.rec.Enqueue(ctx, tr["B"].Ref)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Underflows != 1 {
			t.Fatalf("collection found %d underflows, want 1", res.Underflows)
		}
		requireStored(t, v, tr, false, "A")
		requireStored(t, v, tr, true, "B")
	})

	t.Run("read fills a tree main holds", func(t *testing.T) {
		// D is stored under old. A read fills B, which names C, which names
		// D; main moves onto B without putting its tree; a later read fills
		// C; old is released. D keeps the holder C's fill gave it, and the
		// fills are garbage only once nothing holds them.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		d := tr.add(t, "D")
		tr.add(t, "C", "D")
		b := tr.add(t, "B", "C")
		fill := func(name string) {
			t.Helper()
			err := v.withDirectFill(ctx, func(p *physicalTx) error {
				_, err := block_gc.LandFill(ctx, p.rec, p.blocks, v.now(), tr.entries(name))
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("D"), []block.NamedRoot{{Name: "old", Ref: d}}); err != nil {
			t.Fatal(err)
		}
		fill("B")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
			t.Fatal(err)
		}
		fill("C")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "old", Previous: d}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "B", "C", "D")
		requireAudit(t, v, 0)

		// Releasing main frees the whole tree.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "B", "C", "D")
	})

	t.Run("unheld fill", func(t *testing.T) {
		// A read fills E, which nothing holds: it stays for the grace period
		// and then goes, as an unheld put does.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		err := v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := block_gc.LandFill(ctx, p.rec, p.blocks, v.now(), tr.entries("E"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		collectAll(t, v)
		requireStored(t, v, tr, true, "E")
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "E")
	})

	t.Run("stage lease lapses while open", func(t *testing.T) {
		// A stage holds T, and its process stalls past the lease: a collector
		// that does not renew, as another process's does, releases the
		// stage's root. The next renewal holds it again, so after the grace
		// period a parent the stage's writer puts still finds T.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "T")
		if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
			t.Fatal(err)
		}
		clk.Add(rootLease + time.Second)
		if res, err := v.collector.Collect(ctx); err != nil || res.Lapsed != 1 {
			t.Fatalf("stalled pass lapsed %d err %v, want the stage root", res.Lapsed, err)
		}
		if _, err := v.renewRoots(ctx); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "T")

		// The parent names T under main, the stage lets it go, and T stays.
		p := tr.add(t, "P", "T")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		release()
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "T")
		requireAudit(t, v, 0)
	})

	for _, renew := range []bool{false, true} {
		t.Run("stage stalled past the grace period renew="+strconv.FormatBool(renew), func(t *testing.T) {
			// A stage holds T, and its process stalls past the lease and
			// the grace period: a collector that does not renew releases
			// the stage's root and frees T. The stage's next put names T,
			// and it is refused, as is any put after a renewal that finds T
			// gone, rather than landing S over a freed block.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			tr := testTree{}
			tr.add(t, "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
				t.Fatal(err)
			}
			clk.Add(rootLease + time.Second)
			if _, err := v.collector.Collect(ctx); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			for range 3 {
				if _, err := v.collector.Collect(ctx); err != nil {
					t.Fatal(err)
				}
			}
			requireStored(t, v, tr, false, "T")
			if renew {
				if _, err := v.renewRoots(ctx); err != nil {
					t.Fatal(err)
				}
			}
			tr.add(t, "S", "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); !errors.Is(err, block.ErrStageReleased) {
				t.Fatalf("put over a freed stage top = %v, want the stage lost", err)
			}
			requireStored(t, v, tr, false, "S")
		})
	}

	for _, audited := range []bool{true, false} {
		t.Run("hole refilled by a stage audited="+strconv.FormatBool(audited), func(t *testing.T) {
			// main -> R -> H, and the store loses H with its child list. A
			// stage then puts H again and drops it. Once an audit recorded
			// the hole, the put counts H as stored under R, so H stays.
			// Refilled before any audit, H counts as a new block under the
			// stage alone, and the audit reports the undercount.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			h := tr.add(t, "H")
			r := tr.add(t, "R", "H")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
				t.Fatal(err)
			}
			punchHole(t, v, h)
			if audited {
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
				}
			}
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("H")); err != nil {
				t.Fatal(err)
			}
			release()
			if err := v.withDirectAtomic(ctx, nil); err != nil {
				t.Fatal(err)
			}
			if !audited {
				if res, err := v.Audit(ctx); err != nil || res.Undercount != 1 {
					t.Fatalf("audit of an unrecorded refill found %d undercounts err %v, want 1", res.Undercount, err)
				}
				return
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, "R", "H")
			requireAudit(t, v, 0)
		})
	}

	t.Run("cache fill during an adoption", func(t *testing.T) {
		// main holds C on an unadopted Volume. While an adoption settles its
		// counts, after it counted the child lists, a cache fill stores F
		// naming C, with a fill record that holds C and takes F's implied
		// holder back. The adoption treats F as born and leaves it to its
		// record. A root r then moves onto F: whichever of the journal and
		// the fill record nets first, F stays while r holds it, and once r is
		// released F goes and C stays under main.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "main", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		f := tr.add(t, "F", "C")
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, _, err := v.PrepareOwnedBlock(ctx, "", tr["F"].Data, &block.PutOpts{Refs: tr["F"].Refs, RefsKnown: true, CacheFill: true}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Ref: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "F")
		requireAudit(t, v, 0)

		// Released, F goes and C stays.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Previous: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "F")
		requireStored(t, v, tr, true, "C")
		requireAudit(t, v, 0)
	})

	t.Run("background adoption", func(t *testing.T) {
		// An unadopted Volume holding main -> R -> X and an unheld G opens
		// with frees on and adopts itself in the background: R and X stay
		// and G goes after the grace period.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v1, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v1.PutBlockBatch(ctx, tr.entries("G")); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Reopen with frees on and wait for the adoption.
		v, clk := openLivenessVolume(t, store, true)
		if v.adoptDone == nil {
			t.Fatal("an unadopted Volume opened with frees on started no adoption")
		}
		<-v.adoptDone
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("cache fill of a held block during an adoption", func(t *testing.T) {
		// main holds P on an unadopted Volume, and P names B, which the
		// Volume lacks. While an adoption runs, before it settles, a read
		// fills B. The adoption counts P's holder of B and leaves B to its
		// fill record, so after the grace period B keeps P's holder.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "B")
		p := tr.add(t, "P", "B")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				err := v.withDirectFill(ctx, func(px *physicalTx) error {
					_, err := block_gc.LandFill(ctx, px.rec, px.blocks, v.now(), tr.entries("B"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "B")
		requireAudit(t, v, 0)
	})

	t.Run("record larger than a slice", func(t *testing.T) {
		// A under root a names 5000 leaves. B, born under root b, names the
		// same stored leaves, so its one record carries more up pairs than a
		// net slice applies and splits across slices. Releasing a frees only
		// A; releasing b frees B and every leaf.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 5000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		a := tr.add(t, "A", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "A")...), []block.NamedRoot{{Name: "a", Ref: a}}); err != nil {
			t.Fatal(err)
		}
		b := tr.add(t, "B", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("B"), []block.NamedRoot{{Name: "b", Ref: b}, {Name: "a", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "A")
		requireStored(t, v, tr, true, append(slices.Clone(leaves), "B")...)
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "b", Previous: b}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "B")...)
		requireAudit(t, v, 0)
	})

	t.Run("child named twice", func(t *testing.T) {
		// P names the stored C twice and an empty ref. Its list keeps C once,
		// so releasing P frees both, and the audit finds nothing.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		p := tr.add(t, "P", "C", "C")
		tr["P"].Refs = append(tr["P"].Refs, &block.BlockRef{})
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "p", Ref: p}, {Name: "c", Previous: c}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "p", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("bytes without a list", func(t *testing.T) {
		// Bytes stored below the Volume's liveness, without a child list,
		// are garbage nothing frees; the audit reports them.
		v, _ := newLivenessVolume(t, true)
		ctx := t.Context()
		if _, _, err := v.Store.PutBlock(ctx, []byte("stored without a list"), &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
		if res, err := v.Audit(ctx); err != nil || res.Unlisted != 1 {
			t.Fatalf("audit found %d unlisted blocks err %v, want 1", res.Unlisted, err)
		}
	})

	t.Run("abandoned write", func(t *testing.T) {
		// A writer puts P -> C in two batches, child first, and never lands
		// a root on them. Once the grace period passes both are garbage.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		tr.add(t, "P", "C")
		for _, name := range []string{"C", "P"} {
			if _, err := v.PutBlockBatch(ctx, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent put before its child", func(t *testing.T) {
		// A buffered drain puts P under main before its child C, and C lands
		// alone within the grace period. Both stay, and releasing main frees
		// both.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent collected before its child is stored", func(t *testing.T) {
		// A session puts R2 under main before its child C2, journaling an up
		// for C2. Main lets R2 go and the collector frees it; the cascade
		// takes C2's holder although C2 has no list yet. C2 then lands alone
		// and nothing holds it, so it goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C2")
		r2 := tr.add(t, "R2", "C2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2")
		if _, err := v.PutBlockBatch(ctx, tr.entries("C2")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "C2")
		requireAudit(t, v, 0)
	})

	for _, pinned := range []bool{true, false} {
		t.Run("long staged session over an abandoned base pinned="+strconv.FormatBool(pinned), func(t *testing.T) {
			// main holds B -> X. A staged session observes B, pinning it as
			// the World engine's StageWorldState does. Main abandons B, and
			// the session outlasts the grace period before it puts S naming
			// X without putting X again and lands S under main. With the pin
			// X stays; without it X is freed before S names it.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "X")
			b := tr.add(t, "B", "X")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "B"), []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
				t.Fatal(err)
			}
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if pinned {
				unpin, err := v.PinBucketRoot(ctx, b)
				if err != nil {
					t.Fatal(err)
				}
				defer unpin()
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
				t.Fatal(err)
			}
			advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
			collectAll(t, v)
			s := tr.add(t, "S", "X")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: s}}); err != nil {
				t.Fatal(err)
			}
			release()
			requireStored(t, v, tr, pinned, "X")
		})
	}

	t.Run("frees off", func(t *testing.T) {
		// With frees off the collector nets and frees nothing.
		v, clk := newLivenessVolume(t, false)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Netted == 0 {
			t.Fatalf("freed %d netted %d, want 0 freed and the journal netted", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1")
	})

	t.Run("unadopted", func(t *testing.T) {
		// A Volume that predates adoption journals and frees nothing.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}

		// Pk under root pk names K, which no put has stored yet.
		k := tr.add(t, "K")
		pk := tr.add(t, "Pk", "K")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("Pk"), []block.NamedRoot{{Name: "pk", Ref: pk}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Netted != 0 {
			t.Fatalf("unadopted pass freed %d netted %d, want nothing", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2", "Pk")

		// An adoption interrupted while it settles the counts, with main
		// moving onto a new R3 naming E while it counts, leaves the Volume
		// unadopted.
		r3 := tr.add(t, "R3", "E")
		probe := &adoptProbe{Store: collectorStore{v: v}}
		probeCtx, cancel := context.WithCancel(ctx)
		probe.at = map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r2}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: cancel,
		}
		if _, err := block_gc.Adopt(probeCtx, probe); !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("interrupted adopt = %v with %d writes left, want canceled after both", err, len(probe.at))
		}
		if adoption := livenessAdoption(t, v); adoption != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("interrupted adopt left %v, want unadopted", adoption)
		}
		// It leaves its counts for the next claim to clear, so a Close that
		// cancels it waits for one transaction.

		// A rerun counts every holder once while main moves onto a new R4
		// naming E before it counts the roots; a root aux moves onto R1 and
		// a put stores K under a root k before it counts the child lists; and
		// a put stores Y under a root late before it settles. The next pass
		// frees R2 and R3, and keeps Y, born after the rerun began, and K,
		// which Pk named before it began.
		r4 := tr.add(t, "R4", "E")
		y := tr.add(t, "Y")
		probe = &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R4"), []block.NamedRoot{{Name: "main", Ref: r4, Previous: r3}}); err != nil {
					t.Error(err)
				}
			},
			adoptCountChildren: func() {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "aux", Ref: r1}}); err != nil {
					t.Error(err)
				}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("K"), []block.NamedRoot{{Name: "k", Ref: k}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("Y"), []block.NamedRoot{{Name: "late", Ref: y}}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("rerun adopt = %v with %d writes left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2", "R3")
		requireStored(t, v, tr, true, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)

		// K keeps Pk's holder once root k lets it go.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Previous: k}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "Pk", "K")
		requireAudit(t, v, 0)

		// A fresh adoption of the same Volume reaches the same counts and
		// queue.
		netted := livenessRecords(t, store, 'H', 'Q')
		markUnadopted(t, v)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if again := livenessRecords(t, store, 'H', 'Q'); !maps.Equal(again, netted) {
			t.Fatalf("readoption counts %v, want %v", again, netted)
		}

		// Releasing every root frees everything.
		releases := []block.NamedRoot{{Name: "main", Previous: r4}, {Name: "aux", Previous: r1}, {Name: "late", Previous: y}, {Name: "pk", Previous: pk}}
		if err := v.SetBucketRoots(ctx, "b", nil, releases); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)
	})

	t.Run("incomplete format 6 list", func(t *testing.T) {
		// A format 6 store holds R -> X under a named root, but R's child
		// list lacks X, as a crash between two slices of the old graph's
		// edge batch left it. Adopting would count no holder of X and free
		// it under the root, so the upgraded Volume refuses adoption and
		// frees nothing.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		writeLegacyStore(t, store, keys, tr, "R", true, "X", "R")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(r)...), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnverified) {
			t.Fatalf("adopt of an incomplete list = %v, want ErrAdoptUnverified", err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")

		// A rebuild that decodes X but not R leaves R's list unverified,
		// reports it, and Adopt keeps refusing.
		rebuild := func(names ...string) block_gc.ListRebuildResult {
			t.Helper()
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range names {
				if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
					t.Fatal(err)
				}
			}
			res, err := rb.Finish(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		if res := rebuild("X"); res.Unverified != 1 || !res.UnverifiedSample[0].EqualsRef(r) || res.Cleared {
			t.Fatalf("rebuild without R = %+v, want R unverified", res)
		}
		if _, err := v.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnverified) {
			t.Fatalf("adopt after a partial rebuild = %v, want ErrAdoptUnverified", err)
		}

		// A rerun that decodes R rewrites its list and skips X, which the
		// first run verified, but its walk from the roots is interrupted
		// after its first pass marked what it reached.
		probeCtx, cancel := context.WithCancel(ctx)
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){rebuildSecondPass: cancel}}
		rb, err := block_gc.OpenListRebuild(probeCtx, probe)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"X", "R"} {
			if err := rb.Verify(probeCtx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if res, err := rb.Finish(probeCtx); !errors.Is(err, context.Canceled) || res.Verified != 1 || res.Rewritten != 1 {
			t.Fatalf("interrupted rebuild = %+v err %v, want R rewritten and the walk canceled", res, err)
		}
		reached := append(slices.Clone(livenessPrefix), 'K')
		if countPrefix(t, store, reached) == 0 {
			t.Fatal("interrupted walk left no reached marks")
		}

		// Running it again deletes the interrupted walk's marks, verifies
		// nothing more, and clears the mark: Adopt succeeds and X stays.
		if res := rebuild(); res.Verified != 0 || res.Unverified != 0 || !res.Cleared {
			t.Fatalf("rerun rebuild = %+v, want the mark cleared", res)
		}
		if n := countPrefix(t, store, reached); n != 0 {
			t.Fatalf("finished walk left %d reached marks", n)
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})

	t.Run("adoption claimed by another", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A clears, then B,
		// another process, starts while the Volume is still unadopted. A
		// marks the Volume adopting and counts the roots; B clears A's
		// counts and dies before it marks. A must not settle the cleared
		// counts: B's claim superseded it, and nothing main reaches is freed.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// B claims, waits before its first clear, clears on release, and dies
		// before it marks.
		claimed, cleared, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		releaseB := sync.OnceFunc(func() { close(release) })
		defer releaseB()
		bCtx, bCancel := context.WithCancel(ctx)
		defer bCancel()
		probeB := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptClearCounts: func() { close(claimed); <-release },
			adoptMark:        bCancel,
		}}
		go func() {
			defer close(cleared)
			_, _ = block_gc.Adopt(bCtx, probeB)
		}()

		// A starts B before its mark and lets B clear before it settles.
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptMark: func() { <-claimed },
			adoptSettle: func() {
				releaseB()
				<-cleared
			},
		}}
		_, err := block_gc.Adopt(ctx, probeA)
		releaseB()
		<-cleared
		if !errors.Is(err, block_gc.ErrAdoptSuperseded) {
			t.Fatalf("claimed adoption = %v, want superseded", err)
		}

		// Nothing is freed, and a clean adoption keeps main's tree.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})

	t.Run("overlapping adoptions", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A counts the
		// holders, and before it settles them a second adoption B runs to
		// the end. A must not settle B's counts again: it refuses, B's
		// adoption stands, and nothing main reaches is freed.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, err := block_gc.Adopt(ctx, collectorStore{v: v}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); !errors.Is(err, block_gc.ErrAdoptSuperseded) || len(probe.at) != 0 {
			t.Fatalf("superseded adopt = %v with %d adoptions left, want superseded", err, len(probe.at))
		}
		if adoption := livenessAdoption(t, v); adoption != block_gc.Adoption_ADOPTION_ADOPTED {
			t.Fatalf("overlapping adoptions left %v, want adopted", adoption)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})
}

// rebuildSecondPass is the transaction of a list rebuild's Finish, on a
// Volume whose records fit in one slice, that starts the walk's second pass:
// after the flush, the clear of earlier marks and the first pass.
const rebuildSecondPass = 4

// The transactions of block_gc.Adopt, in order, on a Volume whose records
// each fit in one slice.
const (
	// adoptClearCounts clears the count records an earlier run left.
	adoptClearCounts = 2
	// adoptMark marks the Volume adopting.
	adoptMark = 6
	// adoptCountRoots counts the roots.
	adoptCountRoots = 7
	// adoptCountChildren counts the child lists' holders.
	adoptCountChildren = 8
	// adoptSettle settles the stored blocks' counts.
	adoptSettle = 9
)

// adoptProbe runs an adoption's transactions, calling the func at a
// transaction's position, counted from one, before it runs.
type adoptProbe struct {
	block_gc.Store
	// at maps a transaction's position to the func called before it.
	at map[int]func()
	// n counts the transactions.
	n int
}

// Update calls the func at the transaction's position, then runs it.
func (p *adoptProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	p.n++
	if f := p.at[p.n]; f != nil {
		delete(p.at, p.n)
		f()
	}
	return p.Store.Update(ctx, fn)
}

// livenessAdoption returns the Volume's adoption state.
func livenessAdoption(t *testing.T, v *Volume) block_gc.Adoption {
	t.Helper()
	var adoption block_gc.Adoption
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		state, err := rec.GetState(t.Context())
		adoption = state.GetAdoption()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return adoption
}

// markListsVerified clears the Volume's unverified child lists mark, as a
// repair that rebuilt the lists from the blocks does.
func markListsVerified(t *testing.T, v *Volume) {
	t.Helper()
	err := collectorStore{v: v}.Update(t.Context(), func(rec *block_gc.Records, _ block.StoreOps) error {
		state, err := rec.GetState(t.Context())
		if err != nil {
			return err
		}
		state.UnverifiedLists = false
		return rec.SetState(t.Context(), state)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// markUnadopted returns an adopted Volume to unadopted, as one that predates
// adoption, keeping its records.
func markUnadopted(t *testing.T, v *Volume) {
	t.Helper()
	err := collectorStore{v: v}.Update(t.Context(), func(rec *block_gc.Records, _ block.StoreOps) error {
		state, err := rec.GetState(t.Context())
		if err != nil {
			return err
		}
		state.Adoption = block_gc.Adoption_ADOPTION_UNADOPTED
		return rec.SetState(t.Context(), state)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// livenessRecords returns the liveness records of the kinds in store, by key.
func livenessRecords(t *testing.T, store *store_kvtx_inmem.Store, kinds ...byte) map[string]string {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	out := make(map[string]string)
	for _, kind := range kinds {
		err := tx.ScanPrefix(t.Context(), append(slices.Clone(livenessPrefix), kind), func(key, value []byte) error {
			out[string(key)] = string(value)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// forgetPins drops this process's record of its reader pins, as a process
// that died leaves them: nothing renews or releases their roots.
func forgetPins(t *testing.T, v *Volume) {
	t.Helper()
	unlock, err := v.rootPinMu.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	clear(v.rootPins)
	unlock()
}

// advanceRenewing moves the clock forward by d in steps shorter than the
// lease, renewing this process's leased roots after each, as its renewal
// loop does.
func advanceRenewing(t *testing.T, v *Volume, clk *testClock, d time.Duration) {
	t.Helper()
	for d > 0 {
		step := min(d, rootRenewInterval)
		clk.Add(step)
		d -= step
		if _, err := v.renewRoots(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

// setUnadopted marks store, which must be empty, as a Volume of the current
// format that predates adoption.
func setUnadopted(t *testing.T, store *store_kvtx_inmem.Store) {
	t.Helper()
	keys := store_kvkey.NewDefaultKVKey()
	state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED}
	if err := writeFormat(t.Context(), store, keys, state); err != nil {
		t.Fatal(err)
	}
}

// TestFormatUpgradeCarriesLiveness opens a store of format 6 and checks that
// format 7 keeps its child lists in place, carries its named roots into the
// root table, leaves it unadopted, sweeps the rest of the old graph, and that
// an adoption then counts its holders. A second store whose root names a block
// stored without a child list refuses adoption.
func TestFormatUpgradeCarriesLiveness(t *testing.T) {
	// Write a format 6 store: R1 -> C, its child lists, a root owner edge
	// holding R1, and old graph records.
	ctx := t.Context()
	keys := store_kvkey.NewDefaultKVKey()
	store := store_kvtx_inmem.NewStore()
	tr := testTree{}
	tr.add(t, "C")
	r1 := tr.add(t, "R1", "C")
	writeLegacyStore(t, store, keys, tr, "R1", true, "C", "R1")

	// Open it: the lists and the root carry over, and it is unadopted.
	v, _ := openUnadoptedVolume(t, store)
	if v.legacyDone != nil {
		<-v.legacyDone
	}
	if got, err := v.GetBucketRoot(ctx, "space", "world"); err != nil || !got.EqualsRef(r1) {
		t.Fatalf("carried root = %v err %v, want R1", got, err)
	}
	if children, listed, err := v.GetChildren(ctx, r1); err != nil || !listed || len(children) != 1 || !children[0].EqualsRef(tr["C"].Ref) {
		t.Fatalf("carried child list = %v listed %v err %v", children, listed, err)
	}
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED || state.GetLegacyGraph() || !state.GetUnverifiedLists() {
			t.Fatalf("state %v, want unadopted, unverified, with the old graph swept", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range legacyGraphKinds {
		if n := countPrefix(t, store, append([]byte{livenessPrefix[0]}, kind)); n != 0 {
			t.Fatalf("%d old graph records of kind %q remain", n, kind)
		}
	}

	// Adoption refuses the unverified lists, and once a repair verifies them
	// counts the holders: nothing is garbage.
	if _, err := v.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnverified) {
		t.Fatalf("adopt of unverified lists = %v, want ErrAdoptUnverified", err)
	}
	markListsVerified(t, v)
	res, err := v.Adopt(ctx)
	if err != nil || res.Blocks != 2 || res.Roots != 1 || res.Queued != 0 {
		t.Fatalf("adopt = %+v err %v, want 2 blocks, 1 root, none queued", res, err)
	}
	requireAudit(t, v, 0)

	// A store whose root names a block stored without a child list refuses
	// adoption and stays unadopted.
	store2 := store_kvtx_inmem.NewStore()
	writeLegacyStore(t, store2, keys, tr, "R1", false)
	v2, _ := openUnadoptedVolume(t, store2)
	markListsVerified(t, v2)
	if _, err := v2.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnknown) {
		t.Fatalf("adopt of a listless root = %v, want ErrAdoptUnknown", err)
	}
	if res, err := v2.Collect(ctx); err != nil || res.Netted != 0 {
		t.Fatalf("collect after a refused adoption netted %d err %v", res.Netted, err)
	}

	// A format 6 store opened with frees on starts no background adoption:
	// its lists are unverified until a repair, and an adoption would refuse.
	store4 := store_kvtx_inmem.NewStore()
	writeLegacyStore(t, store4, keys, tr, "R1", true, "C", "R1")
	v4, _ := openLivenessVolume(t, store4, true)
	if v4.adoptDone != nil {
		t.Fatal("a Volume with unverified lists started a background adoption")
	}

	// An adopted format 7 store where main holds B, which a fill stored
	// naming C with a fill mark and no holder for C, opens unadopted, and its
	// adoption counts C under B and deletes the mark.
	store3 := store_kvtx_inmem.NewStore()
	tr3 := testTree{}
	tr3.add(t, "C")
	b := tr3.add(t, "B", "C")
	writeFormat7Fill(t, store3, keys, tr3, b)
	v3, clk3 := openUnadoptedVolume(t, store3)
	err = v3.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err == nil && state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("format 7 state %v, want unadopted", state)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := v3.Adopt(ctx); err != nil || res.Blocks != 2 || res.Queued != 0 {
		t.Fatalf("adopt of format 7 = %+v err %v, want 2 blocks, none queued", res, err)
	}
	if n := countPrefix(t, store3, []byte{livenessPrefix[0], 'F'}); n != 0 {
		t.Fatalf("%d fill marks remain after the adoption", n)
	}
	clk3.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v3)
	requireStored(t, v3, tr3, true, "B", "C")
	requireAudit(t, v3, 0)
}

// writeFormat7Fill writes an adopted format 7 store holding the blocks of tr
// with their child lists, a named root main of bucket b on root, and a fill
// mark on root, as a format 7 fill left a block whose children it gave no
// holder.
func writeFormat7Fill(t *testing.T, store *store_kvtx_inmem.Store, keys *store_kvkey.KVKey, tr testTree, root *block.BlockRef) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Write the blocks, their lists, the root, the mark and the state.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), entry.Data); err != nil {
			t.Fatal(err)
		}
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.SetRoot(ctx, "b", "main", &block_gc.RootEntry{Ref: block_gc.BlockKey(root)}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'F'}, block_gc.BlockKey(root)...), nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_ADOPTED}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(7)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// writeLegacyStore writes a format 6 store holding the blocks of tr, a named
// root world of bucket space on root, the child lists of listed when lists is
// set, and a few old graph records of other kinds.
func writeLegacyStore(t *testing.T, store *store_kvtx_inmem.Store, keys *store_kvkey.KVKey, tr testTree, root string, lists bool, listed ...string) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	set := func(key, value []byte) {
		if err := tx.Set(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}

	// Write the version, the blocks and their child lists.
	set(keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(6))
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		set(keys.GetBlockKey(key), entry.Data)
	}
	if lists {
		for _, name := range listed {
			key := append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(tr[name].Ref)...)
			set(key, block_gc.EncodeChildren(tr[name].Refs))
		}
	}

	// Write the root owner edge and old records of other kinds.
	owner := legacyRootOwnerPrefix + base64.RawURLEncoding.EncodeToString([]byte("space")) + "/" + base64.RawURLEncoding.EncodeToString([]byte("world"))
	edge := append([]byte{livenessPrefix[0], legacyForwardKind, legacyTextNode}, binary.AppendUvarint(nil, uint64(len(owner)))...)
	edge = append(append(edge, owner...), block_gc.BlockKey(tr[root].Ref)...)
	set(edge, nil)
	set(append([]byte{livenessPrefix[0], 'h'}, block_gc.BlockKey(tr[root].Ref)...), []byte{1})
	set([]byte{livenessPrefix[0], 'w'}, []byte("sharing"))
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// countPrefix counts the keys of store under prefix.
func countPrefix(t *testing.T, store *store_kvtx_inmem.Store, prefix []byte) int {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	err = tx.ScanPrefixKeys(t.Context(), prefix, func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
