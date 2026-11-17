package space_world

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// TestCopyHoldsWhatItSkips walks the fence and backfill copy through a Volume
// that frees once a record is grace old. Each case names blocks stored with no
// holder, netted to zero and ripe, which the next collection past grace frees
// unless the copy holds them: a cache fill the copy is asked for as a root, a
// proved block a collection runs beside between the copy's check and the Land
// of its parent, and a stored parent with no proof over a proved child. A
// collection right after the copy, and another once the copy's own records
// aged, keep every block the copy holds. No run-through reaches these: they
// need a grace that separates netting from the free, and blocks nothing holds
// under a copied root.
func TestCopyHoldsWhatItSkips(t *testing.T) {
	// Build an adapter over a Volume with a grace long enough to step through.
	ctx := t.Context()
	const grace = 300 * time.Millisecond
	a, collect := newVolumeAdapter(t, 1<<20, grace)
	store := a.pages.StoreOps

	// build returns the entry of a block naming children.
	build := func(name string, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}

	// put stores a block that nothing holds, as a cache fill is stored.
	put := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		e := build(name, children...)
		if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{e}); err != nil {
			t.Fatal(err)
		}
		return e.Ref
	}

	// keep keeps a page in a's pages, as a writer keeps its pages.
	keep := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		e := build(name, children...)
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{e}); err != nil {
			t.Fatal(err)
		}
		return e.Ref
	}

	// stored reports whether the Volume stores ref.
	stored := func(ref *block.BlockRef) bool {
		t.Helper()
		_, found, err := store.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	// copyCase stores a case's blocks with no holder, nets them to zero and
	// ripe, and copies its root under its own fence root while a collection
	// past grace may free them. A collection right after the copy keeps every
	// block the copy holds, since the hold mark the copy set stops every free;
	// one after the copy's records aged frees a loose block nothing names and
	// keeps them, since the re-hold gave each its holder back.
	copyCase := func(name string, build func() (root *block.BlockRef, held []*block.BlockRef, visited func(*block.BlockRef, []byte))) {
		t.Helper()
		// Store the blocks, and a loose block nothing names.
		root, held, visited := build()
		loose := put(name + " loose")
		dead := slices.DeleteFunc(append(slices.Clone(held), loose), func(ref *block.BlockRef) bool {
			return !stored(ref)
		})

		// Net them to zero once their records are grace old. They ripen at
		// the journal's end past the record landed after them, which a
		// collection passes once the record landed after it ages too.
		time.Sleep(grace)
		put(name + " tick")
		collect()
		put(name + " tock")
		for _, ref := range dead {
			if !stored(ref) {
				t.Fatalf("%s: the first collection freed %s before it ripened", name, ref.MarshalString())
			}
		}
		time.Sleep(grace)

		// Copy the root and hold it.
		opts := block.GraphCopyOptions{Forget: true, Visited: visited}
		if err := a.copyAndHold(ctx, fenceRootPrefix+name, []*block.BlockRef{root}, opts, false); err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}

		// Collect right after the copy and once it aged.
		for i, step := range []string{"right after the copy", "once the copy aged"} {
			if i != 0 {
				time.Sleep(grace)
			}
			collect()
			for _, ref := range held {
				if !stored(ref) {
					t.Errorf("%s: %s: the collection freed held block %s", name, step, ref.MarshalString())
				}
			}
		}
		if stored(loose) {
			t.Errorf("%s: the collection kept the loose block, so it checked no free", name)
		}
	}

	// A cache fill the copy is asked for as a root.
	copyCase("fill", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		fill := put("fill")
		return fill, []*block.BlockRef{fill}, nil
	})

	// A proved block a kept page names beside a kept leaf, with a collection
	// between the copy's check of the page's children and the page's Land.
	copyCase("page", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		proved := put("proved")
		if err := block.MarkRootsComplete(ctx, store, []*block.BlockRef{proved}); err != nil {
			t.Fatal(err)
		}
		leaf := keep("leaf")
		page := keep("page", proved, leaf)
		return page, []*block.BlockRef{page, proved, leaf}, func(ref *block.BlockRef, _ []byte) {
			if ref.EqualsRef(leaf) {
				collect()
			}
		}
	})

	// A stored parent with no proof over a proved child.
	copyCase("parent", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		child := put("child")
		parent := put("parent", child)
		if err := block.MarkRootsComplete(ctx, store, []*block.BlockRef{child}); err != nil {
			t.Fatal(err)
		}
		return parent, []*block.BlockRef{parent, child}, nil
	})
}

// TestAdmitRepeatRewritesTree holds an admitted payload P -> L whose blocks
// are stored and netted to zero, as after the stage that stored them was
// released, through the adapter's payload hold with frees on. The hold is
// refused, and a repeat that keeps the tree in the pages again, as a
// transaction written again does, lands it in the hold's own batch and
// commits; collections past grace keep every block.
func TestAdmitRepeatRewritesTree(t *testing.T) {
	ctx := t.Context()
	const grace = 300 * time.Millisecond
	a, collect := newVolumeAdapter(t, 1<<20, grace)
	store := a.pages.StoreOps
	entry := func(name string, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}
	l := entry("leaf")
	p := entry("payload", l.Ref)
	stored := func() bool {
		t.Helper()
		for _, e := range []*block.PutBatchEntry{p, l} {
			if _, found, err := store.GetBlock(ctx, e.Ref); err != nil || !found {
				return false
			}
		}
		return true
	}

	// Store the tree with no holder and net it to zero, not ripe.
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{l, p}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(grace)
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{entry("tick")}); err != nil {
		t.Fatal(err)
	}
	collect()
	if !stored() {
		t.Fatal("the tree was freed before it ripened")
	}

	// The hold is refused, then a repeat that keeps the tree commits.
	adm := func() *spaceAdmission {
		return &spaceAdmission{payloads: []*block.BlockRef{p.Ref}, done: make(chan struct{})}
	}
	if err := a.holdAdmitted(ctx, adm(), false); !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("hold over a tree at zero = %v, want ErrRootNotHeld", err)
	}
	if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{l, p}); err != nil {
		t.Fatal(err)
	}
	if err := a.holdAdmitted(ctx, adm(), false); err != nil {
		t.Fatalf("repeat: %v", err)
	}

	// Collections past grace keep the tree.
	for range 2 {
		time.Sleep(grace)
		collect()
	}
	if !stored() {
		t.Fatal("a collection freed the admitted tree")
	}
}

// TestAdmitHoldsOverflow admits candidates whose put stores a block past the
// PendingStore's bound on a Volume that frees once a record is grace old, and
// checks that collections past grace leave every block under the candidate's
// root readable from the Volume: one admission whose overflow leaves the kept
// pages under half the bound, and one whose hold waits on the root lock while
// the collections run.
func TestAdmitHoldsOverflow(t *testing.T) {
	const (
		bound = 64 << 10
		grace = 300 * time.Millisecond
	)

	// page returns the entry of a block of at least size bytes naming children.
	page := func(t *testing.T, name string, size int, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		if len(data) < size {
			data = append(data, bytes.Repeat([]byte{'.'}, size-len(data))...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}

	// admission returns an admission of a candidate at the last entry.
	admission := func(entries ...*block.PutBatchEntry) *spaceAdmission {
		root := entries[len(entries)-1].Ref
		return &spaceAdmission{candidate: world.Candidate{Root: root, Entries: entries}, done: make(chan struct{})}
	}

	// age collects past grace often enough to net and free an unheld block.
	age := func(collect func()) {
		for range 3 {
			time.Sleep(grace)
			collect()
		}
	}

	// requireStored fails unless the Volume serves every entry.
	requireStored := func(t *testing.T, a *Adapter, entries ...*block.PutBatchEntry) {
		t.Helper()
		for _, e := range entries {
			_, ok, err := a.pages.StoreOps.GetBlock(t.Context(), e.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("a collection freed block %q of the admitted World", e.Data[:4])
			}
		}
	}

	t.Run("overflow with Full false", func(t *testing.T) {
		a, collect := newVolumeAdapter(t, bound, grace)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, leaf.Ref)
		if err := a.keepAdmitted(t.Context(), admission(leaf, root)); err != nil {
			t.Fatal(err)
		}
		if a.pages.Full() {
			t.Fatal("the kept pages reached half the bound")
		}
		age(collect)
		requireStored(t, a, leaf, root)
	})

	t.Run("hold delayed past grace", func(t *testing.T) {
		a, collect := newVolumeAdapter(t, bound, grace)
		mid := page(t, "mid ", bound/2+1)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, mid.Ref, leaf.Ref)

		// Admit while the test holds the root lock, so the hold waits.
		unlock, err := a.rootMtx.Lock(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() { errCh <- a.keepAdmitted(t.Context(), admission(mid, leaf, root)) }()
		for {
			ok, err := a.pages.StoreOps.GetBlockExists(t.Context(), leaf.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if !a.pages.Full() {
			t.Fatal("the kept pages stayed under half the bound")
		}

		// Collect past grace while the hold waits, then let it proceed.
		age(collect)
		unlock()
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
		age(collect)
		requireStored(t, a, mid, leaf, root)
	})

	t.Run("overflow stage opened under a cancelled context", func(t *testing.T) {
		// The put past the bound opens the lazy stage under a context
		// already cancelled, and the store's open honours it, so the open
		// fails. The admission fails with it
		// and stores nothing the stage would have held: a block the Volume
		// stored unheld would free before any root named it. A repeat under
		// a live context admits the World, and collections keep it.
		a, collect := newVolumeAdapter(t, bound, grace)
		inner := a.pages.StoreOps
		a.pages = block.NewPendingStore(&cancelStageStore{
			StoreOps:    inner,
			LeasedRoots: inner.(block.LeasedRoots),
			RootProofs:  inner.(block.RootProofs),
		}, bound)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, leaf.Ref)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := a.keepAdmitted(ctx, admission(leaf, root)); !errors.Is(err, context.Canceled) {
			t.Fatalf("admission under a cancelled context = %v, want it cancelled", err)
		}
		for _, e := range []*block.PutBatchEntry{leaf, root} {
			if ok, err := a.pages.StoreOps.GetBlockExists(t.Context(), e.Ref); err != nil || ok {
				t.Fatalf("block %q stored=%v err %v after a cancelled admission, want absent", e.Data[:4], ok, err)
			}
		}
		if err := a.keepAdmitted(t.Context(), admission(leaf, root)); err != nil {
			t.Fatal(err)
		}
		age(collect)
		requireStored(t, a, leaf, root)
	})
}

// cancelStageStore is a bucket whose OpenStage fails under a cancelled
// context, as a stage open that waits on a lock does.
type cancelStageStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
}

// OpenStage fails under a cancelled context, and otherwise opens the stage.
func (s *cancelStageStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return s.LeasedRoots.OpenStage(ctx)
}
