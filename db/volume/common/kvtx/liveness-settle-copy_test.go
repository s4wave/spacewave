package kvtx

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// testLivenessSettleCopy checks copied blocks, omitted child references, and reopening.
func testLivenessSettleCopy(t *testing.T,
	oldStore func(t *testing.T, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock, kvtx.Store),
	has func(t *testing.T, store kvtx.Store, prefix []byte, ref *block.BlockRef) bool,
	fill func(t *testing.T, v *Volume, tr testTree, names ...string),
	rootOnto func(t *testing.T, v *Volume, tr testTree, name string, put ...string) error,
	verify func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult,
	collectUntil func(t *testing.T, v *Volume, clk *testClock, done func(block_gc.CollectResult) bool) block_gc.CollectResult,
	flipped func(res block_gc.CollectResult) bool,
	settled func(res block_gc.CollectResult) bool,
	oldU func(t *testing.T, cut bool) (*Volume, *testClock, kvtx.Store, testTree),
	passes func(t *testing.T, v *Volume, clk *testClock, n int),
	run []byte,
	front []byte,
) {
	t.Helper()

	// copied returns name's entry as a copier builds it from a store that
	// recorded refs, which are listed and so not asserted unless the trust
	// switch is set.
	// requireDropped requires err to name exactly the dropped fills at.
	requireDropped := func(t *testing.T, err error, at ...int) {
		t.Helper()
		var dropped *block.FillsDroppedError
		if !errors.As(err, &dropped) || !slices.Equal(dropped.At, at) {
			t.Fatalf("fill batch err %v, want the fills at %v dropped", err, at)
		}
	}
	copied := func(tr testTree, name string, refs ...*block.BlockRef) *block.PutBatchEntry {
		src := block.StoredBlock{Data: tr[name].Data, Refs: refs, RefsSource: block.RefsListed}
		return &block.PutBatchEntry{Ref: tr[name].Ref, Data: src.Data, Refs: src.Refs, RefsKnown: src.RefsSource.Asserted()}
	}
	// trusting sets the switch that takes listed refs as asserted for the
	// rest of t.
	trusting := func(t *testing.T) {
		prev := block.SetTrustListedRefs(true)
		t.Cleanup(func() { block.SetTrustListedRefs(prev) })
	}

	// Each path a copy writes by takes a block P that names C with a copied
	// list lacking C, into an adopted format 6 store holding C unreached.
	// The walk decodes P and keeps C. With the trust switch the copy marks
	// P's list as it read, and the sweep deletes C, which P still names.
	mainOn := func(t *testing.T, v *Volume, tr testTree, name string) error {
		return v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Ref: tr[name].Ref}})
	}
	copyPaths := []struct {
		name string
		put  func(t *testing.T, v *Volume, tr testTree, p *block.PutBatchEntry) error
	}{
		{"roots", func(t *testing.T, v *Volume, tr testTree, p *block.PutBatchEntry) error {
			return v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{p}, []block.NamedRoot{{Name: "main", Ref: p.Ref}})
		}},
		{"batch", func(t *testing.T, v *Volume, tr testTree, p *block.PutBatchEntry) error {
			if _, err := v.PutBlockBatch(t.Context(), []*block.PutBatchEntry{p}); err != nil {
				return err
			}
			return mainOn(t, v, tr, "P")
		}},
		{"owned", func(t *testing.T, v *Volume, tr testTree, p *block.PutBatchEntry) error {
			if _, err := v.PrepareOwnedBlockBatch(t.Context(), "b", []*block.PutBatchEntry{p}); err != nil {
				return err
			}
			return mainOn(t, v, tr, "P")
		}},
		{"staged", func(t *testing.T, v *Volume, tr testTree, p *block.PutBatchEntry) error {
			stage, release, err := v.OpenStage(t.Context())
			if err != nil {
				return err
			}
			defer release()
			if _, err := v.PrepareStagedBlockBatch(t.Context(), stage, []*block.PutBatchEntry{p}); err != nil {
				return err
			}
			return mainOn(t, v, tr, "P")
		}},
	}
	for _, path := range copyPaths {
		for _, trust := range []bool{false, true} {
			t.Run("copied list lacking a child via "+path.name+" trust="+strconv.FormatBool(trust), func(t *testing.T) {
				if trust {
					trusting(t)
				}
				tr := testTree{}
				tr.add(t, "C")
				tr.add(t, "R0")
				v, clk, store := oldStore(t, tr, "R0", []string{"C", "R0"})
				tr.add(t, "P", "C")
				if err := path.put(t, v, tr, copied(tr, "P")); err != nil {
					t.Fatal(err)
				}
				if !trust && (!has(t, store, run, tr["P"].Ref) || !has(t, store, front, tr["P"].Ref)) {
					t.Fatal("the copied P left the run or the front")
				}
				settleAll(t, v, clk, store, tr)
				if trust {
					requireStored(t, v, tr, false, "C")
					return
				}
				requireReads(t, v, tr, "P", "C")
			})
		}
	}

	t.Run("copied fill of a block not listed", func(t *testing.T) {
		// A fill drops the copied P, whose list lacks C, and counts it. A
		// root onto P is then refused, as onto any block not stored, and C
		// stays until the walk finds it unreached.
		tr := testTree{}
		tr.add(t, "C")
		tr.add(t, "R0")
		v, _, _ := oldStore(t, tr, "R0", []string{"C", "R0"})
		tr.add(t, "P", "C")
		p := copied(tr, "P")
		p.CacheFill = true
		_, err := v.PrepareOwnedBlockBatch(t.Context(), "b", []*block.PutBatchEntry{p})
		requireDropped(t, err, 0)
		requireStored(t, v, tr, false, "P")
		requireStored(t, v, tr, true, "C")
		if fill := putFlow(t, v, "fill"); fill.Dropped != 1 || fill.Blocks != 0 {
			t.Fatalf("fill flow = %+v, want P dropped and nothing stored", fill)
		}
	})

	t.Run("mixed fill batch", func(t *testing.T) {
		// A fill batch stores its asserted entry and drops its copied one.
		v, _ := newLivenessVolume(t, true)
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "B")
		a, b := *tr["A"], copied(tr, "B")
		a.CacheFill, b.CacheFill = true, true
		_, err := v.PrepareOwnedBlockBatch(t.Context(), "b", []*block.PutBatchEntry{&a, b})
		requireDropped(t, err, 1)
		requireStored(t, v, tr, true, "A")
		requireStored(t, v, tr, false, "B")
		if fill := putFlow(t, v, "fill"); fill.Blocks != 1 || fill.Dropped != 1 || fill.DroppedHoles != 0 {
			t.Fatalf("fill flow = %+v, want A stored and B dropped", fill)
		}
	})

	t.Run("copied put of a stored unverified block while Open", func(t *testing.T) {
		// U's list lacks K. A copied put of U lacking K too leaves the list
		// unmarked, so the run build names U and the walk decodes it and
		// keeps K.
		v, clk, store, tr := oldU(t, true)
		if err := v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{copied(tr, "U")}, []block.NamedRoot{{Name: "main", Ref: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		if _, src, err := v.GetChildList(t.Context(), tr["U"].Ref); err != nil || src != block.RefsListed {
			t.Fatalf("U's list after the copied put = %v err %v, want listed", src, err)
		}
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "U", "K")
		requireAudit(t, v, 0)
	})

	// requireReopened checks that the copied put of name moved the Volume
	// back to Open with the block in the run and on the front, and that the
	// Volume no longer assumes Settled.
	requireReopened := func(t *testing.T, v *Volume, store kvtx.Store, tr testTree, name string) {
		t.Helper()
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("the generation after the copied put of %s is %v, want Open", name, gen)
		}
		if !has(t, store, run, tr[name].Ref) || !has(t, store, front, tr[name].Ref) {
			t.Fatalf("the reopening put left %s off the run or the front", name)
		}
		if settledWord(v.settled.load()) {
			t.Fatal("the reopened Volume still assumes Settled")
		}
	}

	t.Run("copied put while Sweeping", func(t *testing.T) {
		// Once the generation flips, no walk is left to check a copied
		// list: a foreground copied put of a new block is refused at once,
		// and a background one waits for the sweep to settle, then reopens
		// the Volume, and the walk decodes it.
		v, clk, store, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		tr.add(t, "N")
		put := func(ctx context.Context) error {
			return v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{copied(tr, "N")}, []block.NamedRoot{{Name: "n", Ref: tr["N"].Ref}})
		}
		if err := put(t.Context()); !errors.Is(err, block_gc.ErrSweeping) {
			t.Fatalf("foreground copied put while Sweeping err %v, want ErrSweeping at once", err)
		}
		release := v.KeepCollecting()
		defer release()
		ctx, cancel := context.WithTimeout(block.WithBackground(t.Context()), 50*time.Millisecond)
		defer cancel()
		if err := put(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("background copied put while Sweeping err %v, want it to wait", err)
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SWEEPING {
			t.Fatalf("the generation after the refused and waiting puts is %v, want Sweeping", gen)
		}
		requireStored(t, v, tr, false, "N")

		// The background put lands once the sweep settles.
		done := make(chan error, 1)
		go func() { done <- put(block.WithBackground(t.Context())) }()
		collectUntil(t, v, clk, settled)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the copied put still waits after the sweep settled")
		}
		requireReopened(t, v, store, tr, "N")
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "N")
		requireAudit(t, v, 0)
	})

	t.Run("background copied put while Sweeping and no pass will come", func(t *testing.T) {
		// A background copied put waits for the sweep only while a caller
		// keeps collecting, since otherwise no pass ends the sweep: with no
		// keeper it is refused at once, and a put waiting when the last
		// keeper releases wakes and is refused.
		v, clk, _, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		tr.add(t, "N")
		put := func(ctx context.Context) error {
			return v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{copied(tr, "N")}, []block.NamedRoot{{Name: "n", Ref: tr["N"].Ref}})
		}
		bg := block.WithBackground(t.Context())
		if err := put(bg); !errors.Is(err, block_gc.ErrSweeping) {
			t.Fatalf("background copied put with no keeper err %v, want ErrSweeping at once", err)
		}

		// A put waiting under a keeper is refused once the keeper leaves.
		release := v.KeepCollecting()
		done := make(chan error, 1)
		go func() { done <- put(bg) }()
		select {
		case err := <-done:
			t.Fatalf("background copied put under a keeper returned %v, want it to wait", err)
		case <-time.After(50 * time.Millisecond):
		}
		release()
		release()
		select {
		case err := <-done:
			if !errors.Is(err, block_gc.ErrSweeping) {
				t.Fatalf("background copied put after the release err %v, want ErrSweeping", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the copied put still waits after the last keeper released")
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SWEEPING {
			t.Fatalf("the generation after the refused puts is %v, want Sweeping", gen)
		}
		requireStored(t, v, tr, false, "N")
	})

	t.Run("sweep resumes across passes", func(t *testing.T) {
		// A sweep of a run larger than one pass resumes where the last pass
		// stopped. Mid-sweep a put from content gives birth again to an
		// unmarked block ahead of the cursor and to a swept one behind it,
		// and a copied put of an unmarked block is refused: the sweep keeps
		// both born blocks, deletes every other one, and drops the whole
		// run. A copied put of N, whose key sorts before the old run's,
		// then reopens the Volume, and the next sweep starts at the head of
		// the new run, so a cursor kept from the old one cannot skip N.
		tr := testTree{}
		tr.add(t, "A")
		var names []string
		for i := range 12 {
			name := "L" + strconv.Itoa(i)
			tr.add(t, name)
			names = append(names, name)
		}
		v, clk, store := oldStore(t, tr, "A", append([]string{"A"}, names...))
		verify(t, v, tr, "A")
		ctx := t.Context()
		key := func(name string) []byte { return block_gc.BlockKey(tr[name].Ref) }
		slices.SortFunc(names, func(a, b string) int { return bytes.Compare(key(a), key(b)) })

		// c's every slice outlasts the pass's budget and the slice target,
		// so each pass runs one slice and the pace shrinks the slices to
		// their least, two run entries a sweep slice.
		var n time.Duration
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{
			Frees:  true,
			Budget: time.Nanosecond,
			Now: func() time.Time {
				n++
				return clk.Now().Add(n * 3 * time.Millisecond)
			},
		})
		var total block_gc.CollectResult
		until := func(stop func(block_gc.CollectResult) bool) {
			t.Helper()
			for range 10000 {
				clk.Add(block_gc.DefaultGrace + time.Second)
				res, err := c.Collect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				total.Swept += res.Swept
				if stop(res) {
					return
				}
			}
			t.Fatalf("collection never reached the state wanted; the generation is %v", livenessGeneration(t, v))
		}
		until(flipped)
		until(func(res block_gc.CollectResult) bool { return total.Swept >= 2 })
		if total.Swept >= uint64(len(names)-2) || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SWEEPING {
			t.Fatalf("first sweep passes swept %d of %d, want part", total.Swept, len(names))
		}

		// Put the first block, swept, and the last, unmarked, from content,
		// and refuse a copied put of one between.
		behind, ahead, between := names[0], names[len(names)-1], names[len(names)-2]
		roots := []block.NamedRoot{{Name: "behind", Ref: tr[behind].Ref}, {Name: "ahead", Ref: tr[ahead].Ref}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(behind, ahead), roots); err != nil {
			t.Fatal(err)
		}
		err := v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{copied(tr, between)}, []block.NamedRoot{{Name: "between", Ref: tr[between].Ref}})
		if !errors.Is(err, block_gc.ErrSweeping) {
			t.Fatalf("copied put of the unmarked %s err %v, want ErrSweeping", between, err)
		}
		total.Swept = 0
		until(settled)
		requireStored(t, v, tr, true, "A", behind, ahead)
		requireStored(t, v, tr, false, names[1:len(names)-1]...)
		if has(t, store, run, nil) {
			t.Fatal("the drop left run entries")
		}

		// Reopen with a copied N that sorts first, then release it: the
		// next sweep deletes it.
		var name string
		for i := 0; name == ""; i++ {
			if i == 1000 {
				t.Fatal("no candidate key sorts before the old run's")
			}
			cand := "N" + strconv.Itoa(i)
			tr.add(t, cand)
			if bytes.Compare(key(cand), key(names[0])) < 0 {
				name = cand
			}
		}
		if err := v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{copied(tr, name)}, []block.NamedRoot{{Name: "n", Ref: tr[name].Ref}}); err != nil {
			t.Fatal(err)
		}
		requireReopened(t, v, store, tr, name)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "n", Previous: tr[name].Ref}}); err != nil {
			t.Fatal(err)
		}
		until(settled)
		requireStored(t, v, tr, false, name)
		if has(t, store, run, nil) {
			t.Fatal("the second drop left run entries")
		}
		requireAudit(t, v, 0)
	})

	t.Run("reopen by a copied list lacking a child the sweep freed", func(t *testing.T) {
		// The sweep frees the unreached U -> K. A copied put of U lacking K
		// reopens the Volume. The walk decodes U and pushes the absent K,
		// which no read resolves, so the Volume stays Open and frees
		// nothing while U holds K; a put of K from content lets it settle.
		v, clk, store, tr := oldU(t, false)
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, false, "U", "K")
		if err := v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{copied(tr, "U")}, []block.NamedRoot{{Name: "u", Ref: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		requireReopened(t, v, store, tr, "U")
		verify(t, v, tr, "U")
		if !has(t, store, front, tr["K"].Ref) {
			t.Fatal("the walk of U left the absent K off the front")
		}
		for range 4 {
			if livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_UNADOPTED {
				if _, err := v.Adopt(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Flipped || res.Freed != 0 {
				t.Fatalf("collect while U holds the absent K = %+v, want no flip and no free", res)
			}
		}
		requireStored(t, v, tr, true, "U")
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("K"), nil); err != nil {
			t.Fatal(err)
		}
		if has(t, store, front, tr["K"].Ref) {
			t.Fatal("K born from content stayed on the front, which would hold the flip")
		}
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("reopen by a copied child of a parent that landed first", func(t *testing.T) {
		// While Settled, R lands from content before its child H, and a
		// root names R. A copied put of H lacking the stored X, which
		// nothing else holds, reopens the Volume. No root or down names H,
		// so only H's front entry makes the walk decode it: without it the
		// flip counts H's copied list and the sweep frees X, which H names.
		store := newGuardedStore(t)
		v, clk := openLivenessVolume(t, store, true)
		observePuts(v)
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "H", "X")
		tr.add(t, "R", "H")
		if _, err := v.PutBlockBatch(t.Context(), tr.entries("X", "R")); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{copied(tr, "H")}, []block.NamedRoot{{Name: "main", Ref: tr["R"].Ref}}); err != nil {
			t.Fatal(err)
		}
		requireReopened(t, v, store, tr, "H")
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "R", "H", "X")
		requireAudit(t, v, 0)
	})

	t.Run("reopen by a copied list lacking a stored child, then a restart", func(t *testing.T) {
		// A Settled Volume with frees off stores K, which nothing holds. A
		// copied put of P lacking K reopens it, and it restarts with frees
		// on, reopened. The recount finds K at zero, and the free parks it
		// until the walk decodes P. P's list then gains K, a rewrite the
		// flip recounts before it flips, and K stays.
		store := newGuardedStore(t)
		v, _ := openLivenessVolume(t, store, false)
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SETTLED {
			t.Fatalf("a new Volume's generation is %v, want Settled", gen)
		}
		observePuts(v)
		tr := testTree{}
		tr.add(t, "K")
		tr.add(t, "P", "K")
		if _, err := v.PutBlockBatch(t.Context(), tr.entries("K")); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{copied(tr, "P")}, []block.NamedRoot{{Name: "main", Ref: tr["P"].Ref}}); err != nil {
			t.Fatal(err)
		}
		requireReopened(t, v, store, tr, "P")
		if !v.hidesNone.Load() || settledWord(v.settled.load()) {
			t.Fatal("a reopen with frees off left reads hiding blocks or the Volume Settled")
		}
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}

		// Restart with frees on: the Volume opens reopened, and frees park.
		v2, clk := openLivenessVolume(t, store, true)
		requireReopened(t, v2, store, tr, "P")
		waitAdopted(t, v2)
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v2); res.Flipped || res.Freed != 0 {
				t.Fatalf("collect before the walk = %+v, want no flip and no free", res)
			}
		}
		requireStored(t, v2, tr, true, "K", "P")

		// The walk rewrites P's list to name K, and the flip recounts first.
		if res := settleAll(t, v2, clk, store, tr); !res.Unadopted || res.Freed != 0 {
			t.Fatalf("settle = %+v, want the rewrite recounted and nothing freed", res)
		}
		requireReads(t, v2, tr, "P", "K")
		requireAudit(t, v2, 0)
	})

	for _, marked := range []bool{true, false} {
		t.Run("copied put of a parked block marked="+strconv.FormatBool(marked), func(t *testing.T) {
			// The free parks the unreached G -> X. A copied put of G keeps
			// G's list. A marked one released X, which the free then parked
			// too, so the put of G alone is refused and the writer puts X
			// with it. An unverified one, cut to lack X, returns to the run
			// and the front, where the walk decodes G and keeps X.
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "X")
			tr.add(t, "G", "X")
			var cut []string
			if !marked {
				cut = []string{"G"}
			}
			v, clk, store := oldStore(t, tr, "A", []string{"A", "X", "G"}, cut...)
			if marked {
				verify(t, v, tr, "G", "X")
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Parked == 0 || res.Freed != 0 {
				t.Fatalf("collect while Open = %+v, want G parked", res)
			}
			puts := []*block.PutBatchEntry{copied(tr, "G")}
			if marked {
				var absent *block_gc.AbsentError
				if err := v.SetBucketRoots(t.Context(), "b", puts, []block.NamedRoot{{Name: "main", Ref: tr["G"].Ref}}); !errors.As(err, &absent) || !absent.Ref.EqualsRef(tr["X"].Ref) {
					t.Fatalf("copied put of G alone err %v, want the parked X refused", err)
				}
				puts = append(puts, copied(tr, "X"))
			}
			if err := v.SetBucketRoots(t.Context(), "b", puts, []block.NamedRoot{{Name: "main", Ref: tr["G"].Ref}}); err != nil {
				t.Fatal(err)
			}
			if !marked && (!has(t, store, run, tr["G"].Ref) || !has(t, store, front, tr["G"].Ref)) {
				t.Fatal("the copied put left the unverified G off the run or the front")
			}
			settleAll(t, v, clk, store, tr)
			requireReads(t, v, tr, "G", "X")
			requireAudit(t, v, 0)
		})
	}

	t.Run("copied put of a recorded hole", func(t *testing.T) {
		// The audit records the lost H as a hole. A copied put of H is
		// refused and a copied fill dropped and counted, since only content
		// refills a hole; the put from content refills it.
		v, _ := newLivenessVolume(t, true)
		tr := testTree{}
		tr.add(t, "H")
		tr.add(t, "D", "H")
		if err := rootOnto(t, v, tr, "D", "H", "D"); err != nil {
			t.Fatal(err)
		}
		punchHole(t, v, tr["H"].Ref)
		if res, err := v.Audit(t.Context()); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		if err := v.SetBucketRoots(t.Context(), "b", []*block.PutBatchEntry{copied(tr, "H")}, nil); !errors.Is(err, block.ErrRefsUnknown) {
			t.Fatalf("copied put of the hole err %v, want refused", err)
		}
		h := copied(tr, "H")
		h.CacheFill = true
		_, err := v.PrepareOwnedBlockBatch(t.Context(), "b", []*block.PutBatchEntry{h})
		requireDropped(t, err, 0)
		requireStored(t, v, tr, false, "H")
		if fill := putFlow(t, v, "fill"); fill.DroppedHoles != 1 || fill.Dropped != 0 {
			t.Fatalf("fill flow = %+v, want the hole's fill dropped", fill)
		}
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("H"), nil); err != nil {
			t.Fatal(err)
		}
		requireReads(t, v, tr, "D", "H")
	})

	t.Run("frees off", func(t *testing.T) {
		// With frees off the collector runs none of the settle: the run is
		// never built, nothing flips, and every block stays.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "U")
		store := newGuardedStore(t)
		writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, "A", true, "A", "U")
		v, clk := openLivenessVolume(t, store, false)
		if _, err := v.Adopt(t.Context()); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Unmarked != 0 || res.Flipped || res.Freed != 0 {
			t.Fatalf("collect with frees off = %+v, want no settle", res)
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN || countPrefix(t, store, run) != 0 {
			t.Fatalf("generation %v with %d run entries, want Open and no run", gen, countPrefix(t, store, run))
		}
		requireStored(t, v, tr, true, "A", "U")
	})
}
