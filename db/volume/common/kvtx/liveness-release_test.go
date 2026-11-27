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
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// testLivenessRelease checks deferred releases and blocks shared across intervals.
func testLivenessRelease(t *testing.T) {
	t.Helper()

	for _, tc := range []struct {
		who, ripe string
		restart   bool
	}{
		{"F again", "unripe", false}, {"F again", "unripe behind a record", false}, {"F again", "ripe", false},
		{"a new parent", "unripe", false}, {"a new parent", "unripe behind a record", false}, {"a new parent", "ripe", false},
		{"a new parent", "unripe behind a record", true},
	} {
		ripe, title := tc.ripe, "born put over children of an inline free "+tc.who+" "+tc.ripe
		if tc.restart {
			title += " across a collector restart"
		}
		t.Run(title, func(t *testing.T) {
			// F references three leaves, few enough to free inline,
			// and root g holds G. Root f releases F, and the
			// collector stops once F is freed with its leaves still
			// stored. A writer then puts F again, born since its
			// bytes and list are gone, or a new parent P,
			// referencing the leaves: right after the free, so its
			// record is the first after the free, or behind a
			// younger root move onto G that nets first, or after
			// that move netted, so the leaves are ripe. The same
			// collector then runs on, its tail index extended from
			// the free, or with restart a new one, whose index is
			// read whole from the cursor. Every leaf the parent
			// lists must still read once the collector is quiet.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			leaves := []string{"leaf 0", "leaf 1", "leaf 2"}
			for _, name := range leaves {
				tr.add(t, name)
			}
			f := tr.add(t, "F", leaves...)
			np := tr.add(t, "P", leaves...)
			gr := tr.add(t, "G")
			roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "g", Ref: gr}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "G")...), roots); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			stored := func(name string) bool {
				_, found, err := v.GetBlock(ctx, tr[name].Ref)
				if err != nil {
					t.Fatal(err)
				}
				return found
			}
			// Stop once F is freed.
			resumed := false
			probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, until: func(*block_gc.LivenessState) bool {
				return !resumed && !stored("F")
			}}
			opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
			c := block_gc.NewCollector(probe, opts)
			res, err := c.Collect(ctx)
			if !errors.Is(err, errMidRelease) {
				t.Fatalf("collect = %+v err %v, want it stopped once F is freed", res, err)
			}
			freed := res.Freed
			left := 0
			for _, name := range leaves {
				if stored(name) {
					left++
				}
			}
			t.Logf("stopped with F stored %t and %d of %d leaves stored", stored("F"), left, len(leaves))

			// A root move onto G writes a record after the free.
			if ripe != "unripe" {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g2", Ref: gr}}); err != nil {
					t.Fatal(err)
				}
			}
			if ripe == "ripe" {
				clk.Add(block_gc.DefaultGrace + time.Second)
				nets := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
				if _, err := nets.Collect(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if ripe == "unripe behind a record" {
				clk.Add(block_gc.DefaultGrace / 2)
			}
			parent, ref := "F", f
			if tc.who == "a new parent" {
				parent, ref = "P", np
			}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(parent), []block.NamedRoot{{Name: "p", Ref: ref}}); err != nil {
				t.Fatal(err)
			}

			// Collect to quiescence across the grace periods, with the stopped
			// collector resumed or a new one.
			resumed, probe.stopped = true, false
			if tc.restart {
				c = block_gc.NewCollector(collectorStore{v: v}, opts)
			}
			for range 4 {
				clk.Add(block_gc.DefaultGrace/2 + time.Second)
				for more := true; more; {
					res, err := c.Collect(ctx)
					if err != nil {
						t.Fatal(err)
					}
					freed, more = freed+res.Freed, res.More
				}
			}
			audit, err := v.Audit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			lost := 0
			for _, name := range leaves {
				if !stored(name) {
					lost++
				}
			}
			t.Logf("freed %d, holes %d, leaves lost %d of %d, %s stored %t", freed, audit.Holes, lost, len(leaves), parent, stored(parent))
			if lost != 0 || !stored(parent) {
				t.Errorf("%s lists %d freed leaves", parent, lost)
			}
		})
	}

	for _, mid := range []string{"a crash", "children held again", "a second parent's release", "the releasing block put again", "a crash mid put again", "the releasing block lost"} {
		t.Run("release interrupted by "+mid, func(t *testing.T) {
			// F references 6,000 leaves, more than one slice moves,
			// and G references the first and the last of them in
			// F's list. Root f releases F, and the collector stops
			// once a transaction left F's paged release in
			// progress, with the first leaves released and the last
			// not, and F Dying, stored until its release ends.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			leaves := make([]string, 6_000)
			for i := range leaves {
				leaves[i] = "leaf " + strconv.Itoa(i)
				tr.add(t, leaves[i])
			}
			slices.SortFunc(leaves, func(a, b string) int {
				return bytes.Compare(block_gc.BlockKey(tr[a].Ref), block_gc.BlockKey(tr[b].Ref))
			})
			first, last := leaves[0], leaves[len(leaves)-1]
			f := tr.add(t, "F", leaves...)
			g := tr.add(t, "G", first, last)
			roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "g", Ref: g}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "G")...), roots); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			probe := &stopMidRelease{Store: collectorStore{v: v}, v: v}
			c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
			if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
				t.Fatalf("collect = %v, want it stopped mid-release", err)
			}
			// The free deletes F in the transaction that starts its paged
			// release, which then carries F's downs.
			requireStored(t, v, tr, false, "F")

			// Act while the release is in progress, then collect to the end
			// and once more after every record ages.
			kept := []string{"G", first, last}
			var pages [2]uint64
			switch mid {
			case "children held again":
				err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k1", Ref: tr[leaves[1]].Ref}, {Name: "k2", Ref: tr[leaves[len(leaves)-2]].Ref}})
				if err != nil {
					t.Fatal(err)
				}
				kept = append(kept, leaves[1], leaves[len(leaves)-2])
			case "a second parent's release":
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
					t.Fatal(err)
				}
				kept = nil
			case "the releasing block put again", "a crash mid put again":
				pages[0] = livenessState(t, v).GetUpSeq()
				if err := v.SetBucketRoots(ctx, "b", tr.entries("F"), []block.NamedRoot{{Name: "f", Ref: f}}); err != nil {
					t.Fatal(err)
				}
				pages[1] = livenessState(t, v).GetUpSeq()
				kept = append(slices.Clone(leaves), "F", "G")
			case "the releasing block lost":
				punchHole(t, v, f)
			}
			// The free already deleted F, so the put again is born,
			// and its up pages, which give the leaves back their
			// holder, net across slices. Stop with them partly
			// netted.
			if mid == "a crash mid put again" {
				probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, pages: pages}
				c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
				if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
					t.Fatalf("collect = %v, want it stopped with the put again's ups partly netted", err)
				}
			}
			res := collectAll(t, v)
			clk.Add(block_gc.DefaultGrace + time.Second)
			if more := collectAll(t, v); res.Underflows+more.Underflows != 0 {
				t.Fatalf("%d underflows releasing F", res.Underflows+more.Underflows)
			}

			// Exactly the blocks still held stay, and the counts are exact.
			gone := make([]string, 0, len(leaves))
			for _, name := range append(slices.Clone(leaves), "F", "G") {
				if !slices.Contains(kept, name) {
					gone = append(gone, name)
				}
			}
			requireStored(t, v, tr, true, kept...)
			requireStored(t, v, tr, false, gone...)
			requireAudit(t, v, 0)
		})
	}

	t.Run("hole under a release in progress", func(t *testing.T) {
		// F references 6,000 leaves, and P1 and P2 reference its last
		// leaf, H. Root f releases F, and the collector stops with F's
		// paged release not yet at H. The audit then counts the release
		// among H's holders and leaks none of the leaves it has not
		// reached. The store loses H, and the audit records the hole
		// with F's holder in its count, so once the release reaches H,
		// H is put again and P1 is freed, P2 still holds H.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 6_000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		slices.SortFunc(leaves, func(a, b string) int {
			return bytes.Compare(block_gc.BlockKey(tr[a].Ref), block_gc.BlockKey(tr[b].Ref))
		})
		h := leaves[len(leaves)-1]
		f := tr.add(t, "F", leaves...)
		p1 := tr.add(t, "P1", h)
		p2 := tr.add(t, "P2", h)
		roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "P1", "P2")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
			t.Fatalf("collect = %v, want it stopped mid-release", err)
		}
		// The free already deleted F. Its release pages carry its
		// downs.
		requireStored(t, v, tr, false, "F")
		requireStored(t, v, tr, true, h)
		requireAudit(t, v, 0)

		// Lose H and audit, finish the release, and put H again.
		punchHole(t, v, tr[h].Ref)
		if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		collectAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(h), nil); err != nil {
			t.Fatal(err)
		}

		// Free P1: P2 still holds H.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "p1", Previous: p1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P2", h)
		requireStored(t, v, tr, false, "F", "P1", leaves[0])
		requireAudit(t, v, 0)
	})

	t.Run("hole under a deferred release", func(t *testing.T) {
		// D1 references E1 and D2 references 4,000 leaves, and roots d1
		// and d2 let them go in commits one slice nets. D1's free moves
		// E1 first, so D2's list no longer fits the slice and its free
		// waits for the next. The store then loses D2 with its list, so
		// D2 is never freed, its leaves keeping its holder as leaked
		// garbage, and the frees go on: G, let go after, is freed.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E1")
		d1 := tr.add(t, "D1", "E1")
		leaves := make([]string, 4_000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		d2 := tr.add(t, "D2", leaves...)
		g := tr.add(t, "G")
		roots := []block.NamedRoot{{Name: "d1", Ref: d1}, {Name: "d2", Ref: d2}, {Name: "g", Ref: g}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "E1", "D1", "D2", "G")...), roots); err != nil {
			t.Fatal(err)
		}
		for _, root := range []block.NamedRoot{{Name: "d1", Previous: d1}, {Name: "d2", Previous: d2}} {
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{root}); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		// Stop once D1 is freed and D2 still waits.
		stored := func(name string) bool {
			_, found, err := v.GetBlock(ctx, tr[name].Ref)
			if err != nil {
				t.Fatal(err)
			}
			return found
		}
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, until: func(*block_gc.LivenessState) bool { return !stored("D1") && stored("D2") }}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
			t.Fatalf("collect = %v, want it stopped with D2's free waiting", err)
		}

		// Lose D2, let G go, and collect past the grace period.
		punchHole(t, v, d2)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "D1", "E1", "G")
		requireStored(t, v, tr, true, leaves[0], leaves[len(leaves)-1])
		requireAudit(t, v, uint64(len(leaves)))
	})

	t.Run("format 9 upgrade over a stale queue and a crash mid carry", func(t *testing.T) {
		// The format 9 store of format9Owed, whose upgrade crashed
		// after carrying one count record, opens with frees on. Before
		// the adoption a writer stores N -> C under a new root and
		// releases main, so A and B die while C keeps N. The stale
		// queue entries reference the live S and C, the owed releases
		// would move C down twice, and the rerun carry meets a record
		// already carried: none may free a block a root reaches or
		// leave a dead one with an invented holder.
		store := newGuardedStore(t)
		keys := store_kvkey.NewDefaultKVKey()
		tr := format9Owed(t)
		writeFormat9Owed(t, store, keys, tr)
		tx, err := store.NewTransaction(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := block_gc.NewRecords(tx, livenessPrefix).CarryFormat9Counts(t.Context(), nil, 1); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		v, clk := openLivenessVolume(t, store, true)
		n := tr.add(t, "N", "C")
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("N"), []block.NamedRoot{
			{Name: "n", Ref: n},
			{Name: "main", Previous: tr["A"].Ref},
		}); err != nil {
			t.Fatal(err)
		}
		waitAdopted(t, v)
		for range 3 {
			collectSettled(t, v, clk)
		}
		requireStored(t, v, tr, true, "N", "C", "S")
		requireStored(t, v, tr, false, "A", "B", "P", "G", "H2")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders, want N's alone", holders)
		}
		requireAudit(t, v, 0)
	})

	t.Run("root lands on a held child between the netting and the free", func(t *testing.T) {
		// Roots q and z let Q -> X and Z go. Once a pass nets that, and
		// before it frees, a root moves onto X, which Q still holds, so the
		// Land leaves X's count alone and logs an up. Q's free then takes X
		// to zero at the up log's end, which the up cursor has not reached,
		// so with a writer waiting, which frees one block a slice, the
		// slices that follow keep X until the up nets.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		x := tr.add(t, "X")
		q := tr.add(t, "Q", "X")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Q", "Z"), []block.NamedRoot{{Name: "q", Ref: q}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "q", Previous: q}, {Name: "z", Previous: z}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		rooted := false
		root := func(p block_gc.Phase) func() {
			if p == block_gc.PhaseSweep && !rooted {
				rooted = true
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x", Ref: x}}); err != nil {
					t.Error(err)
				}
			}
			return func() {}
		}
		waiting := func() bool { return true }
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: root, WritersWaiting: waiting})
		if res, err := c.Collect(ctx); err != nil || !rooted || res.Freed != 2 {
			t.Fatalf("collect freed %d err %v rooted %v, want Q and Z freed and X kept", res.Freed, err, rooted)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X")
		requireStored(t, v, tr, false, "Q", "Z")
		requireAudit(t, v, 0)
	})

	t.Run("two born parents name one stored child", func(t *testing.T) {
		// Root c holds C. One Land stores P1 and P2, each referencing
		// C, under roots p1 and p2, which gives C two ups, one for each
		// edge. Once root c and root p1 let go and P1 is freed, P2
		// still holds C.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		cref := tr.add(t, "C")
		p1 := tr.add(t, "P1", "C")
		p2 := tr.add(t, "P2", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: cref}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P1", "P2"), []block.NamedRoot{{Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "c", Previous: cref}, {Name: "p1", Previous: p1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("freed %d, want P1 alone", res.Freed)
		}
		requireStored(t, v, tr, true, "P2", "C")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders, want P2's", holders)
		}
		requireAudit(t, v, 0)
	})

	t.Run("fill lands between the netting and the free", func(t *testing.T) {
		// Root c lets C go. Once a pass nets that, and before it frees,
		// a read fills P, which references C. The free waits for the
		// fill's up, so a root that moves onto P after the grace period
		// finds C.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		cref := tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: cref}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "c", Previous: cref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		filled := false
		fill := func(p block_gc.Phase) func() {
			if p == block_gc.PhaseSweep && !filled {
				filled = true
				err := v.withDirectFill(ctx, func(px *physicalTx) error {
					_, _, err := v.landFill(ctx, px, "b", tr.entries("P"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			}
			return func() {}
		}
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: fill})
		if res, err := c.Collect(ctx); err != nil || !filled || res.Freed != 0 {
			t.Fatalf("collect freed %d err %v filled %v, want P filled and C kept", res.Freed, err, filled)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
	})

	for _, landed := range []bool{true, false} {
		t.Run("holder landed after an audit's snapshot landed="+strconv.FormatBool(landed), func(t *testing.T) {
			// P1 and P2 reference H under roots p1 and p2, and the
			// store loses H. An audit tallies P1 and P2 in its
			// snapshot. With landed set, the counts hold both, and
			// before the audit records the hole a writer lands P3
			// referencing H, which the collector nets. Otherwise
			// the up pair of H's second holder is still in the up
			// log at the snapshot and nothing lands. Either way the
			// audit records H with its true holders: once H is put
			// again and P1 and P2 are freed, P3 or nothing holds H,
			// and H stays while P3 does and is freed with it.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			h := tr.add(t, "H")
			p1 := tr.add(t, "P1", "H")
			p2 := tr.add(t, "P2", "H")
			p3 := tr.add(t, "P3", "H")
			roots := []block.NamedRoot{{Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "P1", "P2"), roots); err != nil {
				t.Fatal(err)
			}
			if landed {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
			}
			punchHole(t, v, h)

			// Audit, landing P3 between the snapshot and the hole's record.
			probe := &auditProbe{collectorStore: collectorStore{v: v}}
			if landed {
				probe.before = func() {
					if err := v.SetBucketRoots(ctx, "b", tr.entries("P3"), []block.NamedRoot{{Name: "p3", Ref: p3}}); err != nil {
						t.Error(err)
					}
					clk.Add(block_gc.DefaultGrace + time.Second)
					collectAll(t, v)
				}
			}
			if res, err := block_gc.Audit(ctx, probe, clk.Now(), true); err != nil || res.Holes != 1 {
				t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
			}

			// Put H again and free P1 and P2.
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H"), nil); err != nil {
				t.Fatal(err)
			}
			release := func(name string, ref *block.BlockRef) {
				t.Helper()
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: name, Previous: ref}}); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					collectAll(t, v)
				}
			}
			release("p1", p1)
			release("p2", p2)
			requireStored(t, v, tr, false, "P1", "P2")
			if landed {
				requireStored(t, v, tr, true, "P3", "H")
				requireAudit(t, v, 0)
				release("p3", p3)
			}
			requireStored(t, v, tr, false, "P3", "H")
			requireAudit(t, v, 0)
		})
	}
}
