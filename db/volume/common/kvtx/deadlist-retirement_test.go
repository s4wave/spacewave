package kvtx

import (
	"errors"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// TestDeadlistRetirementIsAtomic checks failed application, durable retry, and
// orphan removal on either side of a repeated recount's owner cursor.
func TestDeadlistRetirementIsAtomic(t *testing.T) {
	for _, outcome := range []string{"commit-retry", "orphan-before", "orphan-after", "missing-part", "missing-part-before-name"} {
		t.Run(outcome, func(t *testing.T) {
			// A stored parent owns B while its found put waits in a due record.
			ctx := t.Context()
			v, store := newPublicationTestVolume(t)
			b := publicationFor(t, "atomic-B", "", "B").Entries[0]
			p := publicationFor(t, "atomic-P", "", "P").Entries[0]
			p.Refs = []*block.BlockRef{b.Ref}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b, p}, []block.NamedRoot{{Name: "head", Ref: p.Ref}}); err != nil {
				t.Fatal(err)
			}
			found, kept := *b, *p
			found.Netted, kept.Counted = true, true
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&found}, nil); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{&kept}, nil); err != nil {
				t.Fatal(err)
			}
			generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dead := &block.Replacements{CountsComplete: true, CountingGeneration: generation, Found: []*block.BlockRef{b.Ref}}
			entries, ref, err := block.EncodeDeadlist(1, nil, p.Ref, dead)
			if err != nil {
				t.Fatal(err)
			}
			name := block.DeadlistDueRootName(1)
			if outcome == "missing-part-before-name" {
				stage, releaseStage, err := v.OpenStage(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer releaseStage()
				for _, entry := range entries {
					if _, err := v.PrepareStagedBlockBatch(ctx, stage, []*block.PutBatchEntry{entry}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: entries[0].Ref, Tombstone: true}}); err != nil {
					t.Fatal(err)
				}
				pub := publicationFor(t, "missing-part-naming", "", "one")
				pub.BucketID = "bucket"
				pub.Entries = nil
				pub.Roots = []block.NamedRoot{{Name: name, Ref: ref}}
				receipt, err := v.SubmitAtomic(ctx, pub)
				if err != nil {
					t.Fatal(err)
				}
				if err := receipt.Wait(ctx); err != nil {
					t.Fatalf("historical cache miss failed foreground Name: %v", err)
				}
				if receipt.Counting() {
					t.Fatal("Name-triggered recount certified an incomplete publication")
				}
				releaseStage()
				state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
				if err != nil || !state.Phase.Walking() {
					t.Fatalf("missing leaf did not choose deferred recovery: %+v/%v", state, err)
				}
				var stats block_gc.Stats
				if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
					t.Fatal(err)
				}
				roots, err := v.GetRefGraph().GetOutgoingRefs(ctx, RootOwnerIRI("bucket", name))
				if err != nil || len(roots) != 0 {
					t.Fatalf("missing prepared part stranded due name: %v/%v", roots, err)
				}
				return
			}
			if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}

			if outcome == "missing-part" {
				// Byte-only eviction preserves count metadata and its generation,
				// but a local collector cannot read that historical list anymore.
				if _, err := v.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: entries[0].Ref, Tombstone: true}}); err != nil {
					t.Fatal(err)
				}
				var stats block_gc.Stats
				if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
					t.Fatal(err)
				}
				roots, err := v.GetRefGraph().GetOutgoingRefs(ctx, RootOwnerIRI("bucket", name))
				if err != nil || len(roots) != 0 {
					t.Fatalf("missing historical part stranded due record: %v/%v", roots, err)
				}
				if v.deadlistLivePages.Load() != 0 || v.deadlistCountUnderflows.Load() != 0 {
					t.Fatal("missing history published a fate")
				}
				return
			}

			// Check the durable receipt and deferred return together.
			checkState := func(named bool, releases uint64) {
				t.Helper()
				rg := OpenRefGraph(v.kvtxStore)
				roots, err := rg.GetOutgoingRefs(ctx, RootOwnerIRI("bucket", name))
				if err != nil || (len(roots) != 0) != named {
					t.Fatalf("retirement named=%v roots=%v error=%v", named, roots, err)
				}
				n, err := rg.GetReleases(ctx, block_gc.BlockIRI(b.Ref))
				if err != nil || n != releases {
					t.Fatalf("retirement releases=%d want=%d error=%v", n, releases, err)
				}
			}

			if outcome == "commit-retry" {
				// Revision changes share the actual physical commit: reads, an
				// unchanged direct operation and a rejected commit leave it alone.
				revision := func() uint64 {
					t.Helper()
					var value uint64
					err := (&transactionRefGraph{volume: v}).readSnapshot(ctx, func(tx kvtx.Tx, _ *block_gc.RefGraph) error {
						var err error
						value, err = readPhysicalRevision(ctx, tx, v.physicalRevisionKey())
						return err
					})
					if err != nil {
						t.Fatal(err)
					}
					return value
				}
				before := revision()
				if err := v.withDirectAtomic(ctx, nil); err != nil || revision() != before {
					t.Fatalf("unchanged transaction advanced revision: %v", err)
				}

				// A rejected commit changes neither the name, mark holder nor queue.
				failure := errors.New("reject deadlist retirement")
				store.mu.Lock()
				store.errorCommit = failure
				store.mu.Unlock()
				err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead)
				store.mu.Lock()
				store.errorCommit = nil
				store.mu.Unlock()
				if !errors.Is(err, failure) {
					t.Fatalf("application failure=%v", err)
				}
				checkState(true, 0)
				if revision() != before {
					t.Fatal("rejected commit advanced physical revision")
				}
				if n := readHolds(t, v, b.Ref); n != 2 || v.deadlistsChecked.Load() != 0 {
					t.Fatalf("rejected retirement changed counts=%d or receipt=%d", n, v.deadlistsChecked.Load())
				}
				if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead); err != nil {
					t.Fatal(err)
				}
				checkState(false, 1)
				if revision() != before+1 {
					t.Fatal("retirement did not advance its one physical commit")
				}

				// Reopen before the return settles; retrying the old due snapshot
				// cannot queue the same return a second time.
				if err := v.Close(); err != nil {
					t.Fatal(err)
				}
				v, err = NewVolume(ctx, "test/publication", store_kvkey.NewDefaultKVKey(), store, &store_kvtx.Config{}, false, false, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = v.Close() })
				if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), nil, dead); err != nil {
					t.Fatal(err)
				}
				checkState(false, 1)
				if _, err := v.resolveReleases(ctx); err != nil {
					t.Fatal(err)
				}
				if n := readHolds(t, v, b.Ref); n != 1 || v.releaseUnderflows.Load() != 0 {
					t.Fatalf("retry returns holds=%d underflows=%d", n, v.releaseUnderflows.Load())
				}
				return
			}

			// A later-sorting root keeps Owners open after this record is sighted.
			for nonce := 0; ; nonce++ {
				if nonce == 10000 {
					t.Fatal("could not construct the final owner sentinel")
				}
				last := publicationFor(t, "orphan-last-"+strconv.Itoa(nonce), "", "last").Entries[0]
				if block_gc.BlockIRI(last.Ref) <= block_gc.BlockIRI(ref) {
					continue
				}
				if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{last}, []block.NamedRoot{{Name: "last", Ref: last.Ref}}); err != nil {
					t.Fatal(err)
				}
				break
			}
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}

			// An old marker cannot authorize returns before this walk is done.
			for slices := 0; ; slices++ {
				if slices == 10000 {
					t.Fatal("sharing walk did not reach orphan boundary")
				}
				ready := false
				err := v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
					state, err := rg.GetSharing(ctx)
					if err != nil {
						return err
					}
					count, err := rg.GetHolds(ctx, block_gc.BlockIRI(b.Ref))
					if err != nil {
						return err
					}
					ready = state.Phase == block_gc.SharingOwners && (outcome == "orphan-before" || count == 1)
					if ready {
						return nil
					}
					if !state.Phase.Walking() {
						t.Fatal("orphan boundary was skipped")
					}
					_, err = rg.WalkSharing(ctx, func() bool { return true })
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				if ready {
					break
				}
			}
			if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: name}}); err != nil {
				t.Fatal(err)
			}
			checkState(false, 0)
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			want := uint64(1)
			if n := readHolds(t, v, b.Ref); n != want {
				t.Fatalf("orphan surplus=%d want=%d", n, want)
			}

			// Releasing a record name leaves its ordinary leaf edge until
			// physical sweep; only then can recount remove that funding.
			var stats block_gc.Stats
			if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
				t.Fatal(err)
			}

			// A later complete walk repairs the permitted conservative surplus.
			if _, err := OpenRefGraph(v.kvtxStore).RecountSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if n := readHolds(t, v, b.Ref); n != 0 {
				t.Fatalf("orphan after recount holds=%d, want 0", n)
			}
		})
	}
}

// TestDeadlistDroppedAdoptionAfterRead adopts a stored overflow page after the
// resolver read its zero count but before its physical count changes commit.
func TestDeadlistDroppedAdoptionAfterRead(t *testing.T) {
	// The current root owns A -> L; discarded B -> L was stored Netted.
	ctx := t.Context()
	v, store := newPublicationTestVolume(t)
	leaf := publicationFor(t, "read-race-L", "", "L").Entries[0]
	a := publicationFor(t, "read-race-A", "", "A").Entries[0]
	a.Refs = []*block.BlockRef{leaf.Ref}
	r0 := publicationFor(t, "read-race-R0", "", "R0").Entries[0]
	r0.Refs = []*block.BlockRef{a.Ref}
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{leaf, a, r0}, []block.NamedRoot{{Name: "head", Ref: r0.Ref}}); err != nil {
		t.Fatal(err)
	}
	b := publicationFor(t, "read-race-B", "", "B").Entries[0]
	b.Refs, b.Netted = []*block.BlockRef{leaf.Ref}, true
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{b}, nil); err != nil {
		t.Fatal(err)
	}
	generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dead := &block.Replacements{
		CountsComplete: true, CountingGeneration: generation,
		Dropped: []*block.BlockRef{b.Ref}, Existed: []*block.BlockRef{r0.Ref},
	}
	entries, ref, err := block.EncodeDeadlist(1, r0.Ref, r0.Ref, dead)
	if err != nil {
		t.Fatal(err)
	}
	name := block.DeadlistDueRootName(1)
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
		t.Fatal(err)
	}

	// The store's existing gate pauses only the next write transaction, after
	// applyDeadlist already resolved its independent read snapshot.
	mark := markBlocks(t, v)
	gate := store.arm(t)
	applied := make(chan error, 1)
	go func() { applied <- v.applyDeadlist(ctx, "bucket", name, ref, mark, r0.Ref, dead) }()
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// A legitimate later stored parent adopts B in the same generation.
	parent := publicationFor(t, "read-race-R1", "", "R1").Entries[0]
	parent.Refs = []*block.BlockRef{a.Ref, b.Ref}
	copyB := *b
	copyB.Netted = false
	if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{parent, &copyB}, []block.NamedRoot{{Name: "adopter", Ref: parent.Ref}}); err != nil {
		t.Fatal(err)
	}
	gate.unblock()
	if err := <-applied; err != nil {
		t.Fatal(err)
	}
	t.Logf("late adoption B=%d L=%d checked=%d", readHolds(t, v, b.Ref), readHolds(t, v, leaf.Ref), v.deadlistsChecked.Load())
	if n := readHolds(t, v, leaf.Ref); n < 1 {
		t.Fatalf("same-generation late adoption left L with %d shared holds, want A and B", n)
	}
	// R0 remains the current named World across this round trip. Historical
	// From settlement must not consume that independent head's holder.
	check, err := v.resolveReleases(ctx)
	if err != nil || check == nil {
		t.Fatalf("root return check=%v/%v", check, err)
	}
	after, err := v.refGraph.(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
	if err != nil {
		t.Fatal(err)
	}
	v.checkReleases(check, after)
	if v.releaseLiveBlocks.Load() != 0 || v.releaseUnderflows.Load() != 0 {
		t.Fatalf("From settlement released current World: live=%d underflow=%d", v.releaseLiveBlocks.Load(), v.releaseUnderflows.Load())
	}
}

// TestDeadlistDroppedAdoptionAfterCommit adopts a stored overflow page after its
// logical death committed while shadow collection kept the physical bytes.
func TestDeadlistDroppedAdoptionAfterCommit(t *testing.T) {
	for _, death := range []string{"dropped", "replaced", "removed", "release"} {
		t.Run(death, func(t *testing.T) {
			// The current root owns A -> L; discarded B -> L was stored Netted.
			ctx := t.Context()
			v, _ := newPublicationTestVolume(t)
			leaf := publicationFor(t, "read-race-L", "", "L").Entries[0]
			a := publicationFor(t, "read-race-A", "", "A").Entries[0]
			a.Refs = []*block.BlockRef{leaf.Ref}
			r0 := publicationFor(t, "read-race-R0", "", "R0").Entries[0]
			r0.Refs = []*block.BlockRef{a.Ref}
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{leaf, a, r0}, []block.NamedRoot{{Name: "head", Ref: r0.Ref}}); err != nil {
				t.Fatal(err)
			}
			b := publicationFor(t, "read-race-B", "", "B").Entries[0]
			b.Refs, b.Netted = []*block.BlockRef{leaf.Ref}, death == "dropped"
			puts := []*block.PutBatchEntry{b}
			if death != "dropped" {
				// Ordinary batches carry their found children too, funding B's
				// new edge. Dropped Netted edges deliberately remain unfunded.
				puts = append(puts, leaf)
			}
			if err := v.SetBucketRoots(ctx, "bucket", puts, nil); err != nil {
				t.Fatal(err)
			}
			generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
			if err != nil {
				t.Fatal(err)
			}
			dead := &block.Replacements{
				CountsComplete: true, CountingGeneration: generation,
				Existed: []*block.BlockRef{r0.Ref, leaf.Ref},
			}
			switch death {
			case "dropped":
				dead.Dropped = []*block.BlockRef{b.Ref}
			case "replaced":
				dead.Replaced = []*block.BlockRef{b.Ref}
			case "removed":
				dead.Removed = []*block.BlockRef{b.Ref}
			}
			entries, ref, err := block.EncodeDeadlist(1, r0.Ref, r0.Ref, dead)
			if err != nil {
				t.Fatal(err)
			}
			name := block.DeadlistDueRootName(1)
			if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}

			// Shadow application retires the record but keeps B's stored bytes.
			if death == "release" {
				if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{b.Ref}); err != nil {
					t.Fatal(err)
				}
				if _, err := v.resolveReleases(ctx); err != nil {
					t.Fatal(err)
				}
				// The resolver stops at the parent before consuming its child's
				// queued return. Recount discards that obsolete return afterward.
				queued, err := OpenRefGraph(v.kvtxStore).GetReleases(ctx, block_gc.BlockIRI(leaf.Ref))
				if err != nil || queued != 1 || readHolds(t, v, leaf.Ref) != 1 {
					t.Fatalf("parent recovery consumed later releases: queued=%d L=%d error=%v", queued, readHolds(t, v, leaf.Ref), err)
				}
			} else if err := v.applyDeadlist(ctx, "bucket", name, ref, markBlocks(t, v), r0.Ref, dead); err != nil {
				t.Fatal(err)
			}

			state, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
			if err != nil || state.Phase.Counting() {
				t.Fatalf("retained parent's counts stayed complete: state=%+v error=%v", state, err)
			}

			// A legitimate later stored parent adopts B while recount is incomplete.
			parent := publicationFor(t, "read-race-R1", "", "R1").Entries[0]
			parent.Refs = []*block.BlockRef{a.Ref, b.Ref}
			copyB := *b
			copyB.Netted = false
			if err := v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{parent, &copyB}, []block.NamedRoot{{Name: "adopter", Ref: parent.Ref}}); err != nil {
				t.Fatal(err)
			}
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			queued, err := OpenRefGraph(v.kvtxStore).GetReleases(ctx, block_gc.BlockIRI(leaf.Ref))
			if err != nil || queued != 0 || v.releaseUnderflows.Load() != 0 {
				t.Fatalf("rebuild kept obsolete returns=%d underflows=%d error=%v", queued, v.releaseUnderflows.Load(), err)
			}
			t.Logf("post-commit adoption B=%d L=%d checked=%d", readHolds(t, v, b.Ref), readHolds(t, v, leaf.Ref), v.deadlistsChecked.Load())
			if n := readHolds(t, v, leaf.Ref); n < 1 {
				t.Fatalf("same-generation post-commit adoption left L with %d shared holds, want A and B", n)
			}
		})
	}
}
