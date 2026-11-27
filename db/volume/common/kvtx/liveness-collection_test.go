package kvtx

import (
	"errors"
	"hash/fnv"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// testLivenessCollection checks bounded collection, crash recovery, and frees switches.
func testLivenessCollection(t *testing.T) {
	t.Helper()

	t.Run("record larger than a slice", func(t *testing.T) {
		// A under root a references 5000 leaves. B, born under root b,
		// references the same stored leaves, so its one record carries
		// more up pairs than a net slice applies and splits across
		// slices. Releasing a frees only A. Releasing b frees B and
		// every leaf.
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

	t.Run("record larger than a slice", func(t *testing.T) {
		// 20,000 leaves stay held under mids of 1000, each under its
		// root. One commit stores a parent referencing each mid's
		// leaves, so its record carries an up pair for every leaf, more
		// than a net slice holds. The commit writes it in pages and the
		// collector nets it across slices once it ages, so no collector
		// transaction does as much work as the record has pairs or
		// reads half of its bytes.
		ctx := t.Context()
		cs := kvtx_kvtest.NewCountingStore(newGuardedStore(t))
		v, err := NewVolume(ctx, "test/wide", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		clk := &testClock{now: time.Unix(1_800_000_000, 0)}
		v.clock = clk.Now
		t.Cleanup(func() { _ = v.Close() })
		tr := testTree{}
		const pairs = 20_000
		var parents []string
		for m := 0; m*1000 < pairs; m++ {
			names := make([]string, 0, 1000)
			for i := m * 1000; i < (m+1)*1000; i++ {
				names = append(names, "leaf "+strconv.Itoa(i))
				tr.add(t, names[len(names)-1])
			}
			mid := "mid " + strconv.Itoa(m)
			ref := tr.add(t, mid, names...)
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(names, mid)...), []block.NamedRoot{{Name: mid, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			parents = append(parents, "parent "+strconv.Itoa(m))
			tr.add(t, parents[m], names...)
		}
		top := tr.add(t, "top", parents...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(parents), "top")...), []block.NamedRoot{{Name: "top", Ref: top}}); err != nil {
			t.Fatal(err)
		}
		collect := func() {
			t.Helper()
			cs.ResetMax()
			collectAll(t, v)
			if m := cs.Counts().MaxWriteOps; m >= pairs {
				t.Fatalf("a collector transaction made %d operations over a record of %d pairs", m, pairs)
			}
			if b := cs.Counts().MaxWriteReadBytes; b >= pairs*33/2 {
				t.Fatalf("a collector transaction read %d bytes of a record of %d pairs", b, pairs)
			}
		}

		// Leave the young record, then net it once it ages.
		collect()
		clk.Add(block_gc.DefaultGrace + time.Second)
		collect()
		requireStored(t, v, tr, true, append(slices.Clone(parents), "top", "leaf 0", "leaf 19999")...)
		requireAudit(t, v, 0)
	})

	t.Run("commit slower than a slice", func(t *testing.T) {
		// W references 2,000 leaves under root w, released past the
		// grace period. Each transaction's commit costs 3 ms the
		// collector's own work does not, on the wall clock and on the
		// Volume's, more than a slice aims to hold the writer. The pace
		// counts only the collector's work, so the collection frees W
		// and its leaves in a bounded number of transactions rather
		// than shrinking every slice to its smallest.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 2000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		w := tr.add(t, "W", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "W")...), []block.NamedRoot{{Name: "w", Ref: w}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "w", Previous: w}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 100, after: func() {
			clk.Add(3 * time.Millisecond)
			time.Sleep(3 * time.Millisecond)
		}}
		if freed := collectThrough(t, probe, block_gc.CollectorOptions{Frees: true, Now: v.now}); freed != 2001 {
			t.Fatalf("freed %d of W and its 2000 leaves", freed)
		}
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "W")...)
	})

	t.Run("writer always waiting", func(t *testing.T) {
		// G references 100 leaves under root g, released past the grace
		// period, and H references 64 others under root h. A writer
		// waits on every collector transaction and after each commits a
		// new root on a parent of H's leaves, an up for each. A waiting
		// writer does not shrink the netting slices, so they reach the
		// up log's end, and the writer's ups delay no free: G and its
		// leaves are freed in a bounded number of transactions while
		// the writer's blocks stay.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 100)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		g := tr.add(t, "G", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "G")...), []block.NamedRoot{{Name: "g", Ref: g}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
			t.Fatal(err)
		}
		shared := make([]string, 64)
		for i := range shared {
			shared[i] = "shared " + strconv.Itoa(i)
			tr.add(t, shared[i])
		}
		h := tr.add(t, "H", shared...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(shared), "H")...), []block.NamedRoot{{Name: "h", Ref: h}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		held := append(slices.Clone(shared), "H")
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 1000, after: func() {
			root := "round " + strconv.Itoa(len(held))
			ref := tr.add(t, root, shared...)
			if err := v.SetBucketRoots(ctx, "b", tr.entries(root), []block.NamedRoot{{Name: root, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			held = append(held, root)
		}}
		opts := block_gc.CollectorOptions{
			Frees:          true,
			Now:            v.now,
			WritersWaiting: func() bool { return true },
		}
		if freed := collectThrough(t, probe, opts); freed != 101 {
			t.Fatalf("freed %d of G and its 100 leaves", freed)
		}
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "G")...)
		requireStored(t, v, tr, true, held...)
		requireAudit(t, v, 0)
	})

	t.Run("revive over a freed child", func(t *testing.T) {
		// main holds R2 -> C2 -> L and moves to R1, and a pass with frees
		// off nets the move, so R2 is at zero on the free queue when main
		// moves back onto R2 with no put. No transaction frees a child while
		// a stored parent lists it, so the revived tree reads.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "L")
		tr.add(t, "C2", "L")
		r2 := tr.add(t, "R2", "C2")
		r1 := tr.add(t, "R1")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("L", "C2", "R2", "Z"), []block.NamedRoot{{Name: "main", Ref: r2}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1, Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)

		// Revive R2 with no put. R1 goes, and R2's tree reads.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		var freed uint64
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 1 {
			t.Fatalf("freed %d after the revive, want R1 alone", freed)
		}
		requireReads(t, v, tr, "R2", "C2", "L", "Z")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)

		// Once main leaves R2, its whole tree goes.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		freed = 0
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 3 {
			t.Fatalf("freed %d of R2's tree, want 3", freed)
		}
		requireStored(t, v, tr, false, "R2", "C2", "L")
		requireAudit(t, v, 0)
	})

	t.Run("shared child at its other parent's free", func(t *testing.T) {
		// C is under R1, which main holds, and under R3, whose root x is
		// released and netted to zero but not freed. main then moves from R1
		// onto R3 in one Land, and the next pass frees R1. C keeps R3's
		// holder through R1's free.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		r1 := tr.add(t, "R1", "C")
		r3 := tr.add(t, "R3", "C")
		roots := []block.NamedRoot{{Name: "main", Ref: r1}, {Name: "x", Ref: r3}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "R1", "R3"), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x", Previous: r3}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		nets := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
		if res, err := nets.Collect(ctx); err != nil || res.Netted == 0 || res.Freed != 0 {
			t.Fatalf("netting x's release = %+v err %v, want it netted and nothing freed", res, err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r3, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		var freed uint64
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 1 {
			t.Fatalf("freed %d, want R1 alone", freed)
		}
		requireReads(t, v, tr, "R3", "C")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)
	})

	t.Run("deep tree under steady writes", func(t *testing.T) {
		// A tree five levels deep, each block referencing two children,
		// loses its root, and a writer commits a new root referencing
		// A, which root a holds, after every collector transaction, an
		// up pair for each. The writer's ups net with the tree's
		// release, and each free's child downs queue at the up log's
		// end, so the whole tree of 31 blocks goes in the first pass
		// past the grace period, with exact counts, and the writer's
		// blocks stay.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		names := tr.addBinary(t, 5)
		root := tr[names[len(names)-1]].Ref
		a := tr.add(t, "A")
		roots := []block.NamedRoot{{Name: "tree", Ref: root}, {Name: "a", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(names), "A")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "tree", Previous: root}}); err != nil {
			t.Fatal(err)
		}
		held := []string{"A"}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
			name := "write " + strconv.Itoa(len(held))
			ref := tr.add(t, name, "A")
			if err := v.SetBucketRoots(ctx, "b", tr.entries(name), []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			held = append(held, name)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		for i, n := range []uint64{31} {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if freed := collectThrough(t, probe, opts); freed != n {
				t.Fatalf("round %d freed %d, want %d", i, freed, n)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if freed := collectThrough(t, probe, opts); freed != 0 {
			t.Fatalf("freed %d past the tree", freed)
		}
		requireStored(t, v, tr, false, names...)
		requireStored(t, v, tr, true, held...)
		requireAudit(t, v, 0)
	})

	for _, tc := range []struct {
		name   string
		raised func(tr testTree) *block.BlockRef
	}{
		{"queued block", func(tr testTree) *block.BlockRef { return tr["C"].Ref }},
		{"child of a queued block", func(tr testTree) *block.BlockRef { return tr["E"].Ref }},
	} {
		t.Run(tc.name+" raised between the free's read and its write", func(t *testing.T) {
			// R -> E loses main, and C loses aux, past the grace period.
			// The free reads its candidates in a snapshot, then a commit
			// moves root k onto one of them, or onto E, R's child, before
			// the free's write slice. The write takes each candidate only
			// while its queue entry stays, and R's free moves E down only
			// from the count that holds the commit's up, so the block the
			// root names and its children stay.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "E")
			r := tr.add(t, "R", "E")
			c := tr.add(t, "C")
			z := tr.add(t, "Z")
			roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "aux", Ref: c}, {Name: "z", Ref: z}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R", "C", "Z"), roots); err != nil {
				t.Fatal(err)
			}
			gone := []block.NamedRoot{{Name: "main", Previous: r}, {Name: "aux", Previous: c}}
			if err := v.SetBucketRoots(ctx, "b", nil, gone); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)

			// Move k onto the block after the free's first snapshot.
			raised := tc.raised(tr)
			hook := &viewHook{Store: collectorStore{v: v}, after: func() {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Ref: raised}}); err != nil {
					t.Error(err)
				}
			}}
			opts := block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: hook.arm}
			collectThrough(t, &paceProbe{Store: hook, limit: 1000, after: func() {}}, opts)
			if !hook.fired {
				t.Fatal("the free read no snapshot")
			}

			// The raised block and its children stay through every pass.
			kept := []string{"Z", "C"}
			if raised != c {
				kept = []string{"Z", "E"}
			}
			for range 3 {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				requireReads(t, v, tr, kept...)
				requireListsStored(t, v, tr)
			}
			requireAudit(t, v, 0)
		})
	}

	t.Run("free transactions run twice", func(t *testing.T) {
		// A tree five levels deep, each block naming two, loses its root,
		// and a store runs each collector transaction once and discards
		// it, then runs it again and commits, as a store that retries a
		// conflicted transaction does. The collector keeps its place on
		// the free queue from the run that committed, so the whole tree
		// goes and A, which root a holds, stays.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		names := tr.addBinary(t, 5)
		root := tr[names[len(names)-1]].Ref
		a := tr.add(t, "A")
		roots := []block.NamedRoot{{Name: "tree", Ref: root}, {Name: "a", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(names), "A")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "tree", Previous: root}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &paceProbe{Store: &rerunStore{Store: collectorStore{v: v}}, limit: 1000, after: func() {
			requireListsStored(t, v, tr)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now, WritersWaiting: func() bool { return true }}
		collectThrough(t, probe, opts)
		requireStored(t, v, tr, false, names...)
		requireReads(t, v, tr, "A")
		requireAudit(t, v, 0)
	})

	for _, cut := range []int{2, 5, 11, 23} {
		t.Run("restart mid-cascade after "+strconv.Itoa(cut)+" transactions", func(t *testing.T) {
			// A tree five levels deep, each block naming two, loses its
			// root, and a collector freeing one block a slice stops after
			// cut transactions, losing its place on the free queue and the
			// children it kept behind it. A new collector finds every
			// remaining block on the queue, so the whole tree goes and A
			// stays.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			names := tr.addBinary(t, 5)
			root := tr[names[len(names)-1]].Ref
			a := tr.add(t, "A")
			roots := []block.NamedRoot{{Name: "tree", Ref: root}, {Name: "a", Ref: a}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(names), "A")...), roots); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "tree", Previous: root}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			opts := block_gc.CollectorOptions{Frees: true, Now: v.now, WritersWaiting: func() bool { return true }}
			first := block_gc.NewCollector(&paceProbe{Store: collectorStore{v: v}, limit: cut, after: func() {}}, opts)
			for {
				res, err := first.Collect(ctx)
				if errors.Is(err, errPaceLimit) {
					break
				}
				if err != nil || !res.More {
					t.Fatalf("the first collector ended before its cut: err %v", err)
				}
			}
			requireListsStored(t, v, tr)
			collectThrough(t, &paceProbe{Store: collectorStore{v: v}, limit: 1000, after: func() {
				requireListsStored(t, v, tr)
			}}, opts)
			requireStored(t, v, tr, false, names...)
			requireReads(t, v, tr, "A")
			requireAudit(t, v, 0)
		})
	}

	t.Run("restart after a second process unadopts", func(t *testing.T) {
		// A, with frees on, adopts a Volume holding main -> R -> X and an
		// unheld G. B opens the same store writable with frees off, returns
		// it to unadopted, as its reset of an adoption in progress or a list
		// rebuild does, and closes. A restarts with frees on: its open
		// counts the Volume again, and G goes while R and X stay.
		store := newGuardedStore(t)
		a, _ := openLivenessVolume(t, store, true)
		observePuts(a)
		waitAdopted(t, a)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := a.SetBucketRoots(ctx, "b", tr.entries("X", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		b, _ := openLivenessVolume(t, store, false)
		markUnadopted(t, b)
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}

		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireReads(t, v, tr, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("restart between turning frees on and the recount", func(t *testing.T) {
		// A Volume holding main -> R -> X and an unheld G turns frees on,
		// which returns it to unadopted, and closes before its recount
		// runs. Reopened with frees on, which writes nothing since the
		// switch is on already, it counts again before any free, and G
		// goes while R and X stay.
		store := newGuardedStore(t)
		v1, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		state, err := block_gc.SetFrees(ctx, collectorStore{v: v1}, true)
		if err != nil {
			t.Fatal(err)
		}
		if !state.GetFrees() || state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTED {
			t.Fatalf("turning frees on left the state %v, want frees on and a recount due", state)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireReads(t, v, tr, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("frees switched off and on", func(t *testing.T) {
		// A Volume born with frees on holds main -> R -> X and d -> D -> E,
		// and d lets D go. Restarted with frees off, it nets the release
		// and frees nothing, and records frees off. Restarted with them on
		// again, it returns to unadopted and counts every holder again
		// before its first free, then frees D and E and keeps R and X.
		store := newGuardedStore(t)
		v1, clk1 := openLivenessVolume(t, store, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "E")
		d := tr.add(t, "D", "E")
		roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "d", Ref: d}}
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R", "E", "D"), roots); err != nil {
			t.Fatal(err)
		}
		// Net a prefix of the down log, leaving its deleted pages in the store
		// guard. Reopening with the old state encoding must skip that prefix.
		for range maxWriteTombstones + 1 {
			temporary := []block.NamedRoot{{Name: "temporary", Ref: r}}
			if err := v1.SetBucketRoots(ctx, "b", nil, temporary); err != nil {
				t.Fatal(err)
			}
			temporary = []block.NamedRoot{{Name: "temporary", Previous: r}}
			if err := v1.SetBucketRoots(ctx, "b", nil, temporary); err != nil {
				t.Fatal(err)
			}
		}
		clk1.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v1)

		// Leave a live down at the end of the old, deleted prefix.
		if err := v1.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "d", Previous: d}}); err != nil {
			t.Fatal(err)
		}
		// Reproduce a Volume written before it kept the down cursor and the
		// holes-clear mark, retaining its live down and all earlier deletes.
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		clear := slices.Concat(livenessPrefix, []byte{'z'})
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		rec := block_gc.NewRecords(tx, livenessPrefix)
		state, err := rec.GetState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.GetDownCursor() <= maxWriteTombstones || state.GetDownCursor() >= state.GetDownSeq() {
			t.Fatalf("want a deleted down prefix and a live page, got cursor %d and end %d", state.GetDownCursor(), state.GetDownSeq())
		}
		state.DownCursor = 0
		if err := rec.SetState(ctx, state); err != nil {
			t.Fatal(err)
		}
		if err := tx.Delete(ctx, clear); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// Restart with frees off: the release nets and nothing frees.
		v2, clk2 := openLivenessVolume(t, store, false)
		var freed uint64
		for range 3 {
			clk2.Add(3 * (block_gc.DefaultGrace + time.Second))
			freed += collectAll(t, v2).Freed
		}
		if freed != 0 {
			t.Fatalf("freed %d with frees off, want 0", freed)
		}
		if state := livenessState(t, v2); state.GetFrees() {
			t.Fatal("an open with frees off left them recorded on")
		}
		// The frees-off pass must establish the hole proof and net the live
		// page beyond the deleted prefix.
		if n := countPrefix(t, store, clear); n != 1 {
			t.Fatalf("holes-clear records after collection with frees off = %d, want 1", n)
		}
		state = livenessState(t, v2)
		if state.GetDownCursor() != state.GetDownSeq() {
			t.Fatalf("down cursor after netting = %d, want log end %d", state.GetDownCursor(), state.GetDownSeq())
		}

		requireStored(t, v2, tr, true, "X", "R", "E", "D")
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Restart with frees on: the switch recounts, then D and E go.
		v, clk := openLivenessVolume(t, store, true)
		if state := waitAdopted(t, v); !state.GetFrees() || state.GetAdoptionRun() == "" {
			t.Fatalf("frees turned on left the state %v, want them on and a recount's run", state)
		}
		freed = 0
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			freed += collectAll(t, v).Freed
		}
		if freed != 2 {
			t.Fatalf("freed %d after frees turned on again, want D and E", freed)
		}
		requireReads(t, v, tr, "X", "R")
		requireStored(t, v, tr, false, "E", "D")
		requireAudit(t, v, 0)
	})

	t.Run("turning frees on recounts an adopted Volume", func(t *testing.T) {
		// An adopted Volume with frees off holds main -> R -> P -> C, and
		// the store loses C with its list but keeps its count, so no audit
		// records the hole. Turning frees on recounts, which counts C one
		// holder more than P, its namer, so a put of C alone leaves it held
		// by P and it outlives the grace.
		store := newGuardedStore(t)
		v1, _ := openLivenessVolume(t, store, true)
		waitAdopted(t, v1)
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		v2, _ := openLivenessVolume(t, store, false)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		if err := v2.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, v2, c)
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Turn frees on, put C alone, and collect past the grace.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			collectAll(t, v)
		}
		requireReads(t, v, tr, "R", "P", "C")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C counts %d holders after its put, want P's 1", holders)
		}
		requireAudit(t, v, 0)
	})

	t.Run("the audit sees the residual", func(t *testing.T) {
		// After the recount, the store loses C with its list, and a put of
		// C alone comes before any audit records the hole. That put leaves
		// C one holder short of P, its namer, the residual window, and an
		// audit run before the grace reports it. Nothing the Volume records
		// holds C, so the put stores it unheld.
		v, _ := newLivenessVolume(t, true)
		observePuts(v)
		waitAdopted(t, v)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, v, c)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
			t.Fatal(err)
		}
		res, err := block_gc.Audit(ctx, collectorStore{v: v}, v.now(), false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Undercount != 1 || !slices.ContainsFunc(res.UndercountSample, c.EqualsRef) {
			t.Fatalf("audit found %d undercounts in %v, want C alone", res.Undercount, res.UndercountSample)
		}
	})

	t.Run("hole audited after the recount that missed it", func(t *testing.T) {
		// With frees off, main -> P -> X -> C and aux -> C, and the store
		// loses X with its list. Turning frees on recounts, which reads no
		// list for X, so C counts aux alone. An audit then records X's hole,
		// and a put refills it: the refill gives C back X's holder, so C
		// matches a clean recount and outlives aux.
		store := newGuardedStore(t)
		v1, _ := openLivenessVolume(t, store, true)
		waitAdopted(t, v1)
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		v2, _ := openLivenessVolume(t, store, false)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		x := tr.add(t, "X", "C")
		p := tr.add(t, "P", "X")
		roots := []block.NamedRoot{{Name: "main", Ref: p}, {Name: "aux", Ref: c}}
		if err := v2.SetBucketRoots(ctx, "b", tr.entries("C", "X", "P"), roots); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, v2, x)
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Turn frees on, audit, and refill X.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want X's", res.Holes, err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)

		// The counts match a clean recount of a copy, but for X's own: the
		// recount that missed X left its count a raw tally, one high.
		cp := copyAll(t, store)
		vc, _ := openLivenessVolume(t, cp, true)
		markUnadopted(t, vc)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		got, want := adoptedRecords(t, store), adoptedRecords(t, cp)
		xKey := string(livenessPrefix) + "H" + string(block_gc.BlockKey(x))
		delete(got, xKey)
		delete(want, xKey)
		if !maps.Equal(got, want) {
			t.Fatalf("refill of an audited hole differs from a clean recount: %q", recordsDiff(got, want))
		}

		// Releasing aux leaves C held by X.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "aux", Previous: c}}); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			collectAll(t, v)
		}
		requireReads(t, v, tr, "P", "X", "C")
		requireAudit(t, v, 0)
	})

	t.Run("refill names a child the free step is about to take", func(t *testing.T) {
		// The same loss and audit, then aux releases C and its down
		// nets, so C waits ripe on the queue at zero. A put refills X
		// between that net and the free step: the refill references C,
		// so it takes C off the queue with the up it logs, and the free
		// leaves C to X.
		store := newGuardedStore(t)
		off, _ := openLivenessVolume(t, store, false)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		x := tr.add(t, "X", "C")
		p := tr.add(t, "P", "X")
		roots := []block.NamedRoot{{Name: "main", Ref: p}, {Name: "aux", Ref: c}}
		if err := off.SetBucketRoots(ctx, "b", tr.entries("C", "X", "P"), roots); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, off, x)
		if err := off.Close(); err != nil {
			t.Fatal(err)
		}
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want X's", res.Holes, err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "aux", Previous: c}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)

		// Refill X as the pass turns from netting to freeing.
		refilled := false
		phase := func(ph block_gc.Phase) func() {
			if ph == block_gc.PhaseSweep && !refilled {
				refilled = true
				if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), nil); err != nil {
					t.Error(err)
				}
			}
			return func() {}
		}
		collector := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: phase})
		if _, err := collector.Collect(ctx); err != nil || !refilled {
			t.Fatalf("collect err %v, refilled %v", err, refilled)
		}
		requireReads(t, v, tr, "P", "X", "C")

		// C stays once the refill's up nets and every grace period passes.
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			collectAll(t, v)
		}
		requireReads(t, v, tr, "P", "X", "C")
		requireAudit(t, v, 0)
	})

	t.Run("turning frees on recounts after an older build writes", func(t *testing.T) {
		// A Volume adopted with frees on is written by a build that knows
		// neither the switch nor its mark, which keeps both as unknown fields
		// and moves next_seq. Opened again with frees on, the stale mark
		// reads as frees off, so the open returns the Volume to unadopted
		// and counts again before any free.
		store := newGuardedStore(t)
		v1, _ := openLivenessVolume(t, store, true)
		first := waitAdopted(t, v1)
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		writeOlderState(t, store)
		v, _ := openLivenessVolume(t, store, true)
		state := waitAdopted(t, v)
		if !state.GetFrees() || state.GetAdoptionRun() == "" || state.GetAdoptionRun() == first.GetAdoptionRun() {
			t.Fatalf("frees on after an older build's write left the state %v, want them on and a new recount's run", state)
		}

		// A mark from before the frees epoch, the unsalted hash of the
		// other fields, also reads as off and counts again.
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
		known, unknown := splitStateFields(t, store)
		pre := &block_gc.LivenessState{}
		if err := pre.UnmarshalVT(append(known, unknown...)); err != nil {
			t.Fatal(err)
		}
		pre.FreesMark = 0
		value, err := pre.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		h := fnv.New64a()
		_, _ = h.Write(value)
		pre.FreesMark = h.Sum64()
		if value, err = pre.MarshalVT(); err != nil {
			t.Fatal(err)
		}
		setStateValue(t, store, value)
		v3, _ := openLivenessVolume(t, store, true)
		if rerun := waitAdopted(t, v3); rerun.GetAdoptionRun() == state.GetAdoptionRun() {
			t.Fatalf("frees on over a pre-epoch mark kept run %q, want a new recount", rerun.GetAdoptionRun())
		}
	})

	t.Run("turning frees on recounts what an older build wrote", func(t *testing.T) {
		// A Volume adopted with frees on references main -> R -> P -> C. A
		// build that predates the switch then lands R2 -> P, releases
		// main and leaves P one holder short, a count only a recount
		// corrects. It keeps the switch and its mark as unknown fields,
		// so the mark goes stale. Opened again with frees on, the
		// Volume recounts before any free: R2, P and C read after
		// collections past the grace, and R, which nothing references,
		// frees.
		store := newGuardedStore(t)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		r2 := tr.add(t, "R2", "P")
		v1, _ := openLivenessVolume(t, store, true)
		waitAdopted(t, v1)
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		_, switched := splitStateFields(t, store)

		// The older build's writes, made here by a frees-off open, with the
		// switch and its mark put back as that build keeps them.
		v2, clk2 := openLivenessVolume(t, store, false)
		if err := v2.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main2", Ref: r2}}); err != nil {
			t.Fatal(err)
		}
		if err := v2.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk2.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v2)
		err := v2.withDirectAtomic(ctx, func(tx *physicalTx) error {
			holders, ripe, err := tx.rec.GetHolders(ctx, p)
			if err != nil {
				return err
			}
			if holders == 0 {
				return errors.New("P counts no holder")
			}
			return tx.rec.SetHolders(ctx, p, holders-1, ripe)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}
		known, _ := splitStateFields(t, store)
		setStateValue(t, store, append(known, switched...))

		// Frees on again recounts, then collects.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireReads(t, v, tr, "R2", "P", "C")
		requireStored(t, v, tr, false, "R")
		requireAudit(t, v, 0)
	})
}
