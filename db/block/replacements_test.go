package block_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// recording is a transaction recording into its own log, with the root and
// middle pages of a chain loaded, and the store's put count when it opened.
type recording struct {
	tx        *block.Transaction
	log       *block.ReplacementLog
	root, mid *block.Cursor
	puts      int
}

// TestReplacementsMatchDiff walks a chain root -> mid -> leaf through edits,
// moves, removals and root changes, and checks each interval's record against
// the diff of the old and new trees, the pages it holds against the stored
// pages each scenario gives a second parent, and the pages it lists as existed
// against the pages the interval put.
func TestReplacementsMatchDiff(t *testing.T) {
	// Every scenario writes to one in-memory store.
	ctx := t.Context()
	store := newGraphTestStore()

	// chain stores root -> mid -> leaf and returns the root.
	chain := func(msg string) *block.BlockRef {
		// Build the three pages under one new root.
		tx, root := block.NewTransaction(store, nil, nil, nil)
		root.SetBlock(&block_mock.SubBlock{}, true)
		mid := root.FollowRef(1, nil)
		mid.SetBlock(&block_mock.SubBlock{}, true)
		mid.FollowRef(1, nil).SetBlock(block_mock.NewExample(msg), true)

		// Store them and return the root.
		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// open starts a recording transaction at from and loads root and mid.
	open := func(from *block.BlockRef) *recording {
		// Load the root through a recording transaction.
		r := &recording{log: block.NewReplacementLog(from), puts: len(store.puts)}
		r.tx, r.root = block.NewTransaction(store, nil, from, nil)
		r.tx.RecordReplacements(r.log)
		rootBlk, err := r.root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
		if err != nil {
			t.Fatal(err)
		}

		// Load the middle page under it.
		r.mid = r.root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
		if _, err := r.mid.Unmarshal(ctx, block_mock.NewSubBlockBlock); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// puts returns, for each page put since r opened, whether a put found it.
	puts := func(r *recording) map[string]bool {
		found := make(map[string]bool)
		for i, key := range store.puts[r.puts:] {
			found[key] = found[key] || store.found[r.puts+i]
		}
		return found
	}

	// check writes the transaction, compares its record with the diff and
	// the puts, and checks it holds exactly the pages in held.
	check := func(name string, r *recording, from *block.BlockRef, held ...*block.BlockRef) {
		// The interval's record names exactly what the diff finds dead.
		to, _, err := r.tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		found := puts(r)
		rep := r.log.Take(to).Net(found)
		if err := block_mock.CheckReplacements(ctx, store, from, to, rep); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := block_mock.CheckExisted(ctx, store, from, to, rep, found); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(rep.Held) != len(held) || (len(held) == 1 && !rep.Held[0].EqualsRef(held[0])) {
			t.Fatalf("%s: held %v, want %v", name, rep.Held, held)
		}

		// A write with nothing dirty records nothing.
		to, _, err = r.tx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		rep = r.log.Take(to).Net(nil)
		if len(rep.Replaced)+len(rep.Removed)+len(rep.Held) != 0 {
			t.Fatalf("%s: clean interval recorded %d replaced, %d removed", name, len(rep.Replaced), len(rep.Removed))
		}
	}

	// Edit the leaf in place.
	from := chain("edit")
	r := open(from)
	leaf := r.mid.FollowRef(1, nil)
	if _, err := block_mock.UnmarshalExample(ctx, leaf); err != nil {
		t.Fatal(err)
	}
	leaf.SetBlock(block_mock.NewExample("edited"), true)
	check("edit", r, from)

	// Remove the mid subtree without loading the leaf.
	from = chain("remove")
	r = open(from)
	r.root.ClearRef(1)
	rootBlk, _ := r.root.GetBlock()
	rootBlk.(*block_mock.SubBlock).ExamplePtr = nil
	r.root.MarkDirty()
	check("remove", r, from)

	// Replace the mid subtree with a new one by ref.
	from = chain("swap")
	other := chain("other")
	r = open(from)
	r.mid.SetRefAtCursor(other, true)
	check("swap", r, from, other)

	// Move the leaf under a new middle block.
	from = chain("move")
	r = open(from)
	leaf = r.mid.FollowRef(1, nil)
	if _, err := block_mock.UnmarshalExample(ctx, leaf); err != nil {
		t.Fatal(err)
	}

	// Attach it under a fresh middle page.
	nmid := r.root.Detach(false)
	nmid.ClearAllRefs()
	nmid.SetBlock(&block_mock.SubBlock{}, true)
	nmid.SetRef(1, leaf)
	r.root.SetRef(1, nmid)
	check("move", r, from)

	// Make mid the root.
	from = chain("reroot")
	r = open(from)
	if err := r.tx.SetRoot(r.mid); err != nil {
		t.Fatal(err)
	}
	check("reroot", r, from, r.mid.GetRef())

	// Two writes in one interval: the first write's pages die in the second.
	from = chain("twice")
	r = open(from)
	leaf = r.mid.FollowRef(1, nil)
	leaf.SetBlock(block_mock.NewExample("first"), true)
	if _, _, err := r.tx.Write(ctx, false); err != nil {
		t.Fatal(err)
	}
	leaf.SetBlock(block_mock.NewExample("second"), true)
	check("twice", r, from)

	// pair stores root -> pair -> {left, right} and returns the root.
	pair := func(left, right string) *block.BlockRef {
		// Build the pair and its two leaves under one new root.
		tx, root := block.NewTransaction(store, nil, nil, nil)
		root.SetBlock(&block_mock.SubBlock{}, true)
		p := root.FollowRef(1, nil)
		p.SetBlock(&block_mock.Pair{}, true)
		p.FollowRef(1, nil).SetBlock(block_mock.NewExample(left), true)
		p.FollowRef(2, nil).SetBlock(block_mock.NewExample(right), true)

		// Store them and return the root.
		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// editLeft opens a pair tree at from and replaces its left leaf with msg.
	editLeft := func(from *block.BlockRef, msg string) *recording {
		// Load the root and the pair under it.
		r := &recording{log: block.NewReplacementLog(from), puts: len(store.puts)}
		r.tx, r.root = block.NewTransaction(store, nil, from, nil)
		r.tx.RecordReplacements(r.log)
		rootBlk, err := r.root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
		if err != nil {
			t.Fatal(err)
		}
		r.mid = r.root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
		pairBlk, err := r.mid.Unmarshal(ctx, block_mock.NewPairBlock)
		if err != nil {
			t.Fatal(err)
		}

		// Replace the left leaf.
		left := r.mid.FollowRef(1, pairBlk.(*block_mock.Pair).GetLeft())
		if _, err := block_mock.UnmarshalExample(ctx, left); err != nil {
			t.Fatal(err)
		}
		left.SetBlock(block_mock.NewExample(msg), true)
		return r
	}

	// Edit one leaf of a pair: the written pair names the right leaf, so its
	// edge was not counted.
	from = pair("sibling left", "sibling right")
	check("sibling", editLeft(from, "sibling edited"), from)

	// Edit a leaf so the pair matches one already stored: the found pair names
	// the right leaf, so its edge was counted at that pair's put.
	pair("found edited", "found right")
	from = pair("found left", "found right")
	check("found", editLeft(from, "found edited"), from)

	// Join four intervals that edit the leaf to x, y, x again and a last
	// value. The first tree's pages are original, so a put of one is counted;
	// the x leaf was created before it was replaced, so it is not.
	from = chain("join")
	r = open(from)
	midBlk, _ := r.mid.GetBlock()
	leaf = r.mid.FollowRef(1, midBlk.(*block_mock.SubBlock).GetExamplePtr())
	if _, err := block_mock.UnmarshalExample(ctx, leaf); err != nil {
		t.Fatal(err)
	}
	first := leaf.GetRef().Clone()

	// Write and take each interval.
	var steps block.ReplacementSteps
	var x *block.BlockRef
	for _, msg := range []string{"x", "y", "x", "join"} {
		leaf.SetBlock(block_mock.NewExample(msg), true)
		to, _, err := r.tx.Write(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if x == nil {
			x = leaf.GetRef().Clone()
		}
		steps = append(steps, r.log.Take(to))
	}

	// The first tree's pages are original and the x leaf is not.
	if !steps.Original(first) || !steps.Original(from) || steps.Original(x) {
		t.Fatalf("join: original first %v, root %v, x %v", steps.Original(first), steps.Original(from), steps.Original(x))
	}

	// The joined record matches the diff and the puts.
	joined := steps.Join()
	found := puts(r)
	rep := joined.Net(found)
	if err := block_mock.CheckReplacements(ctx, store, from, joined.To, rep); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := block_mock.CheckExisted(ctx, store, from, joined.To, rep, found); err != nil {
		t.Fatalf("join: %v", err)
	}
}
