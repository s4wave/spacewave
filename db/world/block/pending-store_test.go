//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// pendingTree builds blocks bottom up for a PendingStore test, each naming
// the children given.
type pendingTree map[string]*block.PutBatchEntry

// add builds the block name over children and returns it.
func (tr pendingTree) add(t *testing.T, name string, children ...string) *block.PutBatchEntry {
	t.Helper()
	data := []byte("pending " + name)
	refs := make([]*block.BlockRef, 0, len(children))
	for _, c := range children {
		refs = append(refs, tr[c].Ref)
		data = append(data, tr[c].Ref.MarshalString()...)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	tr[name] = e
	return e
}

// ref returns the reference of the block name.
func (tr pendingTree) ref(name string) *block.BlockRef {
	return tr[name].Ref
}

// TestPendingStoreRunThrough checks on a Volume that frees with a 1ns grace
// that a PendingStore stores only the trees a root moves onto: a replaced
// tree never reaches the Volume, a root move stores its tree in the move, a
// pin stores the tree it pins, a sweep forgets what no live root reaches, and
// a tree that outgrows the bound is held under its writer's stage until its
// root lands, and a flush holds the tree it stores until the move that names
// it, after which a move stores a tree whose flush ended again. Collection between every step frees no block a root or pin
// reaches.
func TestPendingStoreRunThrough(t *testing.T) {
	f := newVolumeFixture(t)
	ctx := t.Context()
	ps := block.NewPendingStore(f.store, 1<<20)
	tr := pendingTree{}
	put := func(store block.StoreOps, names ...string) {
		t.Helper()
		entries := make([]*block.PutBatchEntry, len(names))
		for i, name := range names {
			entries[i] = tr[name]
		}
		if _, err := store.PutBlockBatch(ctx, entries); err != nil {
			t.Fatal(err)
		}
	}
	stored := func(want bool, names ...string) {
		t.Helper()
		for _, name := range names {
			if got := f.stored(t, tr.ref(name)); got != want {
				t.Fatalf("block %s stored=%v, want %v", name, got, want)
			}
		}
	}

	// Two Worlds that share E stay in memory and readable.
	tr.add(t, "E")
	tr.add(t, "C1", "E")
	tr.add(t, "R1", "C1")
	tr.add(t, "C2", "E")
	tr.add(t, "R2", "C2")
	put(ps, "E", "C1", "R1", "C2", "R2")
	stored(false, "E", "C1", "R1", "C2", "R2")
	if _, found, err := ps.GetBlock(ctx, tr.ref("R1")); err != nil || !found {
		t.Fatalf("a kept block read %v %v", found, err)
	}

	// A root on the second stores its tree in the move; the replaced first
	// never reaches the Volume, and collection keeps the held tree.
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-test", Ref: tr.ref("R2")}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(true, "E", "C2", "R2")
	stored(false, "C1", "R1")

	// A sweep from the live root forgets the replaced tree.
	ps.Sweep([]*block.BlockRef{tr.ref("R2")}, ps.SweepMark())
	if n := ps.KeptBytes(); n != 0 {
		t.Fatalf("sweep kept %d bytes, want 0", n)
	}

	// A pin on a kept World holds its tree in memory with no write: a sweep
	// that no live root of it reaches keeps it while the pin holds, a pin and
	// release leaves a live World kept, and a sweep after the release forgets
	// it.
	tr.add(t, "C3", "E")
	tr.add(t, "R3", "C3")
	put(ps, "C3", "R3")
	release, err := ps.PinRoot(ctx, tr.ref("R3"))
	if err != nil {
		t.Fatal(err)
	}
	ps.Sweep(nil, ps.SweepMark())
	stored(false, "C3", "R3")
	kept := func(want bool, names ...string) {
		t.Helper()
		for _, name := range names {
			if _, found, err := ps.GetBlock(ctx, tr.ref(name)); err != nil || found != want {
				t.Fatalf("block %s kept=%v err=%v, want %v", name, found, err, want)
			}
		}
	}
	kept(true, "C3", "R3")
	release()
	f.collect(t)
	ps.Sweep([]*block.BlockRef{tr.ref("R3")}, ps.SweepMark())
	kept(true, "C3", "R3")
	ps.Sweep(nil, ps.SweepMark())
	kept(false, "C3", "R3")
	stored(true, "E")

	// A writer's pages kept through its stage survive a sweep that took its
	// mark and gathered its live roots before the writer admitted the root
	// naming them and ended its stage.
	tr.add(t, "D")
	tr.add(t, "RD", "D")
	writer, releaseWriter, err := ps.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	put(writer, "D")
	raceMark := ps.SweepMark()
	put(ps, "RD")
	releaseWriter()
	ps.Sweep(nil, raceMark)
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-race", Ref: tr.ref("RD")}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(true, "D", "RD")

	// A root on a new entry naming kept trees, as a block naming several
	// payload roots is, stores the trees with the entry.
	tr.add(t, "P1")
	tr.add(t, "P2", "E")
	held := tr.add(t, "H", "P1", "P2")
	put(ps, "P1", "P2")
	if err := ps.SetRoots(ctx, []*block.PutBatchEntry{held}, []block.NamedRoot{{Name: "pending-held", Ref: held.Ref}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(true, "H", "P1", "P2", "E")
	if n := ps.KeptBytes(); n != 0 {
		t.Fatalf("a root move kept %d bytes, want 0", n)
	}

	// A block kept after a sweep's mark survives it: its root may be one the
	// sweep's caller gathered its live roots before.
	tr.add(t, "M1")
	mark := ps.SweepMark()
	put(ps, "M1")
	ps.Sweep(nil, mark)
	if _, found, err := ps.GetBlock(ctx, tr.ref("M1")); err != nil || !found {
		t.Fatalf("sweep forgot a block kept after its mark: %v %v", found, err)
	}
	ps.Sweep(nil, ps.SweepMark())

	// A tree a writer's stage put survives a sweep that no live root of it
	// reaches until the stage is released.
	tr.add(t, "S1")
	tr.add(t, "S2", "S1")
	building, releaseBuilding, err := ps.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	put(building, "S1", "S2")
	ps.Sweep(nil, ps.SweepMark())
	for _, name := range []string{"S1", "S2"} {
		if _, found, err := ps.GetBlock(ctx, tr.ref(name)); err != nil || !found {
			t.Fatalf("sweep forgot %s while its stage was open: %v %v", name, found, err)
		}
	}
	releaseBuilding()
	ps.Sweep(nil, ps.SweepMark())
	if n := ps.KeptBytes(); n != 0 {
		t.Fatalf("sweep after the stage ended kept %d bytes, want 0", n)
	}

	// A flush stores a kept tree under its stage before the root move, so a
	// collection between the two frees none of it, and the move puts none of
	// it again and drops it from memory.
	tr.add(t, "F1")
	tr.add(t, "RF", "F1")
	put(ps, "F1", "RF")
	endFlush, err := ps.Flush(ctx, []*block.BlockRef{tr.ref("RF")})
	if err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(true, "F1", "RF")
	kept(true, "F1", "RF")
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-flush", Ref: tr.ref("RF")}}); err != nil {
		t.Fatal(err)
	}
	endFlush()
	f.collect(t)
	stored(true, "F1", "RF")
	if n := ps.KeptBytes(); n != 0 {
		t.Fatalf("a flushed root move kept %d bytes, want 0", n)
	}

	// A flush released with no move leaves its tree kept and unheld, so a
	// collection frees the stored copy, and a later move stores it again.
	tr.add(t, "G1")
	tr.add(t, "RG", "G1")
	put(ps, "G1", "RG")
	endFlush, err = ps.Flush(ctx, []*block.BlockRef{tr.ref("RG")})
	if err != nil {
		t.Fatal(err)
	}
	endFlush()
	f.collect(t)
	kept(true, "G1", "RG")
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-reflush", Ref: tr.ref("RG")}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(true, "G1", "RG")

	// A move that leaves out a flushed tree holds the flush's stage until it
	// lands: the flush ending and a collection running after the move left
	// the tree out and before it reaches the Volume free none of it.
	leased, ok := f.store.(leasedStore)
	if !ok {
		t.Fatal("the Volume bucket holds no leased roots")
	}
	hooked := &hookedRoots{leasedStore: leased}
	hps := block.NewPendingStore(hooked, 1<<20)
	tr.add(t, "K1")
	tr.add(t, "RK", "K1")
	put(hps, "K1", "RK")
	endFlush, err = hps.Flush(ctx, []*block.BlockRef{tr.ref("RK")})
	if err != nil {
		t.Fatal(err)
	}
	hooked.before = func() {
		endFlush()
		f.collect(t)
	}
	if err := hps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-skip", Ref: tr.ref("RK")}}); err != nil {
		t.Fatal(err)
	}
	hooked.before = nil
	f.collect(t)
	stored(true, "K1", "RK")

	// A flush that left out another flush's tree keeps that flush's stage
	// until its own release: the first flush's move landing, its release, its
	// root moving off and a collection free none of the tree before the
	// second flush's move names it.
	tr.add(t, "S1")
	tr.add(t, "RS", "S1")
	put(ps, "S1", "RS")
	endFirst, err := ps.Flush(ctx, []*block.BlockRef{tr.ref("RS")})
	if err != nil {
		t.Fatal(err)
	}
	endSecond, err := ps.Flush(ctx, []*block.BlockRef{tr.ref("RS")})
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-first", Ref: tr.ref("RS")}}); err != nil {
		t.Fatal(err)
	}
	endFirst()
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-first", Previous: tr.ref("RS")}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	if err := ps.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-second", Ref: tr.ref("RS")}}); err != nil {
		t.Fatal(err)
	}
	endSecond()
	f.collect(t)
	stored(true, "S1", "RS")

	// A tree past the bound puts the kept blocks under its overflowing
	// parent with it, under the writer's stage, and keeps them in memory for
	// the kept Worlds that name them; the stage holds them across a
	// collection until the root lands, and the root holds them after the
	// stage ends.
	small := block.NewPendingStore(f.store, len(tr.add(t, "X").Data))
	tr.add(t, "C4", "X")
	tr.add(t, "R4", "C4")
	stage, releaseStage, err := small.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	put(stage, "X", "C4", "R4")
	stored(true, "X", "C4", "R4")
	if n := small.KeptBytes(); n != len(tr["X"].Data) {
		t.Fatalf("the overflow kept %d bytes of the tree it stored, want %d", n, len(tr["X"].Data))
	}
	f.collect(t)
	stored(true, "X", "C4", "R4")
	if err := small.SetRoots(ctx, nil, []block.NamedRoot{{Name: "pending-overflow", Ref: tr.ref("R4")}}); err != nil {
		t.Fatal(err)
	}
	releaseStage()
	f.collect(t)
	stored(true, "X", "C4", "R4")

	// A pin taken while a sweep walks keeps its tree: the sweep read the
	// held set before the pin, so it must check the holds again where it
	// drops. A long kept chain the sweep walks from its live root widens the
	// window, and each round pins the target just after a sweep ends, as the
	// next one starts.
	wide := block.NewPendingStore(f.store, 64<<20)
	chain := make([]string, 0, 5000)
	for i := range cap(chain) {
		name := "W" + strconv.Itoa(i)
		if i == 0 {
			tr.add(t, name)
		} else {
			tr.add(t, name, chain[i-1])
		}
		chain = append(chain, name)
	}
	put(wide, chain...)
	tr.add(t, "PT")
	var sweeps atomic.Uint64
	stop := make(chan struct{})
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		live := []*block.BlockRef{tr.ref(chain[len(chain)-1])}
		for {
			select {
			case <-stop:
				return
			default:
			}
			wide.Sweep(live, wide.SweepMark())
			sweeps.Add(1)
		}
	}()
	waitSweeps := func(n uint64) {
		t.Helper()
		target := sweeps.Load() + n
		deadline := time.Now().Add(10 * time.Second)
		for sweeps.Load() < target {
			if time.Now().After(deadline) {
				t.Fatal("sweeps stalled")
			}
			runtime.Gosched()
		}
	}
	var pinned int
	for round := range 50 {
		// Keep the target, and let a sweep take a mark after its put.
		if _, found, err := wide.GetBlock(ctx, tr.ref("PT")); err != nil || !found {
			put(wide, "PT")
		}
		waitSweeps(1)

		// Pin it as the next sweep starts, and check it stays kept until
		// that sweep and the one after end. A sweep that dropped it before
		// the pin leaves nothing to pin, and the round puts it again.
		releasePin, err := wide.PinRoot(ctx, tr.ref("PT"))
		if errors.Is(err, block.ErrNotFound) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		pinned++
		waitSweeps(2)
		if _, found, err := wide.GetBlock(ctx, tr.ref("PT")); err != nil || !found {
			t.Fatalf("round %d: a sweep dropped a pinned block: %v %v", round, found, err)
		}
		releasePin()
	}
	close(stop)
	<-swept
	if pinned == 0 {
		t.Fatal("no round pinned the kept target")
	}

	// Releasing the roots frees both trees, and the audit finds nothing at
	// risk.
	err = f.store.SetRoots(ctx, nil, []block.NamedRoot{
		{Name: "pending-test", Previous: tr.ref("R2")},
		{Name: "pending-overflow", Previous: tr.ref("R4")},
		{Name: "pending-held", Previous: tr.ref("H")},
		{Name: "pending-race", Previous: tr.ref("RD")},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	stored(false, "E", "C2", "R2", "X", "C4", "R4", "H", "P1", "P2", "D", "RD")
	audit, err := f.volume.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if audit.LiveQueued != 0 || audit.Undercount != 0 {
		t.Fatalf("audit found live blocks at risk: queued=%d undercount=%d", audit.LiveQueued, audit.Undercount)
	}
}

// leasedStore is a store that holds leased roots, such as a Volume bucket.
type leasedStore interface {
	block.StoreOps
	block.LeasedRoots
}

// hookedRoots runs before, when set, ahead of each root move it passes on,
// and keeps the store's stages and pins.
type hookedRoots struct {
	leasedStore
	// before runs ahead of the next root moves.
	before func()
}

// SetRoots runs before, then moves the roots.
func (h *hookedRoots) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if h.before != nil {
		h.before()
	}
	return h.leasedStore.SetRoots(ctx, entries, roots)
}
