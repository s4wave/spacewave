package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
)

// TestIntervalPutsCountSharedPages walks a World of a root over one leaf
// through intervals that write a leaf back, and checks the count the naming
// put leaves on it: a page of the old tree written back gains nothing, and a
// page the interval created, dropped and created again while another World
// held it gains the new edge.
func TestIntervalPutsCountSharedPages(t *testing.T) {
	// Steps are recorded on a scratch store; only the naming put reaches the
	// Volume, as in page view.
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	scratch := block_mock.NewMockStore(hash.RecommendedHashType)

	// world stores a root over the leaf msg in the scratch store.
	world := func(msg string) *block.BlockRef {
		// Write the root and its leaf.
		tx, root := block.NewTransaction(scratch, nil, nil, nil)
		root.SetBlock(&block_mock.SubBlock{}, true)
		root.FollowRef(1, nil).SetBlock(block_mock.NewExample(msg), true)
		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// interval writes the leaf as each msg in turn from the World at from,
	// and returns the steps and the naming put's entries for the last World,
	// with original pages marked as the engine marks them.
	interval := func(from *block.BlockRef, msgs ...string) []*block.PutBatchEntry {
		// Load the root and leaf through a recording transaction.
		t.Helper()
		log := block.NewReplacementLog(from)
		tx, root := block.NewTransaction(scratch, nil, from, nil)
		tx.RecordReplacements(log)
		rootBlk, err := root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
		if err != nil {
			t.Fatal(err)
		}
		leaf := root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
		if _, err := block_mock.UnmarshalExample(ctx, leaf); err != nil {
			t.Fatal(err)
		}

		// Write each leaf and take each commit's step.
		var steps block.ReplacementSteps
		var to *block.BlockRef
		for _, msg := range msgs {
			leaf.SetBlock(block_mock.NewExample(msg), true)
			if to, _, err = tx.Write(ctx, false); err != nil {
				t.Fatal(err)
			}
			steps = append(steps, log.Take(to))
		}

		// Put the last World's pages, marked.
		leafRef := leaf.GetRef().Clone()
		entries := []*block.PutBatchEntry{
			scratchEntry(t, scratch, to, leafRef),
			scratchEntry(t, scratch, leafRef),
		}
		steps.MarkOriginal(entries)
		return entries
	}

	// name puts entries and names the World they end at head.
	name := func(entries []*block.PutBatchEntry) {
		t.Helper()
		roots := []block.NamedRoot{{Name: "head", Ref: entries[0].Ref}}
		if err := v.SetBucketRoots(ctx, "bucket", entries, roots); err != nil {
			t.Fatal(err)
		}
	}

	// The old World holds leaf p. Writing p', then p again, names the old
	// World again: its pages are original and gain no count.
	old := world("p")
	p := scratchLeaf(t, scratch, old)
	name([]*block.PutBatchEntry{scratchEntry(t, scratch, old, p), scratchEntry(t, scratch, p)})
	name(interval(old, "p'", "p"))
	if n := readHolds(t, v, p); n != 0 {
		t.Fatalf("written back leaf holds=%d, want 0", n)
	}
	if n := readHolds(t, v, old); n != 0 {
		t.Fatalf("written back root holds=%d, want 0", n)
	}

	// Another World holds leaf q. An interval creates q, drops it and creates
	// it again: q is not original, so its found put counts the new edge.
	q := scratchLeaf(t, scratch, world("q"))
	other := []*block.PutBatchEntry{scratchEntry(t, scratch, q)}
	if err := v.SetBucketRoots(ctx, "other", other, []block.NamedRoot{{Name: "head", Ref: q}}); err != nil {
		t.Fatal(err)
	}
	name(interval(old, "q", "r", "q"))
	if n := readHolds(t, v, q); n != 1 {
		t.Fatalf("recreated shared leaf holds=%d, want 1", n)
	}
}

// scratchLeaf returns the leaf under root in store.
func scratchLeaf(t *testing.T, store block.StoreOps, root *block.BlockRef) *block.BlockRef {
	// Read the root and take its leaf ref.
	t.Helper()
	_, cursor := block.NewTransaction(store, nil, root, nil)
	blk, err := cursor.Unmarshal(t.Context(), block_mock.NewSubBlockBlock)
	if err != nil {
		t.Fatal(err)
	}
	return blk.(*block_mock.SubBlock).GetExamplePtr()
}

// scratchEntry returns the put of ref from store, naming refs.
func scratchEntry(t *testing.T, store block.StoreOps, ref *block.BlockRef, refs ...*block.BlockRef) *block.PutBatchEntry {
	t.Helper()
	data, found, err := store.GetBlock(t.Context(), ref)
	if err != nil || !found {
		t.Fatalf("read %s: found=%v err=%v", ref.MarshalString(), found, err)
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
}

// readHolds reads ref's count in a graph transaction.
func readHolds(t *testing.T, v *Volume, ref *block.BlockRef) uint64 {
	// Read the count inside a graph read.
	t.Helper()
	var n uint64
	err := (&transactionRefGraph{volume: v}).read(t.Context(), func(rg *block_gc.RefGraph) error {
		var err error
		n, err = rg.GetHolds(t.Context(), block_gc.BlockIRI(ref))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
