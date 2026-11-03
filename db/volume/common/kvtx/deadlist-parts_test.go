package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestDeadlistPreparedRecordOwnership prepares a reused multipart tree through
// actual stage transactions, names it without bodies, and collects its orphan.
func TestDeadlistPreparedRecordOwnership(t *testing.T) {
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	b := publicationFor(t, "record-part-child", "", "B").Entries[0]
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b}, []block.NamedRoot{{Name: "head", Ref: b.Ref}}); err != nil {
		t.Fatal(err)
	}
	const occurrences = block.MaxDeadlistPartRefs * (block.MaxDeadlistPartRefs + 1)
	found := make([]*block.BlockRef, occurrences)
	for i := range found {
		found[i] = b.Ref
	}
	var last *block.BlockRef
	for seq := uint64(1); seq <= 2; seq++ {
		entries, ref, err := block.EncodeDeadlist(seq, nil, nil, &block.Replacements{Found: found})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 4 {
			t.Fatalf("physical repeated tree blocks=%d,want4", len(entries))
		}
		if seq == 1 {
			// Eviction keeps the funded incarnation's metadata. A fresh record
			// edge still needs its own count even when the body is not local.
			if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: b.Ref, Tombstone: true}}); err != nil {
				t.Fatal(err)
			}
		}
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if len(entry.Refs) > block.MaxDeadlistPartRefs {
				t.Fatal("unbounded physical fanout")
			}
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, []*block.PutBatchEntry{entry}); err != nil {
				t.Fatal(err)
			}
		}
		if n := readHolds(t, v, b.Ref); n != 1 {
			t.Fatalf("preparation funded reused child %d times,want1", n)
		}
		if seq == 1 {
			if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{b}); err != nil {
				t.Fatal(err)
			}
		}
		visits := 0
		if err := block.WalkDeadlist(ctx, v, ref, func(part *block.Deadlist) error { visits += len(part.Found); return nil }); err != nil || visits != occurrences {
			t.Fatalf("logical Found=%d,want%d error=%v", visits, occurrences, err)
		}
		if _, _, small, err := block.ReadSmallDeadlist(ctx, v, ref); err != nil || small {
			t.Fatalf("indexed record small=%v error=%v", small, err)
		}
		name := block.DeadlistRootName(seq)
		if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
			t.Fatal(err)
		}
		release()
		if err := v.walkSharing(ctx); err != nil {
			t.Fatal(err)
		}
		if n := readHolds(t, v, b.Ref); n != 1 {
			t.Fatalf("reused physical leaf funded child %d times,want1", n)
		}
		if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: name}}); err != nil {
			t.Fatal(err)
		}
		last = ref
	}
	collectBlocks(t, v, false, last)
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if exists, err := v.GetBlockExists(ctx, last); err != nil || exists {
		t.Fatalf("orphan header remains=%v/%v", exists, err)
	}
	if n := readHolds(t, v, b.Ref); n != 0 || v.releaseUnderflows.Load() != 0 {
		t.Fatalf("orphan tree holds=%d underflows=%d", n, v.releaseUnderflows.Load())
	}
}

// TestDeadlistRemovedSubtreeBudget discards a small record whose one removed
// root expands past the shadow planner's work bound, before any fate executes.
func TestDeadlistRemovedSubtreeBudget(t *testing.T) {
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	entries := make([]*block.PutBatchEntry, 0, block.MaxDeadlistPartRefs+2)
	var root *block.BlockRef
	for i := 0; i < block.MaxDeadlistPartRefs+2; i++ {
		e := publicationFor(t, "removed-budget", "", string(rune(i+1))).Entries[0]
		if root != nil {
			e.Refs = []*block.BlockRef{root}
		}
		entries = append(entries, e)
		root = e.Ref
	}
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: "head", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dead := &block.Replacements{Removed: []*block.BlockRef{root}, CountsComplete: true, CountingGeneration: generation}
	record, ref, err := block.EncodeDeadlist(1, root, nil, dead)
	if err != nil {
		t.Fatal(err)
	}
	name := block.DeadlistDueRootName(1)
	if err := v.SetBucketRoots(ctx, "bucket", record, []block.NamedRoot{{Name: name, Ref: ref}, {Name: "head"}}); err != nil {
		t.Fatal(err)
	}
	if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), root, dead); err != nil {
		t.Fatal(err)
	}
	state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
	if err != nil || !state.Phase.Walking() {
		t.Fatalf("large subtree did not select recovery: %+v/%v", state, err)
	}
	roots, err := v.GetRefGraph().GetOutgoingRefs(ctx, RootOwnerIRI("bucket", name))
	if err != nil || len(roots) != 0 {
		t.Fatalf("bounded recovery left due root=%v/%v", roots, err)
	}
	if v.deadlistLivePages.Load() != 0 || v.deadlistLeakedPages.Load() != 0 || v.deadlistCountUnderflows.Load() != 0 {
		t.Fatal("discarded fate plan published diagnostics")
	}
}

// TestNettedRefillRetainsCountedIncarnation checks that cache eviction changes
// byte existence without turning an already-funded page into a fresh birth.
func TestNettedRefillRetainsCountedIncarnation(t *testing.T) {
	for _, path := range []string{"prepare", "publish"} {
		t.Run(path, func(t *testing.T) {
			ctx := t.Context()
			v, _ := newPublicationTestVolume(t)
			b := publicationFor(t, "netted-refill", "", "B").Entries[0]
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b}, []block.NamedRoot{{Name: "head", Ref: b.Ref}}); err != nil {
				t.Fatal(err)
			}
			found := *b
			found.Netted = true
			first, err := v.PrepareOwnedBlockBatch(ctx, "bucket", []*block.PutBatchEntry{&found})
			if err != nil {
				t.Fatal(err)
			}
			if !first.Counting() || !first.Existed()[0] {
				t.Fatal("original Found put lacked its complete basis")
			}
			if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: b.Ref, Tombstone: true}}); err != nil {
				t.Fatal(err)
			}
			var refill *block.PublicationReceipt
			if path == "prepare" {
				refill, err = v.PrepareOwnedBlockBatch(ctx, "bucket", []*block.PutBatchEntry{&found})
			} else {
				pub := publicationFor(t, "netted-refill-publication", "", "one")
				pub.BucketID = "bucket"
				pub.Entries = []*block.PutBatchEntry{&found}
				refill, err = v.SubmitAtomic(ctx, pub)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := refill.Wait(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("refill existed=%v counting=%v generation=%d holds=%d", refill.Existed(), refill.Counting(), refill.CountingGeneration(), readHolds(t, v, b.Ref))
			if refill.Existed()[0] || refill.Counting() || refill.CountingGeneration() != first.CountingGeneration() {
				t.Fatal("body-only refill certified a fresh counting birth")
			}
			if n := readHolds(t, v, b.Ref); n != 2 {
				t.Fatalf("refill provisional holders=%d,want2", n)
			}
		})
	}
}

// TestCountErasureDuringSharing keeps an in-progress walk's cursor while a
// second physical writer removes and restores a sighted child's metadata.
func TestCountErasureDuringSharing(t *testing.T) {
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	b := publicationFor(t, "walking-erasure-child", "", "B").Entries[0]
	p := publicationFor(t, "walking-erasure-parent", "", "P").Entries[0]
	p.Refs = []*block.BlockRef{b.Ref}
	q := publicationFor(t, "walking-erasure-other", "", "Q").Entries[0]
	q.Refs = []*block.BlockRef{b.Ref}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, p, q}, []block.NamedRoot{{Name: "P", Ref: p.Ref}, {Name: "Q", Ref: q.Ref}}); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; ; i++ {
		if i == 10000 {
			t.Fatal("walk never sighted both parent edges")
		}
		state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.Phase.Walking() && readHolds(t, v, b.Ref) >= 1 {
			break
		}
		if err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
			_, err := rg.WalkSharing(ctx, func() bool { return true })
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		if _, err := rg.RemoveNodeRefs(ctx, block_gc.BlockIRI(b.Ref), false); err != nil {
			return err
		}
		return rg.AddRef(ctx, block_gc.NodeUnreferenced, block_gc.BlockIRI(b.Ref))
	}); err != nil {
		t.Fatal(err)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Doubt {
		t.Fatalf("lost sighted prefix certified complete: %+v Bholds=%d", state, readHolds(t, v, b.Ref))
	}
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
		t.Fatal(err)
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = OpenRefGraph(v.kvtxStore).GetSharing(ctx)
	if err != nil || state.Doubt || readHolds(t, v, b.Ref) != 1 {
		t.Fatalf("quiet recovery did not rebuild both edges: %+v Bholds=%d/%v", state, readHolds(t, v, b.Ref), err)
	}
}

// TestCachedParentFirstOwnership checks a real read-through cache write before
// the same parent first enters this Volume's counted ownership graph.
func TestCachedParentFirstOwnership(t *testing.T) {
	ctx := t.Context()
	v, _ := newPublicationTestVolume(t)
	b := publicationFor(t, "cached-parent-child", "", "B").Entries[0]
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b}, []block.NamedRoot{{Name: "head", Ref: b.Ref}}); err != nil {
		t.Fatal(err)
	}
	p := publicationFor(t, "cached-parent", "", "P").Entries[0]
	p.Refs = []*block.BlockRef{b.Ref}
	p.Netted = true
	if _, _, err := v.PutBlock(ctx, p.Data, &block.PutOpts{Refs: p.Refs, RefsKnown: true, CacheFill: true}); err != nil {
		t.Fatal(err)
	}
	pub := publicationFor(t, "cached-parent-publish", "", "one")
	pub.BucketID = "bucket"
	pub.Entries = []*block.PutBatchEntry{p}
	pub.RootName = "adopter"
	pub.Root = p.Ref
	receipt, err := v.SubmitAtomic(ctx, pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("cached parent existed=%v counting=%v Bholds=%d", receipt.Existed(), receipt.Counting(), readHolds(t, v, b.Ref))
	if receipt.Counting() && readHolds(t, v, b.Ref) < 1 {
		t.Fatal("cached but never-funded parent certified two live child holders with count0")
	}
	if err := v.walkSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if n := readHolds(t, v, b.Ref); n < 1 {
		t.Fatalf("first graph ownership did not rebuild its child edge:%d", n)
	}
	receipt.Release()
}
