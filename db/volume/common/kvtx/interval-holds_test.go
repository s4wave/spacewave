package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
)

// TestIntervalDroppedPagesAdoptedBeforeApply checks that a later writer can
// adopt a page whose overflow put left its edges for a pending death record.
// Surviving dropped parents fund those edges once, including edges to another
// dropped page; dead dropped parents take no counts from their children.
func TestIntervalDroppedPagesAdoptedBeforeApply(t *testing.T) {
	for _, mode := range []string{"dead", "adopted", "provisional"} {
		adopted := mode != "dead"
		for _, nested := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{false: "leaf", true: "nested"}[nested], func(t *testing.T) {
				// Store R0 -> A -> L before the interval writes its overflow pages.
				ctx := t.Context()
				v, _ := newPublicationTestVolume(t)
				entry := func(name string, refs ...*block.BlockRef) *block.PutBatchEntry {
					// Address this block and declare its complete child list.
					t.Helper()
					data := []byte(name)
					ref, err := block.BuildBlockRef(data, nil)
					if err != nil {
						t.Fatal(err)
					}
					return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
				}
				leaf := entry("leaf L")
				a := entry("page A", leaf.Ref)
				r0 := entry("root R0", a.Ref)
				if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{leaf, a, r0}, nil); err != nil {
					t.Fatal(err)
				}

				// The interval stores B -> L, or B -> C -> L, without counting
				// their child edges, then drops them before it names R0 again.
				b := entry("page B", leaf.Ref)
				dropped := []*block.PutBatchEntry{b}
				if nested {
					c := entry("page C", leaf.Ref)
					b = entry("page B", c.Ref)
					dropped = []*block.PutBatchEntry{c, b}
				}
				dead := &block.Replacements{Kept: []*block.BlockRef{a.Ref, leaf.Ref}, Existed: []*block.BlockRef{a.Ref, leaf.Ref}}
				for _, page := range dropped {
					page.Netted = true
					dead.Dropped = append(dead.Dropped, page.Ref)
				}
				if err := v.SetBucketRoots(ctx, "bucket", dropped, nil); err != nil {
					t.Fatal(err)
				}

				// A later writer adopts B before the older record applies. Its
				// found put counts the new parent of B, never B's stored children.
				if adopted {
					copyB := *b
					copyB.Netted = mode == "provisional"
					r1 := entry("root R1", a.Ref, b.Ref)
					r1.Netted = mode == "provisional"
					if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{r1, a, &copyB}, nil); err != nil {
						t.Fatal(err)
					}
				}
				if n := readHolds(t, v, leaf.Ref); n != 0 {
					t.Fatalf("leaf holds before applying the record=%d, want 0", n)
				}

				// Resolve and apply the record through the real reference graph,
				// as the collector's shadow application does, without deleting bytes.
				fates := make(map[string]bool)
				var changes map[string]int64
				err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
					var err error
					changes, _, err = rg.ResolveDeaths(ctx, r0.Ref, dead, func(node string, frees bool) error {
						if _, seen := fates[node]; seen {
							t.Fatalf("page %s decided twice", node)
						}
						fates[node] = frees
						return nil
					})
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				err = (&transactionRefGraph{volume: v}).write(ctx, func(rg *block_gc.RefGraph) error {
					n, err := rg.ChangeHolds(ctx, changes)
					if n != 0 {
						t.Fatalf("record underflows=%d, want 0", n)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}

				// Every dropped page shares its fate with the later adoption. A
				// surviving B, or C, gives L its second holder exactly once.
				if len(fates) != len(dropped) {
					t.Fatalf("decided %d pages, want %d dropped pages", len(fates), len(dropped))
				}
				for _, page := range dropped {
					frees, found := fates[block_gc.BlockIRI(page.Ref)]
					if !found || frees == adopted {
						t.Fatalf("page %s frees=%v found=%v, want frees=%v", page.Ref.MarshalString(), frees, found, !adopted)
					}
					if n := readHolds(t, v, page.Ref); n != 0 {
						t.Fatalf("dropped page holds after application=%d, want 0", n)
					}
				}
				var wantHolds uint64
				if adopted {
					wantHolds = 1
				}
				if n := readHolds(t, v, leaf.Ref); n != wantHolds {
					t.Fatalf("leaf holds after application=%d, want %d", n, wantHolds)
				}

				// A newer interval can drop the page again before naming its
				// record. The found put is then returned after the older record
				// has already consumed it to keep the dropped page alive.
				if mode == "provisional" {
					newer := &block.Replacements{Replaced: dead.Dropped, Found: []*block.BlockRef{b.Ref}}
					entries, root, err := block.EncodeDeadlist(2, nil, nil, newer)
					if err != nil {
						t.Fatal(err)
					}
					if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: block.DeadlistRootName(2), Ref: root}}); err != nil {
						t.Fatal(err)
					}
					if n := v.deadlistCountUnderflows.Load(); n != 0 {
						t.Fatalf("returning consumed provisional hold underflows=%d, want 0", n)
					}
				}
			})
		}
	}
}

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

	// name puts entries with a replay hold over the World they end at, as
	// the engine's naming put carries one, and names that World head.
	name := func(entries []*block.PutBatchEntry) {
		// Carry the root and move its ordinary named owner.
		t.Helper()
		data := []byte("hold " + entries[0].Ref.MarshalString())
		hold, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: hold, Data: data, Refs: []*block.BlockRef{entries[0].Ref}, RefsKnown: true, HoldsRoots: true})
		roots := []block.NamedRoot{{Name: "head", Ref: entries[0].Ref}}
		if err := v.SetBucketRoots(ctx, "bucket", entries, roots); err != nil {
			t.Fatal(err)
		}
	}

	// The old World holds leaf p. Writing p', then p again, names the old
	// World again: its leaf is original and its root is found under the
	// found hold and the same owner edge, so neither gains a count.
	old := world("p")
	p := scratchLeaf(t, scratch, old)
	name([]*block.PutBatchEntry{scratchEntry(t, scratch, old, p), scratchEntry(t, scratch, p)})
	rootHolds, leafHolds := readHolds(t, v, old), readHolds(t, v, p)
	name(interval(old, "p'", "p"))
	if n := readHolds(t, v, p); n != leafHolds {
		t.Fatalf("written back leaf holds=%d, want %d", n, leafHolds)
	}
	if n := readHolds(t, v, old); n != rootHolds {
		t.Fatalf("written back root holds=%d, want %d", n, rootHolds)
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

	// Another World holds leaf s. An overflow put of an interval stores two
	// parents of s afresh and finds s, all Netted: s gains one count, which
	// holds it until the record is named, and none per parent.
	s := scratchLeaf(t, scratch, world("s"))
	if err := v.SetBucketRoots(ctx, "other", []*block.PutBatchEntry{scratchEntry(t, scratch, s)}, []block.NamedRoot{{Name: "s", Ref: s}}); err != nil {
		t.Fatal(err)
	}
	parent := func(msg string) *block.PutBatchEntry {
		data := []byte(msg)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{s}, RefsKnown: true, Netted: true}
	}
	leaf := scratchEntry(t, scratch, s)
	leaf.Netted = true
	overflow := []*block.PutBatchEntry{parent("a over s"), parent("b over s"), leaf}
	if err := v.SetBucketRoots(ctx, "bucket", overflow, nil); err != nil {
		t.Fatal(err)
	}
	if n := readHolds(t, v, s); n != 1 {
		t.Fatalf("found Netted leaf holds=%d, want 1", n)
	}

	// The record holds s once per fresh parent and lists its found put.
	// Its ordinary Found leaf adds one physical parent independently of the
	// temporary put holder, which remains until child funding completes.
	generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Encode both fresh-parent claims and ordinary Found ownership.
	dead := &block.Replacements{CountsComplete: true, CountingGeneration: generation, Held: []*block.BlockRef{s, s}, Found: []*block.BlockRef{s}}
	record, first, err := block.EncodeDeadlist(1, nil, nil, dead)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "bucket", record, []block.NamedRoot{{Name: block.DeadlistRootName(1), Ref: first}}); err != nil {
		t.Fatal(err)
	}

	// Check the persisted Found edge, then account for provisional1, Held2
	// and that physical parent1 above the original named leaf's first holder.
	foundParents := 0
	for _, entry := range record {
		children, known, err := v.GetRefGraph().(*transactionRefGraph).GetBlockEdges(ctx, block_gc.BlockIRI(entry.Ref))
		if err != nil || !known {
			t.Fatalf("record child list: known=%v error=%v", known, err)
		}
		for _, child := range children {
			if child == block_gc.BlockIRI(s) {
				foundParents++
			}
		}
	}
	if foundParents != 1 || readHolds(t, v, s) != 4 {
		t.Fatalf("named record Found parents=%d leaf holds=%d, want1/4", foundParents, readHolds(t, v, s))
	}

	// Application returns Found and retires the header. Its nonleaf release
	// starts shadow recount before returning any physical child funding.
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: block.DeadlistDueRootName(1), Ref: first}, {Name: block.DeadlistRootName(1)}}); err != nil {
		t.Fatal(err)
	}
	if err := v.applyDeadlist(ctx, "bucket", block.DeadlistDueRootName(1), first, markBlocks(t, v), nil, dead); err != nil {
		t.Fatal(err)
	}

	// Resolve the retired parent before rebuilding any child counts.
	check, err := v.resolveReleases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
	if err != nil || !state.Phase.Walking() {
		t.Fatalf("retired parent did not request recount: state=%+v error=%v", state, err)
	}
	if check != nil {
		mark, err := v.GetRefGraph().(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
		if err != nil {
			t.Fatal(err)
		}
		v.checkReleases(check, mark)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if n := readHolds(t, v, s); n != 3 {
		t.Fatalf("retained physical Found edge leaves=%d, want3", n)
	}

	// Normal cleanup removes the now-unreached record tree, then rebuilds
	// from just the two stored parents and the other bucket's named leaf.
	for range 3 {
		var stats block_gc.Stats
		if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if n := readHolds(t, v, s); n != 2 {
		t.Fatalf("collected record leaves=%d, want2", n)
	}
	if _, found, err := v.GetBlock(ctx, s); err != nil || !found || !markBlocks(t, v).Live.Has(block_gc.BlockIRI(s)) {
		t.Fatalf("shared leaf stopped reading after record cleanup: found=%v error=%v", found, err)
	}
	if v.deadlistCountUnderflows.Load() != 0 || v.deadlistLivePages.Load() != 0 || v.releaseUnderflows.Load() != 0 || v.releaseLiveBlocks.Load() != 0 {
		t.Fatalf("record lifecycle underflow/live diagnostics: deadlist=%d/%d release=%d/%d", v.deadlistCountUnderflows.Load(), v.deadlistLivePages.Load(), v.releaseUnderflows.Load(), v.releaseLiveBlocks.Load())
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
