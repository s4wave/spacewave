package space_world

import (
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
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
