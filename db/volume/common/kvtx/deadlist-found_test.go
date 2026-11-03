package kvtx

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
)

// TestDeadlistFoundRecountPreservesBirth checks a retained record across the
// restart recount boundary: a later parent keeps a dropped overflow page live.
// The real cursor/log derives its record, and the Volume's own release oracle
// must never report that the later live parent lost its page.
func TestDeadlistFoundRecountPreservesBirth(t *testing.T) {
	for _, fixture := range []struct {
		nested, recountBetween, releaseBefore bool
	}{{}, {nested: true}, {recountBetween: true}, {nested: true, recountBetween: true}, {releaseBefore: true}, {nested: true, releaseBefore: true}} {
		nested := fixture.nested
		name := map[bool]string{false: "leaf", true: "nested"}[nested]
		if fixture.recountBetween {
			name += "/recount-between-returns"
		}
		if fixture.releaseBefore {
			name += "/sole-found-mark-restart"
		}
		t.Run(name, func(t *testing.T) {
			// Keep the original World pinned before the discarded page exists.
			ctx := t.Context()
			v, store := newPublicationTestVolume(t)
			scratch := block_mock.NewMockStore(hash.RecommendedHashType)
			put := func(value block.Block) *block.BlockRef {
				t.Helper()
				ref, _, err := block.PutBlock(ctx, scratch, value)
				if err != nil {
					t.Fatal(err)
				}
				return ref
			}
			leaf := put(block_mock.NewExample("leaf L"))
			a := put(&block_mock.Pair{Left: leaf, Msg: "A"})
			r0 := put(&block_mock.Pair{Left: a, Msg: "root"})
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{scratchEntry(t, scratch, leaf), scratchEntry(t, scratch, a, leaf), scratchEntry(t, scratch, r0, a)}, []block.NamedRoot{{Name: "head", Ref: r0}}); err != nil {
				t.Fatal(err)
			}
			release, err := v.PinBucketRoot(ctx, r0)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			// Derive R0 -> R1 -> R0 through real recording cursor writes.
			log := block.NewReplacementLog(r0)
			tx, root := block.NewTransaction(scratch, nil, r0, nil)
			tx.RecordReplacements(log)
			if _, err := root.Unmarshal(ctx, block_mock.NewPairBlock); err != nil {
				t.Fatal(err)
			}
			bcur := root.FollowRef(2, nil)
			bcur.SetBlock(&block_mock.Pair{Left: leaf, Msg: "B"}, true)
			if nested {
				bcur.SetBlock(&block_mock.Pair{Msg: "B"}, true)
				bcur.FollowRef(1, nil).SetBlock(&block_mock.Pair{Left: leaf, Msg: "C"}, true)
			}
			r1, _, err := tx.Write(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			steps := block.ReplacementSteps{log.Take(r1)}
			b := bcur.GetRef().Clone()
			_, readB := block.NewTransaction(scratch, nil, b, nil)
			bvalue, err := block.UnmarshalBlock[block_mock.Pair](ctx, readB)
			if err != nil {
				t.Fatal(err)
			}

			// Only B and its optional fresh child overflowed into the Volume.
			overflow := []*block.PutBatchEntry{scratchEntry(t, scratch, b, bvalue.Left)}
			found := map[string]bool{b.MarshalString(): false, r0.MarshalString(): true}
			if nested {
				overflow = append(overflow, scratchEntry(t, scratch, bvalue.Left, leaf))
				found[bvalue.Left.MarshalString()] = false
			}
			for _, page := range overflow {
				page.Netted = true
			}
			if err := v.SetBucketRoots(ctx, "bucket", overflow, nil); err != nil {
				t.Fatal(err)
			}
			root.SetBlock(&block_mock.Pair{Left: a, Msg: "root"}, true)
			back, _, err := tx.Write(ctx, false)
			if err != nil || !back.EqualsRef(r0) {
				t.Fatalf("loop root=%v err=%v", back, err)
			}
			steps = append(steps, log.Take(back))
			oldDead := steps.Join().Net(found)
			if len(oldDead.Dropped) != len(overflow) {
				t.Fatalf("record dropped %d pages, want %d overflow pages", len(oldDead.Dropped), len(overflow))
			}
			oldDead.CountsComplete = true
			oldDead.CountingGeneration, err = OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			oldEntries, oldRef, err := block.EncodeDeadlist(1, r0, r0, oldDead)
			if err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "bucket", oldEntries, []block.NamedRoot{{Name: block.DeadlistRootName(1), Ref: oldRef}}); err != nil {
				t.Fatal(err)
			}

			// A later stored parent adopts B and two puts temporarily hold it.
			parent := scratchEntry(t, scratch, r1, a, b)
			parent.Netted = true
			foundB := scratchEntry(t, scratch, b, bvalue.Left)
			foundB.Netted = true
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{parent, foundB}, []block.NamedRoot{{Name: "head", Ref: r1}}); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{foundB}, nil); err != nil {
				t.Fatal(err)
			}
			newDead := block.NetPuts([]*block.PutBatchEntry{parent, foundB}, map[string]bool{r1.MarshalString(): true})
			newDead.Found = []*block.BlockRef{b, b}
			newDead.FallbackHeld = newDead.Held
			newDead.CountsComplete = true
			newDead.CountingGeneration, err = OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			newEntries, newRef, err := block.EncodeDeadlist(2, nil, r1, newDead)
			if err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "bucket", newEntries, []block.NamedRoot{{Name: block.DeadlistRootName(2), Ref: newRef}}); err != nil {
				t.Fatal(err)
			}
			if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{b}); err != nil {
				t.Fatal(err)
			}

			// Only Found now reaches B: the pin predates B, and neither current
			// root reaches the adopting parent. Both marks and a real sweep must
			// keep the pending record's bytes, including after reopening the Volume.
			if fixture.releaseBefore {
				if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head", Ref: r0}}); err != nil {
					t.Fatal(err)
				}
				walkMark := markBlocks(t, v)
				scanMark, err := v.refGraph.(*transactionRefGraph).MarkBlocksScan(ctx)
				if err != nil || !walkMark.Live.Has(block_gc.BlockIRI(b)) || !scanMark.Live.Has(block_gc.BlockIRI(b)) || walkMark.Live.Has(block_gc.BlockIRI(r1)) {
					t.Fatalf("sole Found mark: walk=%v scan=%v parent=%v error=%v", walkMark.Live.Has(block_gc.BlockIRI(b)), scanMark != nil && scanMark.Live.Has(block_gc.BlockIRI(b)), walkMark.Live.Has(block_gc.BlockIRI(r1)), err)
				}
				var stats block_gc.Stats
				if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
					t.Fatal(err)
				}
				if err := v.Close(); err != nil {
					t.Fatal(err)
				}
				v, err = NewVolume(ctx, "test/publication", store_kvkey.NewDefaultKVKey(), store, &store_kvtx.Config{}, false, false, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = v.Close() })
				for _, page := range overflow {
					if present, err := v.GetBlockExists(ctx, page.Ref); err != nil || !present {
						t.Fatalf("pending page after mark/restart: present=%v error=%v", present, err)
					}
				}
			}

			// A restored ledger requests this walk while preserving its records.
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			// A second restart must reconstruct exactly the same claims while
			// retaining only ordinary physical record ownership.
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("after repeated recount B=%d child=%d leaf=%d", readHolds(t, v, b), readHolds(t, v, bvalue.Left), readHolds(t, v, leaf))

			// Make the retained records due, then apply and retire them in order.
			for _, item := range []struct {
				seq  uint64
				ref  *block.BlockRef
				from *block.BlockRef
				dead *block.Replacements
			}{{1, oldRef, r0, oldDead}, {2, newRef, nil, newDead}} {
				if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: block.DeadlistDueRootName(item.seq), Ref: item.ref}, {Name: block.DeadlistRootName(item.seq)}}); err != nil {
					t.Fatal(err)
				}
				if err := v.applyDeadlist(ctx, "bucket", block.DeadlistDueRootName(item.seq), item.ref, markBlocks(t, v), item.from, item.dead); err != nil {
					t.Fatal(err)
				}
				queued, err := OpenRefGraph(v.kvtxStore).GetReleases(ctx, block_gc.BlockIRI(b))
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("after record %d B=%d child=%d leaf=%d queuedB=%d", item.seq, readHolds(t, v, b), readHolds(t, v, bvalue.Left), readHolds(t, v, leaf), queued)
				if fixture.recountBetween && item.seq == 1 {
					// Rebuild from the remaining physical graph without replaying
					// historical provisional claims.
					if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
						t.Fatal(err)
					}
					if err := v.walkSharing(ctx); err != nil {
						t.Fatal(err)
					}
				}
			}

			// The next pass's release oracle must preserve B under live R1.
			check, err := v.resolveReleases(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if check == nil {
				t.Fatal("release resolver skipped a counting Volume")
			}
			finalMark, err := v.refGraph.(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("after returns B=%d child=%d leaf=%d freedB=%v markedB=%v markedParent=%v underflows=%d", readHolds(t, v, b), readHolds(t, v, bvalue.Left), readHolds(t, v, leaf), check.freed.Has(block_gc.BlockIRI(b)), finalMark.Live.Has(block_gc.BlockIRI(b)), finalMark.Live.Has(block_gc.BlockIRI(r1)), v.releaseUnderflows.Load())
			v.checkReleases(check, finalMark)
			if n := v.releaseLiveBlocks.Load(); n != 0 {
				t.Errorf("recounted record releases %d live blocks", n)
			}
			// Retained physical parents may conservatively keep their edges
			// until ordinary sweep; recount never invents historical refunds.
			if !fixture.releaseBefore && readHolds(t, v, leaf) < 1 {
				t.Error("live parent edge lost funding")
			}

			// Releasing the final adopting parent must return its child edge once.
			if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head", Ref: r0}}); err != nil {
				t.Fatal(err)
			}
			if _, err := v.resolveReleases(ctx); err != nil {
				t.Fatal(err)
			}
			var cleanup block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &cleanup); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("after final parent returns B=%d child=%d leaf=%d releaseUnderflows=%d deadlistUnderflows=%d deadlistLive=%d", readHolds(t, v, b), readHolds(t, v, bvalue.Left), readHolds(t, v, leaf), v.releaseUnderflows.Load(), v.deadlistCountUnderflows.Load(), v.deadlistLivePages.Load())
			if n := readHolds(t, v, leaf); n != 0 {
				t.Errorf("leaf after final parent returns=%d, want only A's implicit holder", n)
			}
			if v.releaseUnderflows.Load() != 0 || v.deadlistCountUnderflows.Load() != 0 || v.deadlistLivePages.Load() != 0 {
				t.Error("record or release reported a count discrepancy")
			}
		})
	}
}

// TestDeadlistFoundPutAcrossRecount keeps an engine alive while another writer
// rebuilds the Volume's counts before the first engine names its found puts.
func TestDeadlistFoundPutAcrossRecount(t *testing.T) {
	// A current parent owns B before either temporary put occurs.
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	b := publicationFor(t, "recount-found", "", "B").Entries[0]
	parent := publicationFor(t, "recount-parent", "", "parent").Entries[0]
	parent.Refs = []*block.BlockRef{b.Ref}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, parent}, []block.NamedRoot{{Name: "head", Ref: parent.Ref}}); err != nil {
		t.Fatal(err)
	}

	// The unnamed engine finds B twice; its scalar claims precede its record.
	found := *b
	found.Netted = true
	var generation uint64
	for _, id := range []string{"first-found", "second-found"} {
		publication := publicationFor(t, id, "", "one")
		publication.BucketID, publication.TrackGC = "bucket", true
		publication.Entries = []*block.PutBatchEntry{&found}
		receipt, err := v.SubmitAtomic(ctx, publication)
		if err != nil {
			t.Fatal(err)
		}
		if err := receipt.Wait(ctx); err != nil {
			t.Fatal(err)
		}
		if !receipt.Counting() {
			t.Fatal("found put did not use complete counts")
		}
		generation = receipt.CountingGeneration()
	}
	if n := readHolds(t, v, b.Ref); n != 2 {
		t.Fatalf("temporary holders=%d, want 2", n)
	}

	// Reaffirming its stored parent removes B's temporary staging graph edge.
	kept := *parent
	kept.Counted = true
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&kept}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
	if err != nil || current == generation {
		t.Fatalf("generation before=%d after=%d error=%v", generation, current, err)
	}
	t.Logf("unnamed found holds after external recount=%d generation=%d→%d", readHolds(t, v, b.Ref), generation, current)

	// Bootstrap naming has no From or deaths, isolating the lost Found claims.
	dead := &block.Replacements{CountsComplete: true, CountingGeneration: generation, Found: []*block.BlockRef{b.Ref, b.Ref}}
	entries, ref, err := block.EncodeDeadlist(1, nil, parent.Ref, dead)
	if err != nil {
		t.Fatal(err)
	}
	name := block.DeadlistDueRootName(1)
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
		t.Fatal(err)
	}
	if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead); err != nil {
		t.Fatal(err)
	}
	check, err := v.resolveReleases(ctx)
	if err != nil || check == nil {
		t.Fatalf("release check=%v error=%v", check, err)
	}
	mark, err := v.refGraph.(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
	if err != nil {
		t.Fatal(err)
	}
	v.checkReleases(check, mark)
	t.Logf("after unnamed returns holds=%d freedB=%v markedB=%v underflows=%d", readHolds(t, v, b.Ref), check.freed.Has(block_gc.BlockIRI(b.Ref)), mark.Live.Has(block_gc.BlockIRI(b.Ref)), v.releaseUnderflows.Load())
	if v.releaseLiveBlocks.Load() != 0 || v.releaseUnderflows.Load() != 0 {
		t.Fatal("temporary holders lost by recount released the live page")
	}
}

// TestDeadlistFoundNamedDuringRecount names ordinary Found leaves before and
// after the child cursor, then retires their record and collects its graph.
func TestDeadlistFoundNamedDuringRecount(t *testing.T) {
	for _, phase := range []string{"children", "owners", "complete"} {
		t.Run(phase, func(t *testing.T) {
			ctx := t.Context()
			v, _ := newPublicationTestVolume(t)
			b := publicationFor(t, "midwalk-B", "", "B").Entries[0]
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b}, []block.NamedRoot{{Name: "head", Ref: b.Ref}}); err != nil {
				t.Fatal(err)
			}
			found := *b
			found.Netted = true
			for range 2 {
				if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&found}, nil); err != nil {
					t.Fatal(err)
				}
			}
			dead := &block.Replacements{Found: []*block.BlockRef{b.Ref, b.Ref}}
			entries, ref, err := block.EncodeDeadlist(1, nil, b.Ref, dead)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			for i := 0; ; i++ {
				if i == 10000 {
					t.Fatal("walk never reached naming boundary")
				}
				state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if (phase == "children" && state.Phase == block_gc.SharingChildren) || (phase == "owners" && state.Phase == block_gc.SharingOwners) || (phase == "complete" && state.Phase == block_gc.SharingComplete) {
					break
				}
				if err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
					_, err := rg.WalkSharing(ctx, func() bool { return true })
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			name := block.DeadlistDueRootName(1)
			if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := v.resolveReleases(ctx); err != nil {
				t.Fatal(err)
			}
			if n := readHolds(t, v, b.Ref); n < 1 {
				t.Fatalf("ordinary parent+record leaf holds=%d,want at least1", n)
			}
			// Staging transfers are uncounted: normalize any conservative surplus.
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if n := readHolds(t, v, b.Ref); n != 1 {
				t.Fatalf("reconstructed graph holds=%d,want1", n)
			}
			if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead); err != nil {
				t.Fatal(err)
			}
			var stats block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if n := readHolds(t, v, b.Ref); n != 0 || v.releaseUnderflows.Load() != 0 || v.releaseLiveBlocks.Load() != 0 {
				t.Fatalf("retired record holds=%d underflows=%d live=%d", n, v.releaseUnderflows.Load(), v.releaseLiveBlocks.Load())
			}
		})
	}
}

// TestDeadlistMixedGenerationFundsDiscardedParents checks that an uncertain
// interval funds even a discarded fresh parent's child edges before retiring.
func TestDeadlistMixedGenerationFundsDiscardedParents(t *testing.T) {
	// P keeps B live while the interval's first put finds B in G0.
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	b := publicationFor(t, "mixed-B", "", "B").Entries[0]
	p := publicationFor(t, "mixed-P", "", "P").Entries[0]
	p.Refs = []*block.BlockRef{b.Ref}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, p}, []block.NamedRoot{{Name: "head", Ref: p.Ref}}); err != nil {
		t.Fatal(err)
	}
	found := *b
	found.Netted = true
	first := publicationFor(t, "mixed-first", "", "one")
	first.BucketID, first.TrackGC, first.Entries = "bucket", true, []*block.PutBatchEntry{&found}
	old := submitPublication(t, v, first)
	if err := old.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}

	// A later fresh C -> B belongs to G1. The interval discards C, but another
	// named root adopts it before the uncertain record can apply.
	c := publicationFor(t, "mixed-C", "", "C").Entries[0]
	c.Refs, c.Netted = []*block.BlockRef{b.Ref}, true
	later := publicationFor(t, "mixed-later", "", "one")
	later.BucketID, later.TrackGC, later.Entries = "bucket", true, []*block.PutBatchEntry{c}
	current := submitPublication(t, v, later)
	if err := current.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if !old.Counting() || !current.Counting() || old.CountingGeneration() == current.CountingGeneration() {
		t.Fatal("fixture did not cross two complete counting generations")
	}
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "adopter", Ref: c.Ref}}); err != nil {
		t.Fatal(err)
	}

	// Normal Held excludes the dead C; the off-writer put summary must fund
	// C's edge instead. No unknown Found refund or historical death is allowed.
	dead := &block.Replacements{
		Dropped: []*block.BlockRef{c.Ref}, Found: []*block.BlockRef{b.Ref},
		FallbackHeld: block.NetPuts([]*block.PutBatchEntry{&found, c}, map[string]bool{c.Ref.MarshalString(): true}).Held,
	}
	if len(dead.FallbackHeld) != 1 || !dead.FallbackHeld[0].EqualsRef(b.Ref) {
		t.Fatal("put summary did not include the discarded parent's edge")
	}
	entries, ref, err := block.EncodeDeadlist(1, nil, p.Ref, dead)
	if err != nil {
		t.Fatal(err)
	}
	name := block.DeadlistDueRootName(1)
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
		t.Fatal(err)
	}
	if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead); err != nil {
		t.Fatal(err)
	}
	if n := readHolds(t, v, b.Ref); n < 1 || v.deadlistLivePages.Load() != 0 || v.deadlistCountUnderflows.Load() != 0 {
		t.Fatalf("mixed basis failed to cover P and C: holds=%d live=%d underflow=%d", n, v.deadlistLivePages.Load(), v.deadlistCountUnderflows.Load())
	}

	// Once the adopter leaves, an ordinary mark/sweep and recount repair any
	// conservative excess. The original current tree still reads afterward.
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "adopter"}}); err != nil {
		t.Fatal(err)
	}
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if present, err := v.GetBlockExists(ctx, b.Ref); err != nil || !present || readHolds(t, v, b.Ref) != 0 {
		t.Fatalf("current B after sweep/recount: present=%v holds=%d error=%v", present, readHolds(t, v, b.Ref), err)
	}
	if v.releaseLiveBlocks.Load() != 0 || v.releaseUnderflows.Load() != 0 {
		t.Fatal("mixed basis released a live or missing holder")
	}
}

// TestDeadlistFoundAcrossPhysicalIncarnation checks staged frontier release
// while another unresolved interval still owns provisional Found receipts.
func TestDeadlistFoundAcrossPhysicalIncarnation(t *testing.T) {
	for _, boundary := range []string{"missing-progress", "late-put", "late-running", "from-reincarnation"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := t.Context()
			v, raw := newPublicationTestVolume(t)
			b := publicationFor(t, "incarnation-B", "", "B").Entries[0]
			p := publicationFor(t, "incarnation-P", "", "P").Entries[0]
			p.Refs = []*block.BlockRef{b.Ref}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, p}, nil); err != nil {
				t.Fatal(err)
			}
			var generation uint64
			for i, original := range []*block.PutBatchEntry{b, b, p} {
				found := *original
				found.Netted = true
				pub := publicationFor(t, "incarnation-put-"+strconv.Itoa(i), "", "one")
				pub.BucketID, pub.TrackGC, pub.Entries = "bucket", true, []*block.PutBatchEntry{&found}
				receipt, err := v.SubmitAtomic(ctx, pub)
				if err != nil {
					t.Fatal(err)
				}
				if err := receipt.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				if !receipt.Counting() {
					t.Fatal("original found receipt was not complete")
				}
				generation = receipt.CountingGeneration()
			}
			// The known stored parent adopts B's staging edge. Releasing P's frontier
			// consumes its one shared hold without a logical parent free or recount.
			kept := *p
			kept.Counted = true
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&kept}, nil); err != nil {
				t.Fatal(err)
			}
			if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{p.Ref}); err != nil {
				t.Fatal(err)
			}
			// No-op and rejected erasure cannot publish a new receipt basis.
			absent := publicationFor(t, "incarnation-absent", "", "absent").Entries[0]
			if err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
				_, err := rg.RemoveNodeRefs(ctx, block_gc.BlockIRI(absent.Ref), false)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			unchanged, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil || unchanged != generation {
				t.Fatalf("absent erasure changed basis: %d/%v", unchanged, err)
			}
			failure := errors.New("reject count erasure")
			raw.mu.Lock()
			raw.errorCommit = failure
			raw.mu.Unlock()
			err = v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
				for _, node := range []*block.BlockRef{b.Ref, p.Ref} {
					if _, err := rg.RemoveNodeRefs(ctx, block_gc.BlockIRI(node), false); err != nil {
						return err
					}
				}
				return nil
			})
			raw.mu.Lock()
			raw.errorCommit = nil
			raw.mu.Unlock()
			if !errors.Is(err, failure) {
				t.Fatalf("rejected erasure error=%v", err)
			}
			unchanged, err = OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil || unchanged != generation || readHolds(t, v, b.Ref) != 2 {
				t.Fatalf("rejected erasure changed basis or counts: %d/%v", unchanged, err)
			}

			var stats block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
				t.Fatal(err)
			}
			for _, ref := range []*block.BlockRef{b.Ref, p.Ref} {
				if exists, err := v.GetBlockExists(ctx, ref); err != nil || exists {
					t.Fatalf("abandoned frontier survived ordinary mark: %v/%v", exists, err)
				}
			}
			current, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil || current != generation+1 {
				t.Fatalf("two-node sweep basis %d→%d error=%v, want one advance", generation, current, err)
			}
			t.Logf("physical sweep generation %d→%d blocks=%+v", generation, current, stats)

			// A new ordinary parent now owns a fresh incarnation of B.
			r := publicationFor(t, "incarnation-R", "", "R").Entries[0]
			r.Refs = []*block.BlockRef{b.Ref}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, r}, []block.NamedRoot{{Name: "head", Ref: r.Ref}}); err != nil {
				t.Fatal(err)
			}
			// A cache tombstone changes bytes, retaining the same funded metadata.
			if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: b.Ref, Tombstone: true}}); err != nil {
				t.Fatal(err)
			}
			cached, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil || cached != current {
				t.Fatalf("cache-only eviction changed basis %d→%d/%v", current, cached, err)
			}
			if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{b}); err != nil {
				t.Fatal(err)
			}

			// A historical From address may itself acquire a new incarnation.
			// The old certificate must not return that new root's first holder.
			var from *block.BlockRef
			if boundary == "from-reincarnation" {
				unpin, err := v.PinBucketRoot(ctx, r.Ref)
				if err != nil {
					t.Fatal(err)
				}
				defer unpin()
				if err := v.walkSharing(ctx); err != nil {
					t.Fatal(err)
				}
				fresh := *p
				fresh.HoldsRoots = true
				r2 := publicationFor(t, "incarnation-From-root", "", "R2").Entries[0]
				r2.Refs = []*block.BlockRef{p.Ref}
				pub := publicationFor(t, "incarnation-From-publication", "", "one")
				pub.BucketID = "bucket"
				pub.Entries = []*block.PutBatchEntry{&fresh, r2}
				pub.Roots = []block.NamedRoot{{Name: "head", Ref: r2.Ref}}
				receipt, err := v.SubmitAtomic(ctx, pub)
				if err != nil {
					t.Fatal(err)
				}
				if err := receipt.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				if !receipt.Counting() || receipt.CountingGeneration() == generation {
					t.Fatal("new From incarnation lacked its own certified basis")
				}
				from = p.Ref
			}
			dead := &block.Replacements{CountsComplete: true, CountingGeneration: generation, Found: []*block.BlockRef{b.Ref, b.Ref, p.Ref}}
			entries, ref, err := block.EncodeDeadlist(1, from, r.Ref, dead)
			if err != nil {
				t.Fatal(err)
			}
			name := block.DeadlistDueRootName(1)
			if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			if boundary == "late-put" || boundary == "late-running" {
				// A complete walk cannot certify fresh births while a stored part
				// already names an absent historical child of that same address.
				if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
					t.Fatal(err)
				}
				if boundary == "late-running" {
					for i := 0; ; i++ {
						if i == 10000 {
							t.Fatal("walk never sighted missing history")
						}
						state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
						if err != nil {
							t.Fatal(err)
						}
						if state.Doubt && state.Phase.Walking() {
							break
						}
						if err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
							_, err := rg.WalkSharing(ctx, func() bool { return true })
							return err
						}); err != nil {
							t.Fatal(err)
						}
					}
				} else if err := v.walkSharing(ctx); err != nil {
					t.Fatal(err)
				}
				state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
				if err != nil {
					t.Fatal(err)
				}
				receipt, err := v.PrepareOwnedBlockBatch(ctx, "bucket", []*block.PutBatchEntry{p})
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("missing historical child beforelateput state=%+v lateReceiptCounting=%v", state, receipt.Counting())
				if !state.Doubt || receipt.Counting() {
					t.Fatal("missing historical edge certified an uncounted fresh birth")
				}
			}

			// Safe historical retirement must progress through the normal collector,
			// even when this very record gives the initial mark a metadata hole.
			passCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
			var progress block_gc.Stats
			err = v.SweepUnreachedBlocks(passCtx, v.GetRefGraph(), &progress)
			cancel()
			roots, rootErr := v.GetRefGraph().GetOutgoingRefs(ctx, RootOwnerIRI("bucket", name))
			if err != nil || rootErr != nil || len(roots) != 0 {
				t.Fatalf("uncertain metadata did not retire through normal pass: roots=%v error=%v/%v unknown=%d", roots, err, rootErr, progress.UnknownBlocks)
			}
			var next block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &next); err != nil || next.UnknownBlocks != 0 {
				t.Fatalf("retired metadata hole persisted: unknown=%d error=%v", next.UnknownBlocks, err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
			if err != nil || state.Doubt || !state.Phase.Counting() {
				t.Fatalf("retired history did not regain known counts: %+v/%v", state, err)
			}
			check, err := v.resolveReleases(ctx)
			if err != nil || check == nil {
				t.Fatalf("release check %v/%v", check, err)
			}
			mark, err := v.refGraph.(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
			if err != nil {
				t.Fatal(err)
			}
			v.checkReleases(check, mark)
			t.Logf("fresh B holds=%d freed=%v reached=%v underflows=%d", readHolds(t, v, b.Ref), check.freed.Has(block_gc.BlockIRI(b.Ref)), mark.Live.Has(block_gc.BlockIRI(b.Ref)), v.releaseUnderflows.Load())
			if v.releaseLiveBlocks.Load() != 0 || v.releaseUnderflows.Load() != 0 {
				t.Fatal("old Found receipt consumed a fresh incarnation's holder")
			}
		})
	}
}

// TestPhysicalErasureRebuildsSurvivingParent checks a partial ordinary sweep:
// the old stored parent's edge must fund a child's new incarnation too.
func TestPhysicalErasureRebuildsSurvivingParent(t *testing.T) {
	ctx := t.Context()
	v, raw := newPublicationTestVolume(t)
	b := publicationFor(t, "surviving-child", "", "B").Entries[0]
	var p *block.PutBatchEntry
	for n := 0; ; n++ {
		p = publicationFor(t, "surviving-parent-"+strconv.Itoa(n), "", "P").Entries[0]
		p.Refs = []*block.BlockRef{b.Ref}
		if block_gc.BlockIRI(p.Ref) > block_gc.BlockIRI(b.Ref) {
			break
		}
	}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, p}, nil); err != nil {
		t.Fatal(err)
	}
	found := *p
	found.Netted = true
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&found}, nil); err != nil {
		t.Fatal(err)
	}
	kept := *p
	kept.Counted = true
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&kept}, nil); err != nil {
		t.Fatal(err)
	}
	if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{p.Ref}); err != nil {
		t.Fatal(err)
	}
	sweepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	raw.mu.Lock()
	raw.writersWaiting = true
	raw.commitObserver = func() {
		be, err := v.GetBlockExists(ctx, b.Ref)
		if err != nil {
			panic(err)
		}
		pe, err := v.GetBlockExists(ctx, p.Ref)
		if err != nil {
			panic(err)
		}
		if !be && pe {
			cancel()
		}
	}
	raw.mu.Unlock()
	var stats block_gc.Stats
	err := v.SweepUnreachedBlocks(sweepCtx, v.GetRefGraph(), &stats)
	raw.mu.Lock()
	raw.writersWaiting = false
	raw.commitObserver = nil
	raw.mu.Unlock()
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if exists, err := v.GetBlockExists(ctx, p.Ref); err != nil || !exists {
		t.Fatalf("partial sweep lost surviving parent: %v/%v", exists, err)
	}
	if exists, err := v.GetBlockExists(ctx, b.Ref); err != nil || exists {
		t.Fatalf("partial sweep did not erase child: %v/%v", exists, err)
	}
	// R1 adopts the fresh child. Its pin remains after R2 adopts stored P.
	r1 := publicationFor(t, "surviving-root-1", "", "R1").Entries[0]
	r1.Refs = []*block.BlockRef{b.Ref}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, r1}, []block.NamedRoot{{Name: "head", Ref: r1.Ref}}); err != nil {
		t.Fatal(err)
	}
	unpin, err := v.PinBucketRoot(ctx, r1.Ref)
	if err != nil {
		t.Fatal(err)
	}
	defer unpin()
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	r2 := publicationFor(t, "surviving-root-2", "", "R2").Entries[0]
	r2.Refs = []*block.BlockRef{p.Ref}
	pub := publicationFor(t, "surviving-adoption", "", "one")
	pub.BucketID = "bucket"
	pub.Entries = []*block.PutBatchEntry{p, r2}
	pub.Roots = []block.NamedRoot{{Name: "head", Ref: r2.Ref}}
	receipt, err := v.SubmitAtomic(ctx, pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("new adoption counting=%v generation=%d B holds=%d", receipt.Counting(), receipt.CountingGeneration(), readHolds(t, v, b.Ref))
	if !receipt.Counting() {
		t.Fatal("post-recovery adoption did not regain complete counts")
	}
	if n := readHolds(t, v, b.Ref); n < 1 {
		t.Fatalf("two live parent edges have only %d shared holds", n)
	}
}
