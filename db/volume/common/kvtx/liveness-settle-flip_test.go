package kvtx

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// testLivenessSettleFlip checks the generation flip and subsequent rebirths.
func testLivenessSettleFlip(t *testing.T,
	oldStore func(t *testing.T, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock, kvtx.Store),
	has func(t *testing.T, store kvtx.Store, prefix []byte, ref *block.BlockRef) bool,
	fill func(t *testing.T, v *Volume, tr testTree, names ...string),
	rootOnto func(t *testing.T, v *Volume, tr testTree, name string, put ...string) error,
	verify func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult,
	requireKept func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree, below, path string),
	collectUntil func(t *testing.T, v *Volume, clk *testClock, done func(block_gc.CollectResult) bool) block_gc.CollectResult,
	flipped func(res block_gc.CollectResult) bool,
	settled func(res block_gc.CollectResult) bool,
	oldU func(t *testing.T, cut bool) (*Volume, *testClock, kvtx.Store, testTree),
	passes func(t *testing.T, v *Volume, clk *testClock, n int),
	run []byte,
	front []byte,
) {
	t.Helper()

	t.Run("read-only open beside a writer that flips", func(t *testing.T) {
		// A writer with frees on flips the generation to Sweeping with
		// the old U unmarked, so U reads absent to a writer checking
		// before it puts. A read-only Volume on the same store, as the
		// debug commands open one, opened before the flip or after it,
		// answers the same: a copy that trusted it would skip U's put
		// and reference a block the sweep then deletes. A Settled
		// Volume never opens a generation again, so a reader that
		// opened Settled has nothing to hide.
		for _, before := range []bool{false, true} {
			t.Run("before="+strconv.FormatBool(before), func(t *testing.T) {
				v, clk, store, tr := oldU(t, false)
				openRO := func() *Volume {
					ro, err := NewVolume(t.Context(), "test/liveness", store_kvkey.NewDefaultKVKey(), readOnlyStore{store.(*seekGuard)}, &store_kvtx.Config{}, true, true, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = ro.Close() })
					return ro
				}
				var ro *Volume
				if before {
					ro = openRO()
				}
				collectUntil(t, v, clk, flipped)
				if !has(t, store, run, tr["U"].Ref) {
					t.Fatal("U is not in the run")
				}
				if !before {
					ro = openRO()
				}
				for _, vol := range []*Volume{v, ro} {
					if found, err := vol.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || found {
						t.Fatalf("U exists=%v err %v on the read-only=%v Volume, want absent", found, err, vol.readOnly)
					}
					if found, err := vol.GetBlockExists(t.Context(), tr["A"].Ref); err != nil || !found {
						t.Fatalf("A exists=%v err %v on the read-only=%v Volume, want present", found, err, vol.readOnly)
					}
				}
			})
		}
	})

	t.Run("unwalked root seeds the flip", func(t *testing.T) {
		// The carried root references A, which no walk decoded. The
		// flip pushes A onto the front rather than flip, and the walk
		// then decodes A.
		tr := testTree{}
		tr.add(t, "K")
		tr.add(t, "A", "K")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K"})
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || res.Seeded != 1 || !has(t, store, front, tr["A"].Ref) {
			t.Fatalf("collect with A unwalked = %+v, want A seeded and no flip", res)
		}
		requireKept(t, v, clk, store, tr, "K", "the carried root and A")
	})

	t.Run("release just before the flip seeds it", func(t *testing.T) {
		// A is verified, and the carried root holds it. Just before the
		// flip's transaction, a session moves the carried root off A,
		// so no named root references A but a down pair past the cursor
		// does, as a reader that held A may still read it. The flip
		// pushes A rather than flip. Once the release nets and ages,
		// the free proof pops A, and the sweep deletes it.
		tr := testTree{}
		tr.add(t, "K")
		tr.add(t, "U", "K")
		a := tr.add(t, "A", "U")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "U", "K"}, "A")
		var released bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if released || !has(t, store, run, nil) || countPrefix(t, store, front) != 0 {
				return
			}
			released = true
			if err := v.SetBucketRoots(t.Context(), "space", nil, []block.NamedRoot{{Name: "world", Previous: a}}); err != nil {
				t.Fatal(err)
			}
		}}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		var total block_gc.CollectResult
		for range 8 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			res, err := c.Collect(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			total.Seeded += res.Seeded
			if res.Flipped {
				break
			}
		}
		if !released || total.Seeded != 1 {
			t.Fatalf("released %v and seeded %d, want the release's down pair to seed A", released, total.Seeded)
		}
		collectUntil(t, v, clk, settled)
		requireStored(t, v, tr, false, "A", "U", "K")
		requireAudit(t, v, 0)
	})

	// holdersOf returns the netted holders of ref.
	holdersOf := func(t *testing.T, v *Volume, ref *block.BlockRef) uint64 {
		t.Helper()
		var n uint64
		err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
			var err error
			n, _, err = rec.GetHolders(t.Context(), ref)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	// deadU builds the root A, the old U -> D unreached, and main holding D,
	// which the walk verified, so D stays outside the run. U dies while Open,
	// and its free parks it, so U's list still holds D beside main. It
	// returns once the generation flips.
	deadU := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "D")
		tr.add(t, "U", "D")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "D", "U"})
		verify(t, v, tr, "A", "D")
		if err := rootOnto(t, v, tr, "D"); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, flipped)
		if has(t, store, run, tr["D"].Ref) || !has(t, store, run, tr["U"].Ref) {
			t.Fatal("the flip left D in the run or U out of it")
		}
		if n := holdersOf(t, v, tr["D"].Ref); n != 1 {
			t.Fatalf("D has %d holders at the flip, want main's, U's park having released its own", n)
		}
		return v, clk, store, tr
	}

	t.Run("pop on a count a rewrite made stale", func(t *testing.T) {
		// A -> P -> C -> D, with P's list cut, so adoption counts C at zero.
		// The walk decodes P, whose list gains C, which goes onto the front
		// and owes a recount. No pass may pop C on its stale count: once the
		// recount gives C P's holder, nothing would push C again, and the
		// sweep would delete it under the live P.
		tr := testTree{}
		tr.add(t, "D")
		tr.add(t, "C", "D")
		tr.add(t, "P", "C")
		tr.add(t, "A", "P")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "P", "C", "D"}, "P")
		verify(t, v, tr, "A")
		if res := verify(t, v, tr, "P"); res.Rewritten != 1 || !has(t, store, front, tr["C"].Ref) {
			t.Fatalf("decode of P = %+v, want its list rewritten and C on the front", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Popped != 0 {
			t.Fatalf("pass with a recount owed = %+v, want C kept on the front", res)
		}
		requireKept(t, v, clk, store, tr, "D", "P's rewritten list and C")
		requireStored(t, v, tr, true, "P", "C", "D")
	})

	t.Run("rebirth of a block whose free released its child", func(t *testing.T) {
		// After the flip a put gives birth again to U under another root.
		// The rebirth releases the old list's holder of D and the new list
		// gives one: D has main and the reborn U, and keeps U's once main
		// moves off.
		v, clk, _, tr := deadU(t)
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("U"), []block.NamedRoot{{Name: "other", Ref: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, settled)
		if n := holdersOf(t, v, tr["D"].Ref); n != 2 {
			t.Fatalf("D has %d holders after U's rebirth, want main and U", n)
		}
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		passes(t, v, clk, 4)
		requireStored(t, v, tr, true, "U", "D")
		requireAudit(t, v, 0)
	})

	t.Run("fill up onto a dead unmarked block", func(t *testing.T) {
		// After the flip a read fills B referencing the unmarked U,
		// whose fill up raises U before the sweep deletes it. The sweep
		// releases the old list's holder of D, and main still holds D.
		v, clk, _, tr := deadU(t)
		tr.add(t, "B", "U")
		fill(t, v, tr, "B")
		clk.Add(block_gc.DefaultGrace + time.Second)
		res := collectAll(t, v)
		if res.Swept == 0 {
			t.Fatalf("pass after the fill = %+v, want the sweep run", res)
		}
		if !res.Settled {
			collectUntil(t, v, clk, settled)
		}
		passes(t, v, clk, 4)
		if n := holdersOf(t, v, tr["D"].Ref); n != 1 {
			t.Fatalf("D has %d holders after the sweep, want main's", n)
		}
		requireStored(t, v, tr, true, "D")
		requireStored(t, v, tr, false, "U")
	})

	t.Run("root onto a dead verified parent after the flip", func(t *testing.T) {
		// The walk verified the unreached P, which references the old C
		// -> E. P dies while Open and parks, which puts it in the run,
		// and its list still holds C, so the proof cannot pop C and the
		// walk verifies C and E. The flip unmarks P alone, and the pass
		// that flips may sweep it too. A root onto P after the flip is
		// refused, as unmarked or as missing, the sweep deletes P and
		// releases C, and the frees take C and E, which nothing
		// references.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "E")
		tr.add(t, "C", "E")
		tr.add(t, "P", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "E", "C", "P"})
		verify(t, v, tr, "A", "P")
		walkFront(t, v, store, tr)
		collectUntil(t, v, clk, flipped)
		if found, err := v.GetBlockExists(t.Context(), tr["P"].Ref); err != nil || found {
			t.Fatalf("the unmarked P exists=%v err %v, want absent", found, err)
		}
		err := rootOnto(t, v, tr, "P")
		var unmarked *block_gc.AbsentError
		switch {
		case errors.Is(err, block.ErrNotFound):
		case !errors.As(err, &unmarked) || !errors.Is(err, block.ErrRefsUnknown) || !unmarked.Ref.EqualsRef(tr["P"].Ref):
			t.Fatalf("root onto the unmarked P err %v, want an AbsentError or ErrNotFound", err)
		}
		if livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED {
			collectUntil(t, v, clk, settled)
		}
		passes(t, v, clk, 3)
		requireStored(t, v, tr, false, "P", "C", "E")
		if has(t, store, run, nil) {
			t.Fatal("the run outlived the settle")
		}
		requireAudit(t, v, 0)
	})

	t.Run("root onto a dead verified parent before its free", func(t *testing.T) {
		// As above, but the root moves onto P in the window between the flip
		// and P's free, while P is still stored. The park put P in the run,
		// so the flip unmarked it: the root is refused as unmarked, and the
		// sweep and the frees take all three. Were it accepted, its hold
		// would reach C and E, and they would have to stay.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "E")
		tr.add(t, "C", "E")
		tr.add(t, "P", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "E", "C", "P"})
		verify(t, v, tr, "A", "P")
		walkFront(t, v, store, tr)
		var tried bool
		var rootErr error
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if tried || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SWEEPING {
				return
			}
			if _, found, err := v.GetBlock(t.Context(), tr["P"].Ref); err != nil || !found {
				return
			}
			tried = true
			rootErr = rootOnto(t, v, tr, "P")
		}}
		swapCollector(v, block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now}))
		collectUntil(t, v, clk, settled)
		if !tried {
			t.Fatal("the root never moved onto P between the flip and its free")
		}
		switch {
		case rootErr == nil:
			requireStored(t, v, tr, true, "P", "C", "E")
		case errors.Is(rootErr, block.ErrRefsUnknown):
			requireStored(t, v, tr, false, "P", "C", "E")
		default:
			t.Fatal(rootErr)
		}
		requireAudit(t, v, 0)
	})

	t.Run("rebirth of a block a fill up raised", func(t *testing.T) {
		// U -> K, unreached, dies while Open. After the flip a read
		// fills B referencing U. Once B's fill up raises U, and before
		// the sweep reaches U, a put gives birth to U and K again under
		// another root. When B's fill ages out and B dies, the root
		// still holds U.
		v, clk, store, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		tr.add(t, "B", "U")
		fill(t, v, tr, "B")
		fillUps := slices.Concat(livenessPrefix, []byte{'N'})
		var put bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if put || countPrefix(t, store, fillUps) != 0 || !has(t, store, run, tr["U"].Ref) {
				return
			}
			put = true
			if err := v.SetBucketRoots(t.Context(), "b", tr.entries("U", "K"), []block.NamedRoot{{Name: "other", Ref: tr["U"].Ref}}); err != nil {
				t.Fatal(err)
			}
		}}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		for range 8 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			for {
				res, err := c.Collect(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if !res.More {
					break
				}
			}
		}
		if !put {
			t.Fatal("the fill up never applied before the sweep")
		}
		requireStored(t, v, tr, true, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("fill naming an unmarked block", func(t *testing.T) {
		// After the flip a read fills B, which references the unmarked
		// C. C is absent to every step from the flip on, so B
		// references an absent child, as any fill of part of a tree
		// does: B's up goes to C's count, which the sweep keeps when it
		// deletes C's bytes. A rooted parent referencing an absent
		// block has no claim on that block's children either, so the
		// outcome is the same whether the sweep ran before or after B
		// arrived. Once a later fill stores C again, C has B's holder
		// alone, and goes when B does.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "C"})
		tr.add(t, "B", "C")
		verify(t, v, tr, "A")
		collectUntil(t, v, clk, flipped)
		if !has(t, store, run, tr["C"].Ref) {
			t.Fatal("the flip left C out of the run")
		}
		fill(t, v, tr, "B")
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "other", Ref: tr["B"].Ref}}); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, settled)
		requireStored(t, v, tr, false, "C")
		requireStored(t, v, tr, true, "B")
		fill(t, v, tr, "C")
		passes(t, v, clk, 4)
		requireStored(t, v, tr, true, "B", "C")
		if n := holdersOf(t, v, tr["C"].Ref); n != 1 {
			t.Fatalf("C has %d holders once filled again, want B's", n)
		}
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "other", Previous: tr["B"].Ref}}); err != nil {
			t.Fatal(err)
		}
		passes(t, v, clk, 6)
		requireStored(t, v, tr, false, "B", "C")
		requireAudit(t, v, 0)
	})

	t.Run("readoption revives a parked block with its list", func(t *testing.T) {
		// A -> C -> F -> G, with C's list cut, so the adoption counts F at
		// zero. While Open the free parks F, which keeps its list and count,
		// and its list still holds G. The walk verifies C, whose list gains F
		// and owes a recount. The readoption counts F from C's rewritten list
		// and G from F's parked list, so the settle keeps both.
		tr := testTree{}
		tr.add(t, "G")
		tr.add(t, "F", "G")
		tr.add(t, "C", "F")
		tr.add(t, "A", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "C", "F", "G"}, "C")

		// Collect until the free parks F.
		var parked uint64
		for i := 0; parked < 1; i++ {
			if i == 8 {
				t.Fatalf("the free parked %d blocks, want F", parked)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			res := collectAll(t, v)
			if res.Flipped || res.Freed != 0 {
				t.Fatalf("collect before the walk = %+v, want no flip or free", res)
			}
			parked += res.Parked
		}

		// Verify A and C, then settle through the readoption.
		if res := verify(t, v, tr, "A", "C"); res.Rewritten != 1 {
			t.Fatalf("decode of A and C = %+v, want C's list rewritten", res)
		}
		res := settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A", "C", "F", "G")
		for _, name := range []string{"F", "G"} {
			if n := holdersOf(t, v, tr[name].Ref); n != 1 {
				t.Fatalf("%s has %d holders after the settle (%+v), want 1", name, n, res)
			}
		}
		requireAudit(t, v, 0)
	})

	t.Run("readoption after a pop with a writer ahead of the cursor", func(t *testing.T) {
		// The free proof pops U while the unverified seed A holds the
		// flip, and a readoption then recounts, which gives U and K new
		// ripe sequences, and the free parks both. Root d holds the old
		// D through the recount. Its release nets with frees off, so D
		// is Dead on the free queue and not parked. Root e then moves
		// onto the Dead D, whose Land takes D off the free queue until
		// that move nets, so the free cannot park D. The walk verifies
		// A, and the flip must not wait for D: D died after the
		// recount, so it references no popped child.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "K")
		tr.add(t, "U", "K")
		tr.add(t, "D")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U", "D"})
		ctx := t.Context()
		d := []block.NamedRoot{{Name: "d", Ref: tr["D"].Ref}}
		if err := v.SetBucketRoots(ctx, "b", nil, d); err != nil {
			t.Fatal(err)
		}
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		var popped uint64
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			popped += res.Popped
			return popped != 0 || res.Flipped
		})
		if res.Flipped || has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U popped and the flip held by A", res)
		}

		// Readopt, and let the free park what the recount left at zero.
		frees := livenessState(t, v).GetFrees()
		for _, on := range []bool{!frees, frees} {
			if _, err := block_gc.SetFrees(ctx, collectorStore{v: v}, on); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Flipped {
				t.Fatalf("collect flipped with A unverified: %+v", res)
			}
		}

		// Release D, net the release with frees off, then move root e onto
		// the Dead D and walk A and D.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "d", Previous: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "e", Ref: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		verify(t, v, tr, "A", "D")

		// Root e's move nets, and D, off the queue, stays. The flip
		// comes.
		if res := collectAll(t, v); !res.Flipped {
			t.Fatalf("collect with D held by root e = %+v, want the flip", res)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A", "D")
		requireStored(t, v, tr, false, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("front entry whose list and bytes are lost", func(t *testing.T) {
		// Root main moves onto the old U, which puts U on the front, and its
		// release nets with frees off, so U is Dead on the front and not
		// popped. Damage then loses U's list: while its
		// bytes remain, U stays on the front for the walk to decode. Once the
		// bytes are lost too nothing can decode U, so the free proof must pop
		// it, and the flip comes with A kept, rather than U keeping the
		// generation Open.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "K")
		tr.add(t, "U", "K")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U"})
		ctx := t.Context()
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := netAll(t, v); !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("netting = %+v, want U Dead on the front", res)
		}

		// Lose U's list: while its bytes remain, U stays once it ripens.
		lose := func(f func(p *physicalTx) error) {
			if err := v.withDirectAtomic(ctx, f); err != nil {
				t.Fatal(err)
			}
		}
		lose(func(p *physicalTx) error { return p.rec.DeleteChildren(ctx, tr["U"].Ref) })
		verify(t, v, tr, "A")
		clk.Add(block_gc.DefaultGrace)
		if res := collectAll(t, v); res.Flipped || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U on the front while its bytes remain", res)
		}

		// Lose U's bytes too, and collect until the flip.
		lose(func(p *physicalTx) error { return p.blocks.RmBlock(ctx, tr["U"].Ref) })
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool { return res.Flipped })
		if has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U off the front", res)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A")
		requireStored(t, v, tr, false, "U")
	})

	t.Run("put of a lost front entry beside its pop", func(t *testing.T) {
		// As "front entry whose list and bytes are lost", U is Dead on the
		// front with its list and bytes lost, so the next pass pops
		// and parks it. A writer puts U and K again and moves root main onto
		// U, just before that pass, just after it, or once the flip left U
		// unmarked. The put finds no list, yet U keeps its zero count, its
		// run entry and, after the pass, its parked key. The store must keep
		// U, which root main holds, through the flip and the sweep.
		for _, name := range []string{"before the pop", "after the pop", "after the flip"} {
			t.Run(name, func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "A")
				tr.add(t, "K")
				tr.add(t, "U", "K")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U"})
				ctx := t.Context()
				if err := rootOnto(t, v, tr, "U"); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				if res := netAll(t, v); !has(t, store, front, tr["U"].Ref) {
					t.Fatalf("netting = %+v, want U Dead on the front", res)
				}

				// Lose U's list and bytes, and walk A, so U alone holds the flip.
				err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
					if err := p.rec.DeleteChildren(ctx, tr["U"].Ref); err != nil {
						return err
					}
					return p.blocks.RmBlock(ctx, tr["U"].Ref)
				})
				if err != nil {
					t.Fatal(err)
				}
				verify(t, v, tr, "A")
				clk.Add(block_gc.DefaultGrace)

				// Put U again and hold it, beside the pass that pops it.
				put := func() {
					t.Helper()
					if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
						t.Fatal(err)
					}
				}
				switch name {
				case "before the pop":
					put()
				case "after the pop":
					if res := collectAll(t, v); res.Popped == 0 {
						t.Fatalf("collect = %+v, want U popped", res)
					}
					put()
				default:
					if res := collectUntil(t, v, clk, flipped); res.Swept != 0 {
						t.Fatalf("collect = %+v, want the flip before the sweep", res)
					}
					put()
				}
				collectUntil(t, v, clk, settled)
				requireStored(t, v, tr, true, "A", "U", "K")
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("unborn front entry at one holder", func(t *testing.T) {
		// X is a block the store lacks, with neither a child list nor a hole
		// record, on the front at one holder. It reads the same whether that
		// holder is the implied one of a block never born, which nothing
		// references once its parents parked, or a real one kept after a
		// loss. The reference scan tells the two apart: it pops X only when
		// no root and no holding list references it, and only while no push
		// deleted the mark it wrote before it read them.
		mark := func(t *testing.T, store kvtx.Store, ref *block.BlockRef) []byte {
			t.Helper()
			tx, err := store.NewTransaction(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			value, _, err := tx.Get(t.Context(), slices.Concat(front, block_gc.BlockKey(ref)))
			if err != nil {
				t.Fatal(err)
			}
			return bytes.Clone(value)
		}
		// collectN collects n times, a grace period apart, and returns the
		// totals.
		collectN := func(t *testing.T, v *Volume, clk *testClock, n int) block_gc.CollectResult {
			t.Helper()
			var total block_gc.CollectResult
			for range n {
				clk.Add(block_gc.DefaultGrace + time.Second)
				res := collectAll(t, v)
				total.Parked += res.Parked
				total.Popped += res.Popped
				total.Unreferenced += res.Unreferenced
				total.RefScanned += res.RefScanned
				total.Flipped = total.Flipped || res.Flipped
				total.Unadopted = total.Unadopted || res.Unadopted
			}
			return total
		}
		// stuck adopts an old store rooted on A in which the unreached P and
		// R reference X, which the store lacks, and walks A and P, so the walk
		// pushes X. With held, root main moves onto the old U, which stays
		// unverified on the front and holds the flip.
		stuck := func(t *testing.T, held bool) (*Volume, *testClock, kvtx.Store, testTree) {
			t.Helper()
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "U")
			tr.add(t, "X")
			tr.add(t, "P", "X")
			tr.add(t, "R", "X")
			x := tr["X"]
			delete(tr, "X")
			v, clk, store := oldStore(t, tr, "A", []string{"A", "U", "P", "R"})
			tr["X"] = x
			if held {
				if err := rootOnto(t, v, tr, "U"); err != nil {
					t.Fatal(err)
				}
			}
			verify(t, v, tr, "A", "P")
			if !has(t, store, front, x.Ref) {
				t.Fatal("the walk of P left the absent X off the front")
			}
			return v, clk, store, tr
		}

		t.Run("nothing references it", func(t *testing.T) {
			// P and R park and release X, which falls to its implied holder.
			// The scan pops it, and the flip and the sweep follow.
			v, clk, store, tr := stuck(t, false)
			res := collectN(t, v, clk, 4)
			if res.Unreferenced != 1 || !res.Flipped || has(t, store, front, tr["X"].Ref) {
				t.Fatalf("collect = %+v, want X popped as unreferenced and the flip", res)
			}
			settleAll(t, v, clk, store, tr)
			requireStored(t, v, tr, true, "A")
			requireStored(t, v, tr, false, "P", "R", "X")
			requireAudit(t, v, 0)
		})

		t.Run("kept with the scan switched off", func(t *testing.T) {
			// The Volume's switch keeps the entry and reads no list. With
			// the switch off again the next collector pops it.
			v, clk, store, tr := stuck(t, false)
			swapCollector(v, v.newCollector(collectorStore{v: v}, true, true, 0))
			if res := collectN(t, v, clk, 4); res.Unreferenced != 0 || res.RefScanned != 0 || res.Flipped || !has(t, store, front, tr["X"].Ref) {
				t.Fatalf("collect with the scan off = %+v, want X kept and no list read", res)
			}
			if got := mark(t, store, tr["X"].Ref); len(got) != 0 {
				t.Fatalf("X's front key holds the mark %x with the scan off, want none", got)
			}
			swapCollector(v, v.newCollector(collectorStore{v: v}, true, false, 0))
			if res := collectN(t, v, clk, 4); res.Unreferenced != 1 || !res.Flipped {
				t.Fatalf("collect with the scan on = %+v, want X popped and the flip", res)
			}
		})

		t.Run("pushed between the mark and the pop", func(t *testing.T) {
			// The walk verifies R after the scan marked X and before its
			// pop, which pushes X and deletes the mark. That pop must leave
			// X, and the next scan, which marks it again, pops it.
			v, clk, store, tr := stuck(t, false)
			x := tr["X"].Ref
			var pushed, checked, kept bool
			probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
				switch {
				case pushed && !checked:
					checked, kept = true, has(t, store, front, x)
				case !pushed && len(mark(t, store, x)) != 0:
					pushed = true
					verify(t, v, tr, "R")
				}
			}}
			swapCollector(v, v.newCollector(probe, true, false, 0))
			res := collectN(t, v, clk, 4)
			if !pushed || !kept {
				t.Fatalf("collect = %+v with X pushed after its mark (%t), want that pop to keep X (%t)", res, pushed, kept)
			}
			if res.Unreferenced != 1 || !res.Flipped {
				t.Fatalf("collect = %+v, want the next scan to pop X and the flip", res)
			}
		})

		t.Run("marked by another collector", func(t *testing.T) {
			// A second collector on the same store writes its own mark over
			// the scan's before the pop. That mark stands for reads this
			// scan did not make, so the pop must leave X, and the next scan,
			// which marks it again, pops it.
			v, clk, store, tr := stuck(t, false)
			x := tr["X"].Ref
			var marked, checked, kept bool
			probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
				switch {
				case marked && !checked:
					checked, kept = true, has(t, store, front, x)
				case !marked && len(mark(t, store, x)) != 0:
					marked = true
					tx, err := store.NewTransaction(t.Context(), true)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Discard()
					if err := tx.Set(t.Context(), slices.Concat(front, block_gc.BlockKey(x)), []byte("another")); err != nil {
						t.Fatal(err)
					}
					if err := tx.Commit(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
			}}
			swapCollector(v, v.newCollector(probe, true, false, 0))
			res := collectN(t, v, clk, 4)
			if !marked || !kept {
				t.Fatalf("collect = %+v with X marked by another collector (%t), want that pop to keep X (%t)", res, marked, kept)
			}
			if res.Unreferenced != 1 || !res.Flipped {
				t.Fatalf("collect = %+v, want the next scan to pop X and the flip", res)
			}
		})

		t.Run("its parent held again after the pop", func(t *testing.T) {
			// Once X is popped, a fill of N, which references the parked P,
			// and a root onto N hold P again. The unpark of P's released
			// list returns X to the front, where it holds the flip: a live
			// list references a block the store lacks.
			v, clk, store, tr := stuck(t, true)
			x := tr["X"].Ref
			if res := collectN(t, v, clk, 3); res.Unreferenced != 1 || res.Flipped || has(t, store, front, x) {
				t.Fatalf("collect = %+v, want X popped and U holding the flip", res)
			}
			tr.add(t, "N", "P")
			fill(t, v, tr, "N")
			if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "keep", Ref: tr["N"].Ref}}); err != nil {
				t.Fatal(err)
			}
			netAll(t, v)
			if !has(t, store, front, x) {
				t.Fatal("the unpark of the released P left X off the front")
			}
			verify(t, v, tr, "U")
			if res := collectN(t, v, clk, 4); res.Flipped || !has(t, store, front, x) {
				t.Fatalf("collect = %+v, want X, which the live P references, to hold the flip", res)
			}
			requireReads(t, v, tr, "A", "U", "N", "P")
		})

		// A holder references X: a held list, a root, or a held list with
		// the hole recorded. The scan finds the holder and keeps X, reads
		// the lists once and not again, and never reads them for a recorded
		// hole. Once the holder releases X, X leaves the front and the flip
		// comes.
		for _, tc := range []struct {
			name string
			root bool
			hole bool
		}{
			{name: "a held list references it"},
			{name: "a root holds it", root: true},
			{name: "a recorded hole", hole: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "A")
				x := tr.add(t, "X")
				tr.add(t, "Q", "X")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "X", "Q"})
				ctx := t.Context()

				// Hold X by root main or through Q, then lose it.
				holder, walked := "Q", []string{"A", "Q"}
				if tc.root {
					holder, walked = "X", []string{"A"}
				}
				// A pass before the loss leaves the front scan idle, so
				// only the walk's push of the lost X ends the idle. A root's
				// X is on the front already, and its loss is no event.
				if err := rootOnto(t, v, tr, holder); err != nil {
					t.Fatal(err)
				}
				if !tc.root {
					clk.Add(block_gc.DefaultGrace + time.Second)
					collectAll(t, v)
				}
				loseBlock(t, v, x)
				verify(t, v, tr, walked...)
				if tc.hole {
					if _, err := v.Audit(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if !has(t, store, front, x) {
					t.Fatal("the lost X is off the front")
				}

				// The scan keeps X, and reads the lists once.
				res := collectN(t, v, clk, 3)
				if res.Unreferenced != 0 || res.Flipped || !has(t, store, front, x) || (res.RefScanned == 0) != tc.hole {
					t.Fatalf("collect = %+v, want X kept on the front", res)
				}
				if res := collectN(t, v, clk, 2); res.RefScanned != 0 || res.Flipped {
					t.Fatalf("collect again = %+v, want no second read of the lists", res)
				}
				if tc.hole {
					return
				}

				// Release the holder, and X leaves the front.
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr[holder].Ref}}); err != nil {
					t.Fatal(err)
				}
				res = collectN(t, v, clk, 4)
				if !res.Flipped || has(t, store, front, x) {
					t.Fatalf("collect after the release = %+v, want X off the front and the flip", res)
				}
				if want := uint64(1); !tc.root && res.Unreferenced != want {
					t.Fatalf("collect after the release = %+v, want X popped as unreferenced", res)
				}
				settleAll(t, v, clk, store, tr)
				requireStored(t, v, tr, true, "A")
				requireStored(t, v, tr, false, "Q", "X")
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("put again of a swept child before its unmarked parent", func(t *testing.T) {
		// The old chain G -> P -> C is garbage, among 300 old leaves, and C's
		// key sorts more than a sweep slice before P's. Between the slice
		// that sweeps C and the one that sweeps P, a put stores C again under
		// the new X, in one batch, so C counts X's implied holder alone. The
		// sweep of P must not release a holder from C, which X holds. In the
		// zero count case C's count reads zero after the sweep, which only
		// damage leaves, since netting floors an unlisted block at its
		// implied holder, so the put reparks C, and the repark must keep C's
		// run entry: P's old list still holds C, and P's
		// sweep reads that entry to keep C's holder.
		for _, zero := range []bool{false, true} {
			t.Run("zero count "+strconv.FormatBool(zero), func(t *testing.T) {
				var tr testTree
				listed := []string{"A", "G", "P", "C"}
				for i := 0; ; i++ {
					if i == 64 {
						t.Fatal("no chain sorts C a slice before P")
					}
					tr = testTree{}
					tr.add(t, "A")
					fillers := make([][]byte, 300)
					for j := range fillers {
						fillers[j] = block_gc.BlockKey(tr.add(t, "F"+strconv.Itoa(j)))
					}
					tr.add(t, "C"+strconv.Itoa(i))
					tr["C"] = tr["C"+strconv.Itoa(i)]
					c := block_gc.BlockKey(tr["C"].Ref)
					p := block_gc.BlockKey(tr.add(t, "P", "C"))
					between := 0
					for _, k := range fillers {
						if bytes.Compare(c, k) < 0 && bytes.Compare(k, p) < 0 {
							between++
						}
					}
					if between > 130 {
						break
					}
				}
				for j := range 300 {
					listed = append(listed, "F"+strconv.Itoa(j))
				}
				tr.add(t, "G", "P")
				tr.add(t, "X", "C")
				v, clk, store := oldStore(t, tr, "A", listed)
				verify(t, v, tr, "A")

				// Put C again under X once the sweep took C and not yet P.
				runValue := func(ref *block.BlockRef) string {
					tx, err := store.NewTransaction(t.Context(), false)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Discard()
					value, _, err := tx.Get(t.Context(), slices.Concat(run, block_gc.BlockKey(ref)))
					if err != nil {
						t.Fatal(err)
					}
					return string(value)
				}
				var put bool
				probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
					if put || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SWEEPING {
						return
					}
					if has(t, store, run, tr["C"].Ref) != zero || !has(t, store, run, tr["P"].Ref) {
						return
					}
					if zero && (runValue(tr["C"].Ref) != "s" || runValue(tr["P"].Ref) != "") {
						return
					}
					put = true
					if zero {
						err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
							return p.rec.SetHolders(t.Context(), tr["C"].Ref, 0, 0)
						})
						if err != nil {
							t.Fatal(err)
						}
					}
					if err := v.SetBucketRoots(t.Context(), "b", tr.entries("C", "X"), []block.NamedRoot{{Name: "x", Ref: tr["X"].Ref}}); err != nil {
						t.Fatal(err)
					}
				}}
				c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
				for range 12 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					for more := true; more; {
						res, err := c.Collect(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						more = res.More
					}
				}
				if !put || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED {
					t.Fatalf("put %v at generation %v, want C put between the slices and the settle done", put, livenessGeneration(t, v))
				}
				requireStored(t, v, tr, true, "X", "C")
				requireStored(t, v, tr, false, "G", "P")
				if n := holdersOf(t, v, tr["C"].Ref); n != 1 {
					t.Fatalf("C has %d holders, want X's", n)
				}
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("put of an unmarked block that lost its list", func(t *testing.T) {
		// The old chain P -> U is garbage, so the flip unmarks both, and
		// U's count keeps P's holder. U's list is then
		// lost, and a Land, or a fill and a root, stores U again under main
		// while Sweeping: the sweep must keep U, which main holds.
		for _, byFill := range []bool{false, true} {
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "U")
			tr.add(t, "P", "U")
			v, clk, _ := oldStore(t, tr, "A", []string{"A", "P", "U"})
			verify(t, v, tr, "A")
			collectUntil(t, v, clk, flipped)
			err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
				return p.rec.DeleteChildren(t.Context(), tr["U"].Ref)
			})
			if err != nil {
				t.Fatal(err)
			}
			if byFill {
				fill(t, v, tr, "U")
				err = rootOnto(t, v, tr, "U")
			} else {
				err = rootOnto(t, v, tr, "U", "U")
			}
			if err != nil {
				t.Fatal(err)
			}
			collectUntil(t, v, clk, settled)
			requireStored(t, v, tr, true, "A", "U")
			requireAudit(t, v, 0)
		}
	})

	t.Run("renewal of a lapsed pin on an unmarked block", func(t *testing.T) {
		// A pin holds the old U, and its process stalls past the lease,
		// so the collector releases the pin's root. The process stays
		// stalled while the release nets a grace period later, since a
		// renewal before then holds U again, as U still counts the pin.
		// U dies, the free proof pops it, and the flip unmarks it. The
		// process then pins the young W and renews: the renewal must
		// drop U's pin, since nothing holds U, and renew W's, rather
		// than fail the renewal on U.
		v, clk, store, tr := oldU(t, false)
		ctx := t.Context()
		if _, err := v.PinBucketRoot(ctx, tr["U"].Ref); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the pin left U off the front")
		}
		clk.Add(rootLease + time.Second)
		if res, err := v.collector.Collect(ctx); err != nil || res.Lapsed != 1 {
			t.Fatalf("stalled pass lapsed %d err %v, want U's pin", res.Lapsed, err)
		}
		for pass := 0; ; pass++ {
			if pass == 16 {
				t.Fatalf("the stalled collection never flipped; the generation is %v", livenessGeneration(t, v))
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			res, err := v.collector.Collect(ctx)
			for err == nil && res.More && !res.Flipped {
				res, err = v.collector.Collect(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Flipped {
				break
			}
		}

		// Pin W, then renew both pins.
		w := tr.add(t, "W")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("W"), []block.NamedRoot{{Name: "w", Ref: w}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PinBucketRoot(ctx, w); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "w", Previous: w}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.renewRoots(ctx, false); err != nil {
			t.Fatalf("renewal with U's pin lapsed onto the unmarked U: %v", err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "W")
		requireStored(t, v, tr, false, "U", "K")
	})
}
