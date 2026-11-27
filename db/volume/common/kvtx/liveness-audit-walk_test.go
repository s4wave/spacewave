package kvtx

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// testLivenessAudit checks audits beside concurrent writers and stalled netting.
func testLivenessAudit(t *testing.T) {
	t.Helper()

	t.Run("streaming audit beside writers", func(t *testing.T) {
		testAuditWalk(t)
	})

	t.Run("streaming audit under stalled netting", func(t *testing.T) {
		testAuditWalkStalled(t)
	})

	for _, tc := range []struct {
		name           string
		audited, named bool
	}{
		{"unaudited", false, false},
		{"audited", true, false},
		{"audited and named again", true, true},
	} {
		t.Run("stage dies holding a parent before its child "+tc.name, func(t *testing.T) {
			// An adopted format 6 store, its generation Open, holds
			// the old root A. A live stage holds L. A second stage
			// puts P, which references C, before C, as a buffered
			// store's drain may order them, and its process dies
			// with both unput C and P's lapsing root. An audit
			// while the root lives records C as a hole. Past the
			// lease the collector releases the dead stage's root
			// and keeps L. Unaudited, P frees once the generation
			// settles. Audited, the hole record outlives every
			// block that referenced C and holds the flip: the
			// generation stays Open, P stays parked, and no pass
			// reports why. A commit that references P again after
			// the lapse, and later puts C, refills the hole, keeps
			// both, and lets the generation settle.
			store := newGuardedStore(t)
			tr := testTree{}
			tr.add(t, "A")
			writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, "A", true, "A")
			v, clk := openUnadoptedVolume(t, store)
			ctx := t.Context()
			if _, err := v.Adopt(ctx); err != nil {
				t.Fatal(err)
			}
			if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
				t.Fatalf("the adoption left the generation %v, want it open", gen)
			}
			live, releaseLive, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseLive()
			tr.add(t, "L")
			if _, err := v.PrepareStagedBlockBatch(ctx, live, tr.entries("L")); err != nil {
				t.Fatal(err)
			}

			// The dying stage puts P alone. Its root moves onto a lease of
			// its own, standing for another process, which stops renewing.
			dead, _, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tr.add(t, "C")
			p := tr.add(t, "P", "C")
			if _, err := v.PrepareStagedBlockBatch(ctx, dead, tr.entries("P")); err != nil {
				t.Fatal(err)
			}
			leaseApart(t, v, clk, dead)
			if tc.audited {
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit under the live stage root recorded %d holes err %v, want C", res.Holes, err)
				}
			}

			// Past the lease the dead stage's root lapses, the live one's
			// does not.
			advanceRenewing(t, v, clk, rootLease+time.Second)
			if res := collectAll(t, v); res.Lapsed != 1 {
				t.Fatalf("pass past the lease lapsed %d roots, want the dead stage's", res.Lapsed)
			}
			if tc.named {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
					t.Fatal(err)
				}
			}

			// Settle as far as the store lets it.
			for range 8 {
				if livenessGeneration(t, v) == block_gc.Generation_GENERATION_OPEN {
					walkFront(t, v, store, tr)
				}
				advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
				collectAll(t, v)
			}
			requireReads(t, v, tr, "A", "L")
			gen := livenessGeneration(t, v)
			holes := countPrefix(t, store, slices.Concat(livenessPrefix, []byte{'O'}))
			switch {
			case tc.named:
				requireReads(t, v, tr, "P", "C")
				if gen != block_gc.Generation_GENERATION_SETTLED || holes != 0 {
					t.Fatalf("after the refill the generation is %v with %d hole records, want Settled and none", gen, holes)
				}
			case tc.audited:
				requireStored(t, v, tr, true, "P")
				requireStored(t, v, tr, false, "C")
				if gen != block_gc.Generation_GENERATION_OPEN || holes != 1 {
					t.Fatalf("the generation is %v with %d hole records, want Open, held by C's", gen, holes)
				}
			default:
				requireStored(t, v, tr, false, "P", "C")
				if gen != block_gc.Generation_GENERATION_SETTLED || holes != 0 {
					t.Fatalf("the generation is %v with %d hole records, want Settled and none", gen, holes)
				}
			}
			requireAudit(t, v, 0)
		})
	}
}

// openAuditScratch opens a test walk's SQLite scratch with the audit
// command's disposable scratch settings.
func openAuditScratch(t *testing.T, name string) (store_kvtx_sqlite.Store, error) {
	return store_kvtx_sqlite.OpenWithPragmas(t.Context(), filepath.Join(t.TempDir(), name), "walk", store_kvtx_sqlite.Pragmas{
		Sync:               store_kvtx_sqlite.SyncOff,
		CacheSize:          -2048,
		DisableMmap:        true,
		TempStore:          1,
		WithoutRowID:       true,
		WALCheckpointBytes: 8 << 20,
	})
}

// testAuditWalk runs the streaming audit while a writer moves roots onto new
// trees and collects with frees on. main -> R -> H, and the store loses H with
// its list: a hole a held root reaches. main2 -> R2 -> H2, whose hole a stage
// refilled before any audit recorded it, so H2 is undercounted. A stage stores
// P ahead of its child C, which lands while the walk waits out its confirm
// delay. X is referenced by more roots than one view of a walk reads. The walk
// reports H as unconfirmed and held, H2 as the one undercount, and C as
// settled, and then refuses its scratch store to a second run. A walk of the
// quiet Volume sums X's namers across its views, finds H2 alone undercounted,
// and counts no block wide.
func testAuditWalk(t *testing.T) {
	// Build the hole, the undercount and the parent ahead of its child.
	v, clk := newLivenessVolume(t, true)
	swapCollector(v, v.newCollector(collectorStore{v: v}, true, false, 0))
	ctx := t.Context()
	tr := testTree{}
	h := tr.add(t, "H")
	tr.add(t, "R", "H")
	tr.add(t, "H2")
	tr.add(t, "R2", "H2")
	tr.add(t, "C")
	tr.add(t, "P", "C")
	tr.add(t, "X")
	roots := []block.NamedRoot{{Name: "main", Ref: tr["R"].Ref}, {Name: "main2", Ref: tr["R2"].Ref}}
	for i := range 4100 {
		roots = append(roots, block.NamedRoot{Name: "wide" + strconv.Itoa(i), Ref: tr["X"].Ref})
	}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "R", "H2", "R2", "X"), roots); err != nil {
		t.Fatal(err)
	}
	punchHole(t, v, h)
	punchHole(t, v, tr["H2"].Ref)
	refill, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PrepareStagedBlockBatch(ctx, refill, tr.entries("H2")); err != nil {
		t.Fatal(err)
	}
	release()
	stage, _, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("P")); err != nil {
		t.Fatal(err)
	}

	// Start the walk once the writer has written a root of each name and
	// more than a check's copy of up log entries, so the walk reads the roots
	// in two views with writes between, and its first check sums up log
	// entries past its copy.
	stop, started, errc := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { errc <- auditWalkWriter(ctx, v, stop, started) }()
	<-started

	// Walk, landing C while the walk waits out its delay.
	scratch, err := openAuditScratch(t, "walk.db")
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	wait := func(ctx context.Context, d time.Duration) error {
		if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("C")); err != nil {
			return err
		}
		clk.Add(d)
		return nil
	}
	res, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "first", Wait: wait})
	close(stop)
	if werr := <-errc; werr != nil {
		t.Fatal(werr)
	}
	if err != nil {
		t.Fatal(err)
	}

	// Check the report.
	c := res.Counts
	if c.GetUndercount() != 1 || !slices.ContainsFunc(res.UndercountSample, tr["H2"].Ref.EqualsRef) {
		t.Fatalf("walk found %d undercounts, sample %v, want H2 alone", c.GetUndercount(), res.UndercountSample)
	}
	if c.GetCandidates() != 2 || c.GetSettled() != 1 || c.GetUnconfirmed() != 1 || c.GetUnconfirmedHeld() != 1 {
		t.Fatalf("walk counted %d candidates, %d settled, %d unconfirmed, %d of them held, want 2, 1, 1 and 1",
			c.GetCandidates(), c.GetSettled(), c.GetUnconfirmed(), c.GetUnconfirmedHeld())
	}
	if len(res.UnconfirmedSample) != 1 || !res.UnconfirmedSample[0].Ref.EqualsRef(h) {
		t.Fatalf("walk sampled %v unconfirmed, want H", res.UnconfirmedSample)
	}

	// A second run refuses the first run's scratch store.
	if _, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "second"}); !errors.Is(err, block_gc.ErrWalkTaken) {
		t.Fatalf("second run on the scratch store returned %v, want ErrWalkTaken", err)
	}

	// A walk of the quiet Volume checks the wide X soundly.
	quiet, err := openAuditScratch(t, "quiet.db")
	if err != nil {
		t.Fatal(err)
	}
	defer quiet.Close()
	res, err = v.AuditWalk(ctx, quiet, block_gc.WalkOptions{Run: "quiet", Delay: -1})
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Counts; c.GetUndercount() != 1 || c.GetWide() != 0 || !slices.ContainsFunc(res.UndercountSample, tr["H2"].Ref.EqualsRef) {
		t.Fatalf("quiet walk found %d undercounts, sample %v, and %d wide, want H2 alone and none", c.GetUndercount(), res.UndercountSample, c.GetWide())
	}

	// Between X's two views, a recount moves one root off X and another
	// onto it with no log entry. X's first view counts the edges of wide0
	// to wide995 in name order, the second those of wide996 to wide999.
	// wide999 leaves X before the first view. While the Volume is
	// unadopted, wide0 leaves and wide999 returns. The recount writes X's
	// holders as the first view read them. The mark must move, so X is wide
	// and not undercounted.
	tr.add(t, "Y")
	cycled := &walkHookStore{collectorStore: collectorStore{v: v}}
	cycled.hook = func(prog *block_gc.WalkProgress) error {
		if prog.GetPhase() != block_gc.WalkPhase_WALK_PHASE_CHECK {
			return nil
		}
		switch cycled.step {
		case 0:
			// Move wide999 to Y and net it.
			cycled.step++
			if err := v.SetBucketRoots(ctx, "b", tr.entries("Y"), []block.NamedRoot{{Name: "wide999", Ref: tr["Y"].Ref, Previous: tr["X"].Ref}}); err != nil {
				return err
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		case 1:
			// Past X's first view, swap wide0 and wide999 unadopted, then
			// count again.
			if prog.GetWideSeen() == 0 {
				return nil
			}
			cycled.step++
			frees := livenessState(t, v).GetFrees()
			for _, on := range []bool{!frees, frees} {
				if _, err := block_gc.SetFrees(ctx, collectorStore{v: v}, on); err != nil {
					return err
				}
			}
			roots := []block.NamedRoot{
				{Name: "wide0", Ref: tr["Y"].Ref, Previous: tr["X"].Ref},
				{Name: "wide999", Ref: tr["X"].Ref, Previous: tr["Y"].Ref},
			}
			if err := v.SetBucketRoots(ctx, "b", nil, roots); err != nil {
				return err
			}
			_, err := v.Adopt(ctx)
			return err
		}
		return nil
	}
	cycledScratch, err := openAuditScratch(t, "cycled.db")
	if err != nil {
		t.Fatal(err)
	}
	defer cycledScratch.Close()
	cycled.scratch = cycledScratch
	res, err = block_gc.Walk(ctx, cycled, cycledScratch, block_gc.WalkOptions{Run: "cycled", Delay: -1, Now: v.now})
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Counts; cycled.step != 2 || c.GetWide() != 1 || slices.ContainsFunc(res.UndercountSample, tr["X"].Ref.EqualsRef) {
		t.Fatalf("cycled walk took %d steps, found %d wide and undercount sample %v, want 2, X wide and not undercounted", cycled.step, c.GetWide(), res.UndercountSample)
	}
}

// testAuditWalkStalled references one block S from about 76,000 parents while
// netting stalls, so the up log past the cursor holds more entries of S than
// one check view may read. The walk copies them all into scratch, and every
// view of S then finds more copies than its budget: the walk must stay behind
// rather than read them all in one view.
func testAuditWalkStalled(t *testing.T) {
	// Land S, then batches of 2,000 parents of S under one root each. Every
	// batch after the first logs an up of S per parent.
	v, _ := newLivenessVolume(t, true)
	ctx := t.Context()
	s := []byte("stalled shared leaf")
	sRef, err := block.BuildBlockRef(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for b := range 38 {
		entries := []*block.PutBatchEntry{{Ref: sRef, Data: s, RefsKnown: true}}
		var parents []*block.BlockRef
		for i := range 2000 {
			p := []byte("stalled parent " + strconv.Itoa(b) + " " + strconv.Itoa(i))
			pRef, err := block.BuildBlockRef(p, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: pRef, Data: p, Refs: []*block.BlockRef{sRef}, RefsKnown: true})
			parents = append(parents, pRef)
		}
		g := []byte("stalled grandparent " + strconv.Itoa(b))
		gRef, err := block.BuildBlockRef(g, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: gRef, Data: g, Refs: parents, RefsKnown: true})
		if err := v.SetBucketRoots(ctx, "b", entries, []block.NamedRoot{{Name: "g" + strconv.Itoa(b), Ref: gRef}}); err != nil {
			t.Fatal(err)
		}
	}

	// Walk without netting. The copy of the up log ends within 20 views, so
	// a walk still behind at its 40th wait reads S's copies against the
	// budget; stop it there. Use the audit walk's disposable scratch
	// settings so each check does not flush its scratch writes to disk.
	scratch, err := openAuditScratch(t, "stalled.db")
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	errStalled := errors.New("still behind")
	var waits int
	wait := func(context.Context, time.Duration) error {
		if waits++; waits == 40 {
			return errStalled
		}
		return nil
	}
	res, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "stalled", Delay: -1, Wait: wait})
	if !errors.Is(err, errStalled) {
		t.Fatalf("walk under stalled netting returned %v with %+v after %d waits, want it behind at 40", err, res, waits)
	}
}

// walkHookStore is a walk's view of a Volume that calls hook with the walk's
// progress in scratch before each view.
type walkHookStore struct {
	collectorStore
	// scratch is the walk's scratch store.
	scratch kvtx.Store
	// hook runs before each view.
	hook func(prog *block_gc.WalkProgress) error
	// step is how far the hook has gone.
	step int
}

// View reads the walk's progress, runs the hook, then runs fn in a view.
func (s *walkHookStore) View(ctx context.Context, fn func(rec *block_gc.Records) error) error {
	tx, err := s.scratch.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	value, found, err := tx.Get(ctx, []byte{'p'})
	tx.Discard()
	if err != nil {
		return err
	}
	prog := &block_gc.WalkProgress{}
	if found {
		if err := prog.UnmarshalVT(value); err != nil {
			return err
		}
	}
	if err := s.hook(prog); err != nil {
		return err
	}
	return s.collectorStore.View(ctx, fn)
}

// auditWalkWrites is how many roots auditWalkWriter writes: two up log
// entries each past the first 300, so the 2500 before the walk starts leave
// more than walkSliceEdges entries of up log for the walk's first check.
const auditWalkWrites = 3000

// auditWalkWriter moves roots onto new three-block trees and runs a
// collection pass after each. It cycles through 300 root names, more than
// one view of a walk reads, closes started 500 writes before its last, and
// returns once it has written auditWalkWrites roots and stop closes.
func auditWalkWriter(ctx context.Context, v *Volume, stop, started chan struct{}) error {
	prev := make([]*block.BlockRef, 300)
	for i := 0; ; i++ {
		// Stop when asked.
		select {
		case <-stop:
			return nil
		default:
		}
		switch i {
		case auditWalkWrites - 500:
			close(started)
		case auditWalkWrites:
			<-stop
			return nil
		}

		// Write a leaf, one of eight shared leaves and their parent under the
		// next root name, and collect. Each parent adds an up log entry to
		// its shared leaf, so the check of every slice sums pairs the walk
		// copied in earlier slices and pairs past its last copy.
		leaf := []byte("writer leaf " + strconv.Itoa(i))
		leafRef, err := block.BuildBlockRef(leaf, nil)
		if err != nil {
			return err
		}
		shared := []byte("writer shared " + strconv.Itoa(i%8))
		sharedRef, err := block.BuildBlockRef(shared, nil)
		if err != nil {
			return err
		}
		parent := append([]byte("writer parent "), leafRef.MarshalString()...)
		parentRef, err := block.BuildBlockRef(parent, nil)
		if err != nil {
			return err
		}
		entries := []*block.PutBatchEntry{
			{Ref: leafRef, Data: leaf, RefsKnown: true},
			{Ref: sharedRef, Data: shared, RefsKnown: true},
			{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef, sharedRef}, RefsKnown: true},
		}
		name := i % 300
		root := []block.NamedRoot{{Name: "w" + strconv.Itoa(name), Ref: parentRef, Previous: prev[name]}}
		if err := v.SetBucketRoots(ctx, "w", entries, root); err != nil {
			return err
		}
		prev[name] = parentRef
		if _, err := v.Collect(ctx); err != nil {
			return err
		}
	}
}
