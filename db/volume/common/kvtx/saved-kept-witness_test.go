package kvtx

import (
	"testing"

	world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// TestSavedKeptOutsideChildDrainsWrapperReturn checks the Volume boundary of a
// pages-mode save whose candidate overflow is empty. The new kept pages are
// ordinary entries; the replaced replay wrapper's return precedes GC fates.
func TestSavedKeptOutsideChildDrainsWrapperReturn(t *testing.T) {
	for _, fixture := range []struct{ recordBeforeReturns, sharedWrapper, sharedHeader bool }{{true, false, false}, {false, false, false}, {true, true, false}, {false, true, false}, {true, false, true}, {false, false, true}, {true, true, true}, {false, true, true}} {
		recordBeforeReturns := fixture.recordBeforeReturns
		name := "record-after-returns"
		if recordBeforeReturns {
			name = "record-before-returns"
		}
		if fixture.sharedWrapper {
			name += "/shared-old-wrapper"
		}
		if fixture.sharedHeader {
			name += "/shared-record-header"
		}
		t.Run(name, func(t *testing.T) {
			// Build real content-addressed World pages and actual replay-span bytes.
			ctx := t.Context()
			v, raw := newPublicationTestVolume(t)
			scratch := block_mock.NewMockStore(hash.RecommendedHashType)
			put := func(value block.Block) *block.BlockRef {
				t.Helper()
				ref, _, err := block.PutBlock(ctx, scratch, value)
				if err != nil {
					t.Fatal(err)
				}
				return ref
			}

			// Build two physical parents of the same stored leaf.
			leaf := put(block_mock.NewExample("saved-kept L"))
			a := put(&block_mock.Pair{Left: leaf, Msg: "A"})
			f := put(&block_mock.Pair{Left: a, Msg: "F"})
			b := put(&block_mock.Pair{Left: leaf, Msg: "B"})
			g := put(&block_mock.Pair{Left: b, Msg: "G"})

			// Encode the actual ordered replay wrapper payloads.
			wrapper := func(roots ...*block.BlockRef) *block.PutBatchEntry {
				// Serialize and address this immutable root tuple.
				t.Helper()
				data, err := (&world_engine.ReplaySpan{Worlds: roots}).MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				return &block.PutBatchEntry{Ref: ref, Data: data, Refs: roots, RefsKnown: true, HoldsRoots: true}
			}
			old, next := wrapper(f), wrapper(f, g)

			// Seed the complete first lineage and its current replay-base wrapper.
			seed := publicationFor(t, "saved-kept", "", "F")
			seed.BucketID = "bucket"
			seed.Entries = []*block.PutBatchEntry{scratchEntry(t, scratch, leaf), scratchEntry(t, scratch, a, leaf), scratchEntry(t, scratch, f, a), old}
			seed.Roots = []block.NamedRoot{{Name: "replay-base", Ref: old.Ref}}
			first := submitPublication(t, v, seed)
			if err := awaitPublication(t, first); err != nil || !first.Counting() {
				t.Fatalf("first lineage: counting=%v error=%v", first.Counting(), err)
			}

			// A second engine can name the identical content-addressed wrapper
			// in another bucket without adding a second physical child list.
			if fixture.sharedWrapper {
				if err := v.SetBucketRoots(ctx, "other-engine", nil, []block.NamedRoot{{Name: "replay-base", Ref: old.Ref}}); err != nil {
					t.Fatal(err)
				}
				if n := readHolds(t, v, old.Ref); n != 1 {
					t.Fatalf("second wrapper owner extra=%d, want1", n)
				}
			}

			// A write fork pins its opening World until the admitted write is
			// discarded; that real extra holder returns with the old wrapper.
			releaseFork, err := v.PinBucketRoot(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseFork()

			// tx.publish marks only candidate overflow. With none, rootHold.worlds
			// carries ordinary G/B, fallback F, and the HoldsRoots wrapper [F,G].
			save := publicationFor(t, "saved-kept", "F", "G")
			save.BucketID = "bucket"
			save.After = first
			save.Entries = []*block.PutBatchEntry{scratchEntry(t, scratch, b, leaf), scratchEntry(t, scratch, g, b), scratchEntry(t, scratch, f, a), next}
			save.Roots = []block.NamedRoot{{Name: "replay-base", Ref: next.Ref}}

			// Wait for the saved graph before returning the fork pin.
			receipt := submitPublication(t, v, save)
			if err := awaitPublication(t, receipt); err != nil {
				t.Fatal(err)
			}

			// Observe the old wrapper return before collector work.
			releaseFork()
			queued, err := OpenRefGraph(v.kvtxStore).GetReleases(ctx, block_gc.BlockIRI(old.Ref))
			if err != nil || queued != 1 {
				t.Fatalf("old wrapper return: queued=%d error=%v", queued, err)
			}
			t.Logf("after save: L=%d F=%d G=%d counting=%v queued-old=%d", readHolds(t, v, leaf), readHolds(t, v, f), readHolds(t, v, g), receipt.Counting(), queued)

			// An already-saved G makes nameWorld return without another page put.
			// The empty receipt set clears steps and emits this actual NetPuts.
			dead := block.NetPuts(nil, nil)
			if dead.RefCount() != 0 || dead.CountsComplete {
				t.Fatalf("empty unknown interval: %+v", dead)
			}
			entries, ref, err := block.EncodeDeadlist(1, nil, g, dead)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				// Keep this durable owner through the restart and final oracle.
				if fixture.sharedHeader {
					if err := v.SetBucketRoots(ctx, "other-record-owner", nil, []block.NamedRoot{{Name: "record-retention"}}); err != nil {
						t.Error(err)
					}
				}
			}()
			nameRecord := func() {
				t.Helper()
				if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: block.DeadlistRootName(1), Ref: ref}}); err != nil {
					t.Fatal(err)
				}
				if fixture.sharedHeader {
					if err := v.SetBucketRoots(ctx, "other-record-owner", nil, []block.NamedRoot{{Name: "record-retention", Ref: ref}}); err != nil {
						t.Fatal(err)
					}
				}
				if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: block.DeadlistDueRootName(1), Ref: ref}, {Name: block.DeadlistRootName(1)}}); err != nil {
					t.Fatal(err)
				}
			}
			if recordBeforeReturns {
				nameRecord()
			}

			// Only the old wrapper's last owner return frees a nonleaf and starts
			// recount. A second bucket keeps the same physical wrapper alive.
			check, err := v.resolveReleases(ctx)
			state, stateErr := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
			if err != nil || stateErr != nil || check == nil {
				t.Fatalf("wrapper return: check=%v state=%+v errors=%v/%v", check, state, err, stateErr)
			}
			freedOld := check.freed.Has(block_gc.BlockIRI(old.Ref))
			if fixture.sharedWrapper {
				if freedOld || !state.Phase.Counting() || state.Doubt {
					t.Fatalf("shared wrapper unexpectedly freed/recounted: freed=%v state=%+v", freedOld, state)
				}
			} else if !freedOld || !state.Phase.Walking() {
				t.Fatalf("last wrapper return did not start recount: freed=%v state=%+v", freedOld, state)
			}

			// Check released nodes against a mark with matching set seeds.
			mark, err := v.GetRefGraph().(*transactionRefGraph).MarkBlocks(ctx, block_gc.MarkLike(check.freed))
			if err != nil {
				t.Fatal(err)
			}
			v.checkReleases(check, mark)
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			if n := readHolds(t, v, leaf); !fixture.sharedWrapper && n != 1 {
				t.Fatalf("after wrapper recount L=%d, want both stored parent edges", n)
			}
			t.Logf("after wrapper returns: shared=%v L=%d state=%+v", fixture.sharedWrapper, readHolds(t, v, leaf), state)
			if !recordBeforeReturns {
				nameRecord()
			}

			// Ordinary block passes retire the small unknown record, consume its
			// returns and finish any erasure-triggered recount on disposable data.
			for range 4 {
				var stats block_gc.Stats
				if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
					t.Fatal(err)
				}

				// Observe the physical cleanup and count generation after this pass.
				passState, err := OpenRefGraph(v.kvtxStore).GetSharing(ctx)
				if err != nil {
					t.Fatal(err)
				}
				generation, err := OpenRefGraph(v.kvtxStore).GetSharingGeneration(ctx)
				if err != nil {
					t.Fatal(err)
				}
				headerPresent, err := v.GetBlockExists(ctx, ref)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("after ordinary pass: shared=%v L=%d checked=%d state=%+v generation=%d header=%v", fixture.sharedWrapper, readHolds(t, v, leaf), v.deadlistsChecked.Load(), passState, generation, headerPresent)
			}

			// Finish rebuilding and verify the retired record disappeared.
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}
			roots, err := v.GetRefGraph().GetOutgoingRefs(ctx, RootOwnerIRI("bucket", block.DeadlistDueRootName(1)))
			if err != nil || len(roots) != 0 || v.deadlistsChecked.Load() == 0 {
				t.Fatalf("small record retirement: roots=%v checked=%d error=%v", roots, v.deadlistsChecked.Load(), err)
			}

			// Require settled count provenance and empty release queues.
			state, err = OpenRefGraph(v.kvtxStore).GetSharing(ctx)
			if err != nil || !state.Phase.Counting() || state.Doubt {
				t.Fatalf("terminal basis incomplete: %+v error=%v", state, err)
			}
			for _, page := range []*block.BlockRef{old.Ref, next.Ref, ref, f, a, leaf, g, b} {
				queued, err := OpenRefGraph(v.kvtxStore).GetReleases(ctx, block_gc.BlockIRI(page))
				if err != nil || queued != 0 {
					t.Fatalf("terminal return remains for %s: queued=%d error=%v", page.MarshalString(), queued, err)
				}
			}
			if v.releaseUnderflows.Load() != 0 || v.releaseLiveBlocks.Load() != 0 || v.deadlistCountUnderflows.Load() != 0 || v.deadlistLivePages.Load() != 0 {
				t.Fatalf("terminal L=%d release under/live=%d/%d deadlist under/live=%d/%d", readHolds(t, v, leaf), v.releaseUnderflows.Load(), v.releaseLiveBlocks.Load(), v.deadlistCountUnderflows.Load(), v.deadlistLivePages.Load())
			}
			t.Logf("after record/drain: L=%d checked=%d", readHolds(t, v, leaf), v.deadlistsChecked.Load())

			// Reopen the real store and re-mark both live parent graphs.
			if err := v.Close(); err != nil {
				t.Fatal(err)
			}
			v, err = NewVolume(ctx, "test/publication", store_kvkey.NewDefaultKVKey(), raw, &store_kvtx.Config{}, false, false, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = v.Close() })
			if err := v.walkSharing(ctx); err != nil {
				t.Fatal(err)
			}

			// Read both live World graphs from the reopened store.
			mark = markBlocks(t, v)
			if mark.Unknown != 0 {
				t.Fatalf("restart graph unknown=%d", mark.Unknown)
			}
			for _, page := range []*block.BlockRef{f, a, leaf, g, b} {
				if !mark.Live.Has(block_gc.BlockIRI(page)) {
					t.Fatalf("live page missing after restart: %s", page.MarshalString())
				}
				if _, found, err := v.GetBlock(ctx, page); err != nil || !found {
					t.Fatalf("live page unreadable after restart: %v/%v", found, err)
				}
			}

			// Check count provenance before the retained-header coverage oracle.
			state, err = OpenRefGraph(v.kvtxStore).GetSharing(ctx)
			if err != nil || !state.Phase.Counting() || state.Doubt {
				t.Fatalf("restart basis incomplete: %+v error=%v", state, err)
			}
			headerPresent, err := v.GetBlockExists(ctx, ref)
			if err != nil || (fixture.sharedHeader && !headerPresent) {
				t.Fatalf("retained header after restart: present=%v error=%v", headerPresent, err)
			}
			t.Logf("after restart/read oracle: sharedWrapper=%v sharedHeader=%v header=%v L=%d state=%+v", fixture.sharedWrapper, fixture.sharedHeader, headerPresent, readHolds(t, v, leaf), state)
			if n := readHolds(t, v, leaf); n != 1 {
				t.Fatalf("complete live-parent coverage: L extra=%d, want1 for stored A->L and B->L", n)
			}
		})
	}
}
