package kvtx

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// testLivenessSettleSweep checks holds and readoption while Sweeping.
func testLivenessSettleSweep(t *testing.T,
	oldStore func(t *testing.T, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock, kvtx.Store),
	has func(t *testing.T, store kvtx.Store, prefix []byte, ref *block.BlockRef) bool,
	fill func(t *testing.T, v *Volume, tr testTree, names ...string),
	rootOnto func(t *testing.T, v *Volume, tr testTree, name string, put ...string) error,
	verify func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult,
	collectUntil func(t *testing.T, v *Volume, clk *testClock, done func(block_gc.CollectResult) bool) block_gc.CollectResult,
	flipped func(res block_gc.CollectResult) bool,
	settled func(res block_gc.CollectResult) bool,
	oldU func(t *testing.T, cut bool) (*Volume, *testClock, kvtx.Store, testTree),
	run []byte,
) {
	t.Helper()

	t.Run("hold of an unmarked block while Sweeping", func(t *testing.T) {
		// Once the generation flips, U and K are unmarked: the existence
		// check answers absent, a root onto U is refused as unmarked, which
		// asks the writer to put it again, and the sweep deletes both.
		v, clk, store, tr := oldU(t, false)
		var entered uint64
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			entered += res.Unmarked
			return res.Flipped
		})
		if entered != 2 {
			t.Fatalf("flip = %+v after %d run entries, want U and K in the run", res, entered)
		}
		if found, err := v.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || found {
			t.Fatalf("exists U while Sweeping = %v err %v, want absent", found, err)
		}
		err := rootOnto(t, v, tr, "U")
		var unmarked *block_gc.AbsentError
		if !errors.As(err, &unmarked) || !errors.Is(err, block.ErrRefsUnknown) || !unmarked.Ref.EqualsRef(tr["U"].Ref) {
			t.Fatalf("root onto the unmarked U err %v, want an AbsentError", err)
		}
		if res := collectUntil(t, v, clk, settled); res.Swept != 2 || has(t, store, run, nil) {
			t.Fatalf("sweep = %+v, want U and K swept and the run gone", res)
		}
		requireStored(t, v, tr, false, "U", "K")
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SETTLED {
			t.Fatalf("generation %v after the sweep, want Settled", gen)
		}
		requireAudit(t, v, 0)
	})

	t.Run("put of an unmarked block while Sweeping", func(t *testing.T) {
		// U's list lacks K. After the flip a put of U under main alone
		// is refused, since it holds the unmarked K. A put of both
		// gives birth to each again, and the sweep keeps them.
		v, clk, store, tr := oldU(t, true)
		collectUntil(t, v, clk, flipped)
		if err := rootOnto(t, v, tr, "U", "U"); !errors.Is(err, block.ErrRefsUnknown) {
			t.Fatalf("put of U alone err %v, want K refused as unmarked", err)
		}
		if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"U", "K"} {
			if !has(t, store, run, tr[name].Ref) {
				t.Fatalf("the rebirth dropped %s's run entry before the sweep", name)
			}
			if found, err := v.GetBlockExists(t.Context(), tr[name].Ref); err != nil || !found {
				t.Fatalf("exists %s after its rebirth = %v err %v, want found", name, found, err)
			}
		}
		if res := collectUntil(t, v, clk, settled); res.Swept != 0 {
			t.Fatalf("sweep = %+v, want the reborn U and K kept", res)
		}
		requireStored(t, v, tr, true, "A", "U", "K")
		requireAudit(t, v, 0)
	})

	// slotted builds the root A referencing C and a hub of 300 leaves, then
	// the block name referencing C, unreached, whose content places its
	// list at rank among the lists, and returns the tree, the listed names
	// and the first slice's last list key, at which a readoption's cursor
	// stops.
	slotted := func(t *testing.T, name string, rank int) (testTree, []string, []byte) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "C")
		listed := []string{"C"}
		for i := range 300 {
			leaf := "F" + strconv.Itoa(i)
			tr.add(t, leaf)
			listed = append(listed, leaf)
		}
		tr.add(t, "H", listed[1:]...)
		tr.add(t, "A", "C", "H")
		listed = append(listed, "H", "A")
		for i := 0; tr[name] == nil; i++ {
			try := name + strconv.Itoa(i)
			key := block_gc.BlockKey(tr.add(t, try, "C"))
			below := 0
			for _, other := range listed {
				if bytes.Compare(block_gc.BlockKey(tr[other].Ref), key) < 0 {
					below++
				}
			}
			if below == rank {
				tr[name] = tr[try]
			}
			delete(tr, try)
		}
		listed = append(listed, name)
		keys := make([][]byte, 0, len(listed))
		for _, other := range listed {
			keys = append(keys, block_gc.BlockKey(tr[other].Ref))
		}
		slices.SortFunc(keys, bytes.Compare)
		return tr, listed, keys[255]
	}
	// readoptTo readopts v, stopping once the cursor of the pass phase
	// reaches stop.
	readoptTo := func(t *testing.T, v *Volume, phase block_gc.AdoptPhase, stop []byte) {
		t.Helper()
		markUnadopted(t, v)
		ctx, cancel := context.WithCancel(t.Context())
		var stopped bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			cur := livenessState(t, v).GetAdoptCursor()
			if cur.GetPhase() == phase && bytes.Equal(cur.GetAfter(), stop) {
				stopped = true
				cancel()
			}
		}}
		_, err := block_gc.Adopt(ctx, probe, v.now)
		cancel()
		if !errors.Is(err, context.Canceled) || !stopped {
			t.Fatalf("adopt = %v, stopped %v, want canceled at the cursor", err, stopped)
		}
	}
	// requireClean finishes the adoption of v, nets its logs and checks its
	// counts against a clean recount of a copy.
	requireClean := func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store) {
		t.Helper()
		ctx := t.Context()
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clean := copyAll(t, store)
		vc, cclk := openUnadoptedVolume(t, clean)
		markUnadopted(t, vc)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		cclk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		collectAll(t, vc)
		if got, want := adoptedRecords(t, store), adoptedRecords(t, clean); !maps.Equal(got, want) {
			t.Fatalf("adoption differs from a clean one: %q", recordsDiff(got, want))
		}
	}
	// cursorCases stop a readoption exactly at a block's list, which the
	// count passed, and just below it, which it has not.
	cursorCases := []struct {
		name string
		rank int
	}{{"at its list", 255}, {"below its list", 256}}

	t.Run("rebirth while a readoption counts the lists", func(t *testing.T) {
		// The root A references C, and the unreached U references C
		// too, so after the flip U is in the run and C outside it. A
		// readoption counts the lists, and a put of U under its own
		// root gives birth to U again. The rebirth releases U's old
		// hold of C only where the count gave it, so the adoption
		// agrees with a clean recount.
		for _, tc := range cursorCases {
			t.Run(tc.name, func(t *testing.T) {
				tr, listed, stop := slotted(t, "U", tc.rank)
				v, clk, store := oldStore(t, tr, "A", listed)
				verify(t, v, tr, listed[:len(listed)-1]...)
				collectUntil(t, v, clk, flipped)
				readoptTo(t, v, block_gc.AdoptPhase_ADOPT_PHASE_CHILDREN, stop)
				roots := []block.NamedRoot{{Name: "u", Ref: tr["U"].Ref}}
				if err := v.SetBucketRoots(t.Context(), "b", tr.entries("U"), roots); err != nil {
					t.Fatal(err)
				}
				requireClean(t, v, clk, store)
			})
		}
	})

	t.Run("put again of a short list while a readoption counts the lists", func(t *testing.T) {
		// The root A references C, and the unreached P references C
		// too, but a crash cut P's list. While Open, a readoption
		// counts the lists, and a put of P under its own root adds C to
		// P's list. Where the count passed P's list the put logs C's
		// holder, and where it has not the count reads the new list, so
		// the adoption agrees with a clean recount and owes no second
		// one.
		for _, tc := range cursorCases {
			t.Run(tc.name, func(t *testing.T) {
				tr, listed, stop := slotted(t, "P", tc.rank)
				v, clk, store := oldStore(t, tr, "A", listed, "P")
				readoptTo(t, v, block_gc.AdoptPhase_ADOPT_PHASE_CHILDREN, stop)
				roots := []block.NamedRoot{{Name: "p", Ref: tr["P"].Ref}}
				if err := v.SetBucketRoots(t.Context(), "b", tr.entries("P"), roots); err != nil {
					t.Fatal(err)
				}
				if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
					t.Fatalf("generation %v at the put, want Open", gen)
				}
				requireClean(t, v, clk, store)
				if owed := livenessRecords(t, store, 'Y'); len(owed) != 0 {
					t.Fatal("the adoption left a recount owed")
				}
			})
		}
	})

	t.Run("hole while a readoption settles the lists", func(t *testing.T) {
		// The root A references C, a hub of 300 leaves and X, and the
		// store loses X with its list. An audit records the hole. A
		// readoption counts A's holder of X, and the settle writes X's
		// tally as its holders though X has no list. A put of X under
		// its own root refills it, either behind the settle's cursor or
		// once the Volume is adopted, and X's count agrees with a clean
		// recount.
		ctx := t.Context()
		for _, refilled := range []string{"behind the settle cursor", "once adopted"} {
			t.Run(refilled, func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "C")
				leaves := make([]string, 0, 300)
				for i := range 300 {
					leaves = append(leaves, "F"+strconv.Itoa(i))
					tr.add(t, leaves[i])
				}
				tr.add(t, "H", leaves...)
				names := append([]string{"C", "H"}, leaves...)
				var stop []byte
				for i := 0; stop == nil; i++ {
					// Choose X, then A referencing it, so
					// that X's key sorts below the end of
					// the settle's first slice, which holds
					// the hole as well as the lists.
					try := "X" + strconv.Itoa(i)
					x := block_gc.BlockKey(tr.add(t, try, "C"))
					tr["X"] = tr[try]
					delete(tr, try)
					tr.add(t, "A", "C", "H", "X")
					keys := make([][]byte, 0, len(names)+1)
					for _, name := range append(names, "A", "X") {
						keys = append(keys, block_gc.BlockKey(tr[name].Ref))
					}
					slices.SortFunc(keys, bytes.Compare)
					if bytes.Compare(x, keys[255]) < 0 {
						stop = keys[255]
					}
				}
				store := newGuardedStore(t)
				v, clk := openLivenessVolume(t, store, true)
				all := append(names, "X", "A")
				if err := v.SetBucketRoots(ctx, "b", tr.entries(all...), []block.NamedRoot{{Name: "main", Ref: tr["A"].Ref}}); err != nil {
					t.Fatal(err)
				}
				punchHole(t, v, tr["X"].Ref)
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit recorded %d holes err %v, want X", res.Holes, err)
				}
				if refilled == "behind the settle cursor" {
					readoptTo(t, v, block_gc.AdoptPhase_ADOPT_PHASE_SETTLE, stop)
				} else {
					markUnadopted(t, v)
					if _, err := v.Adopt(ctx); err != nil {
						t.Fatal(err)
					}
				}
				roots := []block.NamedRoot{{Name: "x", Ref: tr["X"].Ref}}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), roots); err != nil {
					t.Fatal(err)
				}
				requireClean(t, v, clk, store)
				if holders, _ := blockHolders(t, v, tr, "X"); holders != 2 {
					t.Fatalf("X has %d holders, want A and its root", holders)
				}
			})
		}
	})

	t.Run("listless holder while a readoption counts the lists", func(t *testing.T) {
		// The root A references C, a hub of 300 leaves and P, and the
		// store loses P's list, keeping its bytes or losing them too.
		// A's list sits in the count's first slice, and a Land writes
		// between the slices. P stored without its list refuses the
		// adoption, which leaves the Volume unadopted with no adoption
		// record. P lost with its bytes is a missing block, which does
		// not.
		ctx := t.Context()
		for _, stored := range []bool{true, false} {
			t.Run("stored "+strconv.FormatBool(stored), func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "C")
				leaves := make([]string, 0, 300)
				for i := range 300 {
					leaves = append(leaves, "F"+strconv.Itoa(i))
					tr.add(t, leaves[i])
				}
				tr.add(t, "H", leaves...)
				tr.add(t, "P", "C")
				tr.add(t, "Z", "C")
				listed := append([]string{"C", "H"}, leaves...)
				var stop []byte
				for i := 0; stop == nil; i++ {
					// Choose A so its list sorts within the first slice.
					try := "A" + strconv.Itoa(i)
					a := block_gc.BlockKey(tr.add(t, try, "C", "H", "P"))
					tr["A"] = tr[try]
					delete(tr, try)
					keys := make([][]byte, 0, len(listed)+1)
					for _, name := range append(listed, "A") {
						keys = append(keys, block_gc.BlockKey(tr[name].Ref))
					}
					slices.SortFunc(keys, bytes.Compare)
					if bytes.Compare(a, keys[255]) <= 0 {
						stop = keys[255]
					}
				}
				store := newGuardedStore(t)
				v, _ := openLivenessVolume(t, store, false)
				all := append(listed, "P", "A")
				if err := v.SetBucketRoots(ctx, "b", tr.entries(all...), []block.NamedRoot{{Name: "main", Ref: tr["A"].Ref}}); err != nil {
					t.Fatal(err)
				}
				if stored {
					err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
						return p.rec.DeleteChildren(ctx, tr["P"].Ref)
					})
					if err != nil {
						t.Fatal(err)
					}
				} else {
					loseBlock(t, v, tr["P"].Ref)
				}
				readoptTo(t, v, block_gc.AdoptPhase_ADOPT_PHASE_CHILDREN, stop)
				if err := v.SetBucketRoots(ctx, "b", tr.entries("Z"), []block.NamedRoot{{Name: "z", Ref: tr["Z"].Ref}}); err != nil {
					t.Fatal(err)
				}
				_, err := v.Adopt(ctx)
				if !stored {
					if err != nil {
						t.Fatalf("adopt over a missing P = %v, want adopted", err)
					}
					return
				}
				if !errors.Is(err, block_gc.ErrAdoptUnknown) {
					t.Fatalf("adopt over a listless P = %v, want ErrAdoptUnknown", err)
				}
				if state := livenessState(t, v); state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTED {
					t.Fatalf("state %v after the refusal, want not adopted", state)
				}
				if left := livenessRecords(t, store, 'H', 'Q', 'X', 'x', 'u', 'd', 'B', 'A', 'D', 'Y', 'T'); len(left) != 0 {
					t.Fatalf("the refused adoption left %d records", len(left))
				}
			})
		}
	})

	t.Run("fill of an unmarked block while Sweeping", func(t *testing.T) {
		// After the flip a read fills U and K, which gives birth to each
		// again, and a root moves onto U: the sweep keeps them both.
		v, clk, store, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		fill(t, v, tr, "U", "K")
		if found, err := v.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || !found {
			t.Fatalf("exists U after its fill = %v err %v, want found", found, err)
		}
		if !has(t, store, run, tr["U"].Ref) || !has(t, store, run, tr["K"].Ref) {
			t.Fatal("the fill dropped a run entry before the sweep")
		}
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatalf("root onto the filled U: %v", err)
		}
		var swept uint64
		collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			swept += res.Swept
			return res.Settled
		})
		if swept != 0 {
			t.Fatalf("the sweep after the fill swept %d, want the reborn U and K kept", swept)
		}
		requireStored(t, v, tr, true, "U", "K")
	})

	t.Run("put meets the records of an earlier life", func(t *testing.T) {
		// U references K. An earlier life leaves U's records in one
		// state: queued while Settled, fill-born with its logs not
		// netted, parked while Open, or unmarked while Sweeping. Then a
		// writer puts U and K again, by a Land or by a fill and a root.
		// Whatever records U's earlier life left, the put must leave U
		// reading, and the collector must keep it.
		fresh := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
			t.Helper()
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "K")
			tr.add(t, "U", "K")
			tr.add(t, "W")
			store := newGuardedStore(t)
			v, clk := openLivenessVolume(t, store, true)
			waitAdopted(t, v)
			return v, clk, store, tr
		}
		queuedKey := slices.Concat(livenessPrefix, []byte{'Q'})
		parkedKey := slices.Concat(livenessPrefix, []byte{'X'})
		releasedKey := slices.Concat(livenessPrefix, []byte{'x'})
		states := []struct {
			name string
			// open builds the state and returns the Volume.
			open func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree)
			// settle collects the Volume to its end state.
			settle func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree)
		}{{
			name: "queued while Settled",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				// A pass with frees off nets the release, so U waits on the
				// free queue.
				v, clk, store, tr := fresh(t)
				roots := []block.NamedRoot{{Name: "main", Ref: tr["U"].Ref}, {Name: "w", Ref: tr["W"].Ref}}
				if err := v.SetBucketRoots(t.Context(), "b", tr.entries("K", "U", "W"), roots); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				if res := netAll(t, v); countPrefix(t, store, queuedKey) == 0 {
					t.Fatalf("collect = %+v, want U queued", res)
				}
				requireStored(t, v, tr, true, "U")
				return v, clk, store, tr
			},
		}, {
			name: "fill-born with its logs not netted",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				v, clk, store, tr := fresh(t)
				fill(t, v, tr, "U", "K")
				return v, clk, store, tr
			},
		}, {
			name: "parked while Open",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				// W, never walked, holds the generation Open.
				tr := testTree{}
				tr.add(t, "A")
				tr.add(t, "K")
				tr.add(t, "U", "K")
				tr.add(t, "W")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U", "W"})
				verify(t, v, tr, "A")
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "w", Ref: tr["W"].Ref}}); err != nil {
					t.Fatal(err)
				}
				if err := rootOnto(t, v, tr, "U"); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				for range 4 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					res := collectAll(t, v)
					if res.Flipped {
						t.Fatalf("collect = %+v, want W to hold the flip", res)
					}
				}
				if countPrefix(t, store, parkedKey)+countPrefix(t, store, releasedKey) == 0 {
					t.Fatal("U is not parked")
				}
				return v, clk, store, tr
			},
			settle: func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree) {
				verify(t, v, tr, "W")
				settleAll(t, v, clk, store, tr)
			},
		}, {
			name: "unmarked while Sweeping",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				v, clk, store, tr := oldU(t, false)
				collectUntil(t, v, clk, flipped)
				if !has(t, store, run, tr["U"].Ref) {
					t.Fatal("U is not in the run")
				}
				return v, clk, store, tr
			},
			settle: func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree) {
				settleAll(t, v, clk, store, tr)
			},
		}}
		for _, st := range states {
			for _, byFill := range []bool{false, true} {
				name := st.name + " put by a Land"
				if byFill {
					name = st.name + " put by a fill"
				}
				t.Run(name, func(t *testing.T) {
					v, clk, store, tr := st.open(t)
					if byFill {
						fill(t, v, tr, "U", "K")
						if err := rootOnto(t, v, tr, "U"); err != nil {
							t.Fatal(err)
						}
					} else if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
						t.Fatal(err)
					}
					requireStored(t, v, tr, true, "U", "K")
					// A pass just inside the put's grace period nets the records
					// before it, and must keep U while the put waits past the cursor.
					clk.Add(block_gc.DefaultGrace - time.Second)
					collectAll(t, v)
					requireStored(t, v, tr, true, "U", "K")
					if st.settle != nil {
						st.settle(t, v, clk, store, tr)
					}
					for range 4 {
						clk.Add(block_gc.DefaultGrace + time.Second)
						collectAll(t, v)
					}
					requireStored(t, v, tr, true, "U", "K")
					requireAudit(t, v, 0)
				})
			}
		}
	})
}
