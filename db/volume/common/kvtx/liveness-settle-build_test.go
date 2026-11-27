package kvtx

import (
	"bytes"
	"encoding/binary"
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

// testLivenessSettleBuild checks the unmarked run and its front while Open.
func testLivenessSettleBuild(t *testing.T,
	adoptOldStore func(t *testing.T, store kvtx.Store, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock),
	oldStore func(t *testing.T, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock, kvtx.Store),
	has func(t *testing.T, store kvtx.Store, prefix []byte, ref *block.BlockRef) bool,
	fill func(t *testing.T, v *Volume, tr testTree, names ...string),
	rootOnto func(t *testing.T, v *Volume, tr testTree, name string, put ...string) error,
	verify func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult,
	requireKept func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree, below, path string),
	collectUntil func(t *testing.T, v *Volume, clk *testClock, done func(block_gc.CollectResult) bool) block_gc.CollectResult,
	flipped func(res block_gc.CollectResult) bool,
	pairs func(t *testing.T, tr testTree, n int) (listed, cut []string),
	buildRun func(t *testing.T, v *Volume, store kvtx.Store, during func()),
	oldU func(t *testing.T, cut bool) (*Volume, *testClock, kvtx.Store, testTree),
	passes func(t *testing.T, v *Volume, clk *testClock, n int),
	run []byte,
	front []byte,
) {
	t.Helper()

	t.Run("a park releases its children", func(t *testing.T) {
		// A -> L -> S, with P -> Q, S unheld beside them: P's list and Q's
		// are from content, and S stays on the front so the generation
		// stays Open. The free parks P and gives its children their downs:
		// Q, which only P holds, parks in the same pass, and S, which L
		// also holds, stays. Then either the flip and the sweep delete P
		// and Q and give S no second down, or a writer roots main onto P
		// and Q before the flip, and their rebirth gives S and Q back the
		// ups the park took. The old park, kept selectable, parks P alone
		// with its holds. Or a fill of X, which names P, unparks P with no
		// up for S and owes a recount; until it runs nothing is freed or
		// parked, since X's park would take P back to zero and P's second
		// park would give S, which L still names, a second down. Each way
		// the settle keeps what main reaches, and the audit finds every
		// count exact.
		for _, tc := range []struct {
			name   string
			old    bool
			reborn bool
			owed   bool
		}{
			{name: "swept"},
			{name: "reborn before the flip", reborn: true},
			{name: "with the park keeping its holds", old: true},
			{name: "unparked by a fill", owed: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "S")
				tr.add(t, "Q")
				tr.add(t, "P", "Q", "S")
				tr.add(t, "L", "S")
				tr.add(t, "A", "L")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "L", "P", "Q", "S"})
				if tc.old {
					swapCollector(v, block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, ParkKeepsHolds: true}))
				}
				verify(t, v, tr, "A", "L", "P", "Q")

				// One pass while Open parks P, and Q below it unless the
				// park keeps its holds.
				clk.Add(block_gc.DefaultGrace + time.Second)
				res := collectAll(t, v)
				parks := uint64(2)
				if tc.old {
					parks = 1
				}
				if res.Parked != parks || res.Freed != 0 || res.Flipped {
					t.Fatalf("collect while Open = %+v, want %d parked and no flip", res, parks)
				}
				requireReads(t, v, tr, "A", "L", "S")

				// While P is parked both audits leave a released list out of
				// its children's namers, so S's one holder, L, reads exact.
				// Lost and refilled, S is a hole L alone names, and the audit
				// adds it no holder. The park that keeps its holds leaves Q,
				// which no root reaches, held by P until the sweep.
				leaked := uint64(0)
				if tc.old {
					leaked = 1
				}
				requireAudit(t, v, leaked)
				scratch, err := openAuditScratch(t, "walk.db")
				if err != nil {
					t.Fatal(err)
				}
				defer scratch.Close()
				walk, err := v.AuditWalk(t.Context(), scratch, block_gc.WalkOptions{Run: "parked", Delay: -1})
				if err != nil || walk.Counts.GetUndercount() != 0 {
					t.Fatalf("walk while parked = %+v err %v, want no undercount", walk, err)
				}
				if !tc.old {
					loseBlock(t, v, tr["S"].Ref)
					if _, err := v.Audit(t.Context()); err != nil {
						t.Fatal(err)
					}
					if holders, _ := blockHolders(t, v, tr, "S"); holders != 1 {
						t.Fatalf("hole S under the released P has %d holders, want 1", holders)
					}
					fill(t, v, tr, "S")
					requireReads(t, v, tr, "S")
				}

				// A fill of X holds P again and owes the recount its
				// released list's children lack. The passes free and park
				// nothing until the flip's attempt unadopts, and S, which L
				// names, reads throughout.
				if tc.owed {
					tr.add(t, "X", "P")
					fill(t, v, tr, "X")
					netAll(t, v)
					if owed := livenessRecords(t, store, 'Y'); len(owed) == 0 {
						t.Fatal("the fill's unpark of the released P owes no recount")
					}
					res := collectAll(t, v)
					if !res.Unadopted {
						t.Fatalf("collect with a recount owed = %+v, want the readoption", res)
					}
					requireReads(t, v, tr, "A", "L", "S")
				}

				// Root main onto P and Q again, or leave them to the sweep,
				// and free what the settle released: the old park gives
				// P's downs only when the sweep deletes it.
				kept := []string{"A", "L", "S"}
				if tc.reborn {
					if err := rootOnto(t, v, tr, "P", "P", "Q"); err != nil {
						t.Fatal(err)
					}
					kept = []string{"P", "Q", "S"}
				}
				settleAll(t, v, clk, store, tr)
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				requireReads(t, v, tr, kept...)
				if !tc.reborn {
					requireStored(t, v, tr, false, "P", "Q")
				}
				if tc.owed {
					requireStored(t, v, tr, false, "X")
				}
				requireListsStored(t, v, tr)
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("frees off and on while the generation is open", func(t *testing.T) {
		// B -> C -> D, with C's list cut, none reached, and one
		// frees-on pass partway through the settle. Frees go off, and a
		// writer roots main onto a new N referencing one C. Frees come
		// back on, which recounts. Land keeps the settle's barriers by
		// the stored generation, not by the frees switch, so the walk
		// still finds D below C, and the settle keeps N, C and D.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		store := newGuardedStore(t)
		v1, _ := adoptOldStore(t, store, tr, "A", listed, cut...)
		verify(t, v1, tr, "A")
		if gen := livenessGeneration(t, v1); gen != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("the adoption left the generation %v, want it open", gen)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Frees off: the writer lands N, and the passes free nothing.
		v2, clk2 := openLivenessVolume(t, store, false)
		tr.add(t, "N", "C7")
		if err := rootOnto(t, v2, tr, "N", "N"); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			clk2.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v2); res.Freed != 0 || res.Swept != 0 {
				t.Fatalf("a pass with frees off freed %d and swept %d", res.Freed, res.Swept)
			}
		}
		requireFront(t, v2)
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Frees on again: the recount, then the settle.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "N", "C7", "D7")
		requireFront(t, v)
		requireAudit(t, v, 0)
	})

	t.Run("reader sight across an adoption while the generation is open", func(t *testing.T) {
		// R1 -> E in an old store, its generation Open, R1 its world
		// root. A reader sees R1, the root moves to R2, and the Volume
		// adopts again before the reader pins R1: the adoption drops
		// the record of the move, so no seed of the settle walk reaches
		// R1. Passes within the grace period walk, and the flip, which
		// would unmark R1 for the sweep, waits for the adoption's
		// marker. The reader pins R1 and the settle keeps it.
		tr := testTree{}
		tr.add(t, "E")
		tr.add(t, "R1", "E")
		v, clk, store := oldStore(t, tr, "R1", []string{"E", "R1"})
		tr.add(t, "R2")
		world := []block.NamedRoot{{Name: "world", Ref: tr["R2"].Ref, Previous: tr["R1"].Ref}}
		if err := v.SetBucketRoots(t.Context(), "space", tr.entries("R2"), world); err != nil {
			t.Fatal(err)
		}
		markUnadopted(t, v)
		if _, err := v.Adopt(t.Context()); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			walkFront(t, v, store, tr)
			collectAll(t, v)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2")
		unpin, err := v.PinBucketRoot(t.Context(), tr["R1"].Ref)
		if err != nil {
			t.Fatal(err)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// Once the pin is released, the old tree goes after the grace
		// period.
		unpin()
		if err := v.withDirectAtomic(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("put again while the run builds", func(t *testing.T) {
		// B -> C -> D, with C's list cut, none reached. Between two slices
		// of the run's build, a session puts again a B the build entered,
		// whose C it has not, and moves main onto it. The put marks B's list
		// and pushes C onto the front, which holds the flip until the walk
		// decodes C and finds D.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk, store := oldStore(t, tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		var pick string
		buildRun(t, v, store, func() {
			for i := range 150 {
				s := strconv.Itoa(i)
				if has(t, store, run, tr["B"+s].Ref) && !has(t, store, run, tr["C"+s].Ref) {
					pick = s
					break
				}
			}
			if pick == "" {
				t.Fatal("no B entered with its C ahead")
			}
			if err := rootOnto(t, v, tr, "B"+pick, "B"+pick); err != nil {
				t.Fatal(err)
			}
		})
		if has(t, store, run, tr["B"+pick].Ref) || !has(t, store, front, tr["C"+pick].Ref) {
			t.Fatal("the put left B in the run or C off the front")
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("generation %v with C on the front, want Open", gen)
		}
		requireKept(t, v, clk, store, tr, "D"+pick, "B"+pick+" and C"+pick+"'s short list")
	})

	t.Run("fill while the run builds", func(t *testing.T) {
		// Between two slices of the run's build, a read fills X
		// referencing a C the build has not entered, with X's key
		// behind the build's position, and main moves onto X. The fill
		// pushes C onto the front.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk, store := oldStore(t, tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		var x, c string
		buildRun(t, v, store, func() {
			var behind []byte
			tx, err := store.NewTransaction(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			err = tx.ScanPrefixKeys(t.Context(), run, func(key []byte) error {
				if k := key[len(run):]; bytes.Compare(k, behind) > 0 {
					behind = bytes.Clone(k)
				}
				return nil
			})
			tx.Discard()
			if err != nil {
				t.Fatal(err)
			}
			for i := range 150 {
				if s := strconv.Itoa(i); !has(t, store, run, tr["C"+s].Ref) {
					c = "C" + s
					break
				}
			}
			for j := 0; c != "" && x == ""; j++ {
				name := "X" + strconv.Itoa(j)
				if bytes.Compare(block_gc.BlockKey(tr.add(t, name, c)), behind) < 0 {
					x = name
				}
			}
			if x == "" {
				t.Fatal("no C ahead of the build")
			}
			fill(t, v, tr, x)
			if err := rootOnto(t, v, tr, x); err != nil {
				t.Fatal(err)
			}
		})
		if !has(t, store, front, tr[c].Ref) {
			t.Fatalf("the fill of %s left %s off the front", x, c)
		}
		requireKept(t, v, clk, store, tr, "D"+c[1:], x+" and "+c+"'s short list")
	})

	// cutC builds A and C -> F with C's list cut, adopts, and verifies A.
	cutC := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "C", "F")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "F", "C"}, "C")
		verify(t, v, tr, "A")
		return v, clk, store, tr
	}

	t.Run("fill top down", func(t *testing.T) {
		// A read fills P while its child X is absent, then fills X,
		// which references the old C. Main then moves onto P.
		v, clk, store, tr := cutC(t)
		tr.add(t, "X", "C")
		tr.add(t, "P", "X")
		fill(t, v, tr, "P")
		fill(t, v, tr, "X")
		if err := rootOnto(t, v, tr, "P"); err != nil {
			t.Fatal(err)
		}
		requireKept(t, v, clk, store, tr, "F", "P, X and C's short list")
	})

	t.Run("fill under a held block", func(t *testing.T) {
		// Main holds B, whose child X is absent. A read then fills X,
		// which references the old C.
		v, clk, store, tr := cutC(t)
		tr.add(t, "X", "C")
		tr.add(t, "B", "X")
		if err := rootOnto(t, v, tr, "B", "B"); err != nil {
			t.Fatal(err)
		}
		fill(t, v, tr, "X")
		requireKept(t, v, clk, store, tr, "F", "B, X and C's short list")
	})

	t.Run("refilled hole", func(t *testing.T) {
		// Main holds A -> H -> C -> D, with C's list cut. The store loses H
		// and an audit records the hole, so the walk from A pushes H, which
		// no read resolves, and C stays unreached. H's front entry holds the
		// flip until a put refills H, which pushes C.
		tr := testTree{}
		tr.add(t, "D")
		tr.add(t, "C", "D")
		h := tr.add(t, "H", "C")
		tr.add(t, "A", "H")
		v, clk, store := oldStore(t, tr, "A", []string{"D", "C", "H", "A"}, "C")
		punchHole(t, v, h)
		if res, err := v.Audit(t.Context()); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		verify(t, v, tr, "A")
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || countPrefix(t, store, front) != 1 || !has(t, store, front, h) {
			t.Fatalf("collect with a hole = %+v, want no flip and only H on the front", res)
		}
		if _, err := v.PutBlockBatch(t.Context(), tr.entries("H")); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["C"].Ref) {
			t.Fatal("the refill of H left C off the front")
		}
		requireKept(t, v, clk, store, tr, "D", "H and C's short list")
	})

	for _, late := range []bool{false, true} {
		t.Run("written-off hole, put after the sweep="+strconv.FormatBool(late), func(t *testing.T) {
			// Main holds A -> H -> C -> D, A -> B -> H and A -> Q, with
			// C's list cut. The store loses H and Q, and an audit records
			// both holes, which hold the flip. An operator writes off H
			// alone: H leaves the front, the walk of B does not push it
			// again, and Q still holds the flip until a put refills it. A put of H from content
			// before the flip is the refill, which pushes C, so D stays.
			// Without one the flip passes H and the sweep deletes C and D,
			// which no walk reached, the loss the operator accepted. A put
			// of H, C and D from content after it ends the hole, and H's
			// list holds C from then on.
			tr := testTree{}
			tr.add(t, "D")
			tr.add(t, "C", "D")
			h := tr.add(t, "H", "C")
			q := tr.add(t, "Q")
			tr.add(t, "B", "H")
			tr.add(t, "A", "H", "Q", "B")
			v, clk, store := oldStore(t, tr, "A", []string{"D", "C", "H", "Q", "B", "A"}, "C")
			ctx := t.Context()
			punchHole(t, v, h)
			punchHole(t, v, q)
			if res, err := v.Audit(ctx); err != nil || res.Holes != 2 {
				t.Fatalf("audit recorded %d holes err %v, want 2", res.Holes, err)
			}
			verify(t, v, tr, "A")
			holes := func() (int, int) {
				t.Helper()
				st, err := v.SettleStatus(ctx, 8)
				if err != nil {
					t.Fatal(err)
				}
				return st.Holes, st.OpenHoles
			}

			// A list naming a block that is no hole writes off nothing.
			if _, err := v.WriteOffHoles(ctx, []*block.BlockRef{h, tr["D"].Ref}); !errors.Is(err, block_gc.ErrNotHole) {
				t.Fatalf("write-off naming the stored D err %v, want refused", err)
			}
			if n, open := holes(); n != 2 || open != 2 || !has(t, store, front, h) {
				t.Fatalf("the refused write-off left %d holes, %d open, want both open and H on the front", n, open)
			}

			// Write off H, twice: the second changes nothing.
			for range 2 {
				if res, err := v.WriteOffHoles(ctx, []*block.BlockRef{h}); err != nil || res.WrittenOff != 1 || res.Ended != 0 {
					t.Fatalf("write-off of H = %+v err %v, want one hole written off", res, err)
				}
			}
			if n, open := holes(); n != 2 || open != 1 || has(t, store, front, h) || !has(t, store, front, q) {
				t.Fatalf("after the write-off %d holes, %d open, want Q's open and only Q on the front", n, open)
			}

			// The walk of B, which names H, pushes nothing, and Q holds
			// the flip.
			verify(t, v, tr, "B")
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Flipped || has(t, store, front, h) {
				t.Fatalf("collect with Q's hole open = %+v, want no flip and H off the front", res)
			}

			// A put of H before the flip is the refill.
			if !late {
				if _, err := v.PutBlockBatch(ctx, tr.entries("H")); err != nil {
					t.Fatal(err)
				}
				if n, _ := holes(); n != 1 || !has(t, store, front, tr["C"].Ref) {
					t.Fatalf("the refill of H left %d holes, want Q's, and C on the front", n)
				}
			}

			// Refill Q and settle.
			if _, err := v.PutBlockBatch(ctx, tr.entries("Q")); err != nil {
				t.Fatal(err)
			}
			settleAll(t, v, clk, store, tr)
			requireStored(t, v, tr, true, "A", "Q", "B")
			requireStored(t, v, tr, !late, "H", "C", "D")
			if n, open := holes(); late && (n != 1 || open != 0) {
				t.Fatalf("after the sweep %d holes, %d open, want H's written off", n, open)
			}

			// Put the lost tree from content and let two grace periods
			// pass.
			if late {
				if _, err := v.PutBlockBatch(ctx, tr.entries("D", "C", "H")); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
			}
			if n, _ := holes(); n != 0 {
				t.Fatalf("%d holes after every put, want none", n)
			}
			requireReads(t, v, tr, "A", "H", "C", "D", "Q", "B")
			requireAudit(t, v, 0)
		})
	}

	t.Run("hole written off before the adoption", func(t *testing.T) {
		// A Volume that predates adoption holds main A -> H -> C -> D
		// and A -> B -> H, with C's list cut. Opened with frees off, as
		// an operator's tool opens it, it loses H, an audit records the
		// hole and the operator writes it off. The next open turns frees
		// on, which returns the Volume to unadopted, and its recount runs
		// over the written-off hole. The hole stays written off and off
		// the front, the flip passes it, and the sweep deletes C and D,
		// which no walk reached. A put of H, C and D from content then
		// ends the hole, and H's list holds C from then on.
		tr := testTree{}
		tr.add(t, "D")
		tr.add(t, "C", "D")
		h := tr.add(t, "H", "C")
		tr.add(t, "B", "H")
		tr.add(t, "A", "H", "B")
		store := newGuardedStore(t)
		ctx := t.Context()
		writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, "A", true, "D", "H", "B", "A")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		cut := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(tr["C"].Ref))
		if err := tx.Set(ctx, cut, block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// Lose H, record its hole and write it off with frees off.
		v1, _ := openLivenessVolume(t, store, false)
		punchHole(t, v1, h)
		if res, err := v1.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		if res, err := v1.WriteOffHoles(ctx, []*block.BlockRef{h}); err != nil || res.WrittenOff != 1 {
			t.Fatalf("write-off of H = %+v err %v, want one hole written off", res, err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Turn frees on and let the recount run.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		holes := func() (int, int) {
			t.Helper()
			st, err := v.SettleStatus(ctx, 8)
			if err != nil {
				t.Fatal(err)
			}
			return st.Holes, st.OpenHoles
		}
		if n, open := holes(); n != 1 || open != 0 || has(t, store, front, h) {
			t.Fatalf("the recount left %d holes, %d open, want H's written off and off the front", n, open)
		}

		// The flip passes the hole, and the sweep takes what it hid.
		if res := settleAll(t, v, clk, store, tr); !res.Flipped {
			t.Fatalf("settle = %+v, want a flip", res)
		}
		requireStored(t, v, tr, true, "A", "B")
		requireStored(t, v, tr, false, "H", "C", "D")
		if n, open := holes(); n != 1 || open != 0 {
			t.Fatalf("after the sweep %d holes, %d open, want H's written off", n, open)
		}

		// Put the lost tree from content and let two grace periods pass.
		if _, err := v.PutBlockBatch(ctx, tr.entries("D", "C", "H")); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		if n, _ := holes(); n != 0 {
			t.Fatalf("%d holes after the put, want none", n)
		}
		requireReads(t, v, tr, "A", "H", "C", "D", "B")
		requireAudit(t, v, 0)
	})

	t.Run("root onto an unverified block while Open", func(t *testing.T) {
		// A root moves onto the unreached U without a put. The hold
		// pushes U onto the front, which holds the flip. The walk
		// decodes U, and K stays. Before the hold nets, U's count is
		// zero at a ripe sequence the cursor passed, so only its
		// dequeue by the Land keeps the free proof from popping U.
		v, clk, store, tr := oldU(t, false)
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the hold left U off the front")
		}
		if res := collectAll(t, v); res.Popped != 0 || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect before the hold nets = %+v, want U kept on the front", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || res.Popped != 0 {
			t.Fatalf("collect with U held on the front = %+v, want no pop or flip", res)
		}
		requireKept(t, v, clk, store, tr, "K", "U")
	})

	t.Run("born block's hold onto an unverified block while Open", func(t *testing.T) {
		// A put stores N, which holds the unreached U. The hold pushes U onto
		// the front. Before the hold nets, U's count is zero at a ripe
		// sequence the cursor passed, so only its dequeue by the Land, which
		// the hold onto a stored block at zero makes, keeps the free proof
		// from popping U.
		v, clk, store, tr := oldU(t, false)
		tr.add(t, "N", "U")
		roots := []block.NamedRoot{{Name: "n", Ref: tr["N"].Ref}}
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("N"), roots); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the hold left U off the front")
		}
		if res := collectAll(t, v); res.Popped != 0 || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect before the hold nets = %+v, want U kept on the front", res)
		}
		requireKept(t, v, clk, store, tr, "K", "U")
	})

	t.Run("front entry the free proof pops", func(t *testing.T) {
		// A root moves onto U and away again. Once its release nets and
		// ages, the free proof holds for U: the collector takes U off the
		// front unwalked, flips, and sweeps U and K, which nothing holds.
		// While U is held, a whole front scan leaves the front idle, so a
		// pass with nothing changed reads no entry; the netting that queues
		// U at zero wakes the scan, or U would hold the flip forever.
		v, clk, store, tr := oldU(t, false)
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Popped != 0 || res.Flipped {
			t.Fatalf("collect with U held = %+v, want no pop or flip", res)
		}
		for range 2 {
			if res, err := v.Collect(t.Context()); err != nil || res.FrontScanned != 0 {
				t.Fatalf("pass over an idle front = %+v, %v, want no entry read", res, err)
			}
		}
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		var total block_gc.CollectResult
		collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			total.Popped += res.Popped
			total.Swept += res.Swept
			return res.Settled
		})
		if total.Popped != 1 || total.Swept != 2 || countPrefix(t, store, front) != 0 {
			t.Fatalf("settle popped %d and swept %d, want U popped and U and K swept", total.Popped, total.Swept)
		}
		requireStored(t, v, tr, true, "A")
		requireStored(t, v, tr, false, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("front scan resumes across passes", func(t *testing.T) {
		// A front larger than one pass reads takes several passes to scan,
		// each resuming where the last stopped. A root u holds the old U,
		// which the hold pushes onto the front, so the free that runs before
		// the scan does not park U and K. Fillers are block keys that sort
		// after U and K, each listed with no queue entry, so they stay held
		// and the scan is past K once it reads one. Mid-scan a root moves
		// onto K, pushing it behind the cursor, an entry lands ahead of it
		// and one ahead is removed: the scan that ends reads every entry
		// ahead and none behind, and the front it leaves holds K, so it
		// must not flip, or the sweep would take K under main. A collector
		// that restarts mid-scan begins again at the head and counts no
		// partial scan as done.
		v, clk, store, tr := oldU(t, false)
		ctx := t.Context()
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "u", Ref: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the hold left U off the front")
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		last := slices.MaxFunc([][]byte{block_gc.BlockKey(tr["U"].Ref), block_gc.BlockKey(tr["K"].Ref)}, bytes.Compare)
		// after returns a block key past U's and K's whose low two bytes are
		// n, so the keys sort by n.
		if last[len(last)-3] == 0xff {
			t.Fatal("the last key leaves no room after it")
		}
		after := func(n uint16) []byte {
			key := slices.Clone(last)
			key[len(key)-3]++
			binary.BigEndian.PutUint16(key[len(key)-2:], n)
			return key
		}
		children := slices.Concat(livenessPrefix, []byte{'c'})
		write := func(key []byte, del bool) {
			t.Helper()
			tx, err := store.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			for _, k := range [][]byte{slices.Concat(front, key), slices.Concat(children, key)} {
				if del {
					err = tx.Delete(ctx, k)
				} else {
					err = tx.Set(ctx, k, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		filler := func(i int) []byte { return after(uint16(i)) }
		const fillers = 300
		for i := range fillers {
			write(filler(i), false)
		}
		ahead := after(0xffff)

		// newCollector returns a collector whose every slice outlasts the
		// pass's budget and the slice target, so each pass runs one slice
		// and the pace shrinks the slices to their least.
		newCollector := func() *block_gc.Collector {
			var n time.Duration
			return block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{
				Frees:  true,
				Budget: time.Nanosecond,
				Now: func() time.Time {
					n++
					return clk.Now().Add(n * 3 * time.Millisecond)
				},
			})
		}
		// scan runs passes of c until stop reports true of a pass, and
		// returns the front entries the passes read.
		scan := func(c *block_gc.Collector, stop func(block_gc.CollectResult) bool) (uint64, block_gc.CollectResult) {
			t.Helper()
			var read uint64
			for range 10000 {
				res, err := c.Collect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if res.Flipped {
					t.Fatalf("pass = %+v flipped with U on the front", res)
				}
				if read += res.FrontScanned; stop(res) {
					return read, res
				}
			}
			t.Fatal("the front scan never stopped")
			return 0, block_gc.CollectResult{}
		}
		done := func(res block_gc.CollectResult) bool { return res.FrontDone }

		// Read U and part of the fillers, then change the front around the
		// cursor.
		c := newCollector()
		var first uint64
		read, res := scan(c, func(res block_gc.CollectResult) bool {
			first += res.FrontScanned
			return first >= 2
		})
		if res.FrontDone || read >= fillers {
			t.Fatalf("first front passes read %d of %d (%+v), want part", read, fillers+1, res)
		}
		if err := rootOnto(t, v, tr, "K"); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["K"].Ref) {
			t.Fatal("the hold left K off the front")
		}
		write(ahead, false)
		write(filler(fillers-1), true)

		// The scan ends having read each entry ahead once and K not at all.
		rest, res := scan(c, done)
		if read+rest != fillers+1 || res.Front != fillers+1 {
			t.Fatalf("scan read %d and kept %d, want each of %d entries once", read+rest, res.Front, fillers+1)
		}

		// A restart mid-scan reads from the head, K included, and only a
		// whole scan reports done.
		_, res = scan(newCollector(), func(res block_gc.CollectResult) bool { return res.FrontScanned != 0 })
		if res.FrontDone {
			t.Fatalf("first pass after a restart = %+v, want a part of the front", res)
		}
		c = newCollector()
		read, res = scan(c, done)
		if read != fillers+2 || res.Front != fillers+2 {
			t.Fatalf("scan after a restart read %d and kept %d, want all %d entries from the head", read, res.Front, fillers+2)
		}

		// Once the fillers go, the settle walks U and K and keeps K under
		// main.
		for i := range fillers - 1 {
			write(filler(i), true)
		}
		write(ahead, true)
		requireKept(t, v, clk, store, tr, "K", "main")
		requireStored(t, v, tr, true, "A", "U", "K")
	})
}
