package kvtx

import (
	"slices"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func TestRootRetentionOwnersReadersAndAbandonedPins(t *testing.T) {
	// Prepare a retained root with a shared child block.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	child, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("shared child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("root"), &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err)
	}

	// Persist and verify the root completion proof.
	if err := v.MarkRootsComplete(ctx, []*block.BlockRef{root}); err != nil {
		t.Fatal(err)
	}
	if found, err := v.RootComplete(ctx, root); err != nil || !found {
		t.Fatalf("root proof found=%v err=%v", found, err)
	}

	// Retain the root through two named bucket owners.
	for _, name := range []string{"head", "fork"} {
		if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: name, Ref: root}}); err != nil {
			t.Fatal(err)
		}
	}

	// Collect unowned roots and verify the expected root and child retention.
	collect := func(want bool) {
		// Reap abandoned reader pins and collect unowned blocks.
		t.Helper()
		if err := v.ReapRootPins(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}

		// Verify the root and child have the expected retained state.
		for _, ref := range []*block.BlockRef{root, child} {
			if found, err := v.GetBlockExists(ctx, ref); err != nil || found != want {
				t.Fatalf("retention want=%v found=%v err=%v", want, found, err)
			}
		}
	}

	// Remove one named owner and verify the other retains the root.
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head"}}); err != nil {
		t.Fatal(err)
	}
	collect(true)

	// Acquire two reader pins before removing the remaining named owner.
	one, err := v.PinBucketRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := v.PinBucketRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify releasing one reader leaves the other reader retaining the root.
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "fork"}}); err != nil {
		t.Fatal(err)
	}
	one()
	one()
	collect(true)

	// Release the last reader and verify collection removes the root and proof.
	two()
	collect(false)
	if found, err := v.RootComplete(ctx, root); err != nil || found {
		t.Fatalf("collected root kept a completion proof: %v %v", found, err)
	}

	// Simulate process exit: the lease goes away while its persistent owner
	// remains. The next sweep must distinguish it from a live reader.
	root, _, err = v.PrepareOwnedBlock(ctx, "bucket", []byte("abandoned root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.PinBucketRoot(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Drop the named root and simulate loss of the reader process lease.
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head"}}); err != nil {
		t.Fatal(err)
	}
	if err := v.rootPinLease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	v.rootPinLease = nil
	v.rootPinsClosed = true

	// Reap the abandoned reader pin and verify its root is collected.
	if err := v.ReapRootPins(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := v.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("abandoned root retained: %v %v", found, err)
	}
}

func TestPublicationRootHandoffSurvivesSupersessionAndCrash(t *testing.T) {
	// Publish a retained root and keep its consumer receipt.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	first := publicationFor(t, "retained-head", "", "first")
	first.RootName, first.Root = "world", first.Entries[0].Ref
	receipt := submitPublication(t, v, first)
	if err := awaitPublication(t, receipt); err != nil {
		t.Fatal(err)
	}

	// Publish a superseding root and verify the first consumer still retains its root.
	second := publicationFor(t, "retained-head", "first", "second")
	second.RootName, second.Root = "world", second.Entries[0].Ref
	if err := v.PublishAtomic(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := v.GetBlockExists(ctx, first.Root); err != nil || !found {
		t.Fatalf("publication handoff lost root: %v %v", found, err)
	}

	// The first consumer never completed its handoff. Process death must not
	// leave permanent bucket staging attached to that superseded revision.
	if err := v.rootPinLease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	v.rootPinLease = nil
	v.rootPinsClosed = true
	if err := v.ReapRootPins(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := v.GetBlockExists(ctx, first.Root); err != nil || found {
		t.Fatalf("abandoned publication retained: %v %v", found, err)
	}
	if found, err := v.GetBlockExists(ctx, second.Root); err != nil || !found {
		t.Fatalf("durable head lost after crash: %v %v", found, err)
	}
	receipt.Release()
}

func TestRootRetentionFollowsBucketDeletion(t *testing.T) {
	// Prepare and retain a named root under the bucket.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	root, _, err := v.PrepareOwnedBlock(ctx, "deleted-bucket", []byte("retained root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "deleted-bucket", nil, []block.NamedRoot{{Name: "head", Ref: root}}); err != nil {
		t.Fatal(err)
	}

	// Delete the bucket ownership and verify its named root is collected.
	if err := v.GetRefGraph().ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{Subject: block_gc.NodeGCRoot, Object: block_gc.BucketIRI("deleted-bucket")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := v.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("deleted bucket kept a named root: %v %v", found, err)
	}
}

func TestRootRetentionConcurrentPins(t *testing.T) {
	// Prepare two roots and remove their named bucket owners.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	var roots []*block.BlockRef
	for _, data := range []string{"first root", "second root"} {
		root, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
		if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: data, Ref: root}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: data}}); err != nil {
			t.Fatal(err)
		}
	}

	// Churn pins so adds, removals, and waits on in-flight edge writes overlap.
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 64 {
		wg.Go(func() {
			for range 8 {
				release, err := v.PinBucketRoot(ctx, roots[i%len(roots)])
				if err != nil {
					errs <- err
					return
				}
				release()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	// A held pin retains its root; the released root is collected once its
	// deferred edge removal is written.
	release, err := v.PinBucketRoot(ctx, roots[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ReapRootPins(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		if found, err := v.GetBlockExists(ctx, roots[i]); err != nil || found != want {
			t.Fatalf("root %d: want=%v found=%v err=%v", i, want, found, err)
		}
	}
	release()
}

// TestReleaseBucketRootsKeepsReferencedRoots releases staged roots: a root no
// one else holds is collected, a root another block references survives, and
// a named root survives.
func TestReleaseBucketRootsKeepsReferencedRoots(t *testing.T) {
	// Stage a lone root, a root referenced by a staged parent, and a named root.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	prepare := func(data string, refs ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		ref, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte(data), &block.PutOpts{Refs: refs})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	lone := prepare("lone payload")
	spliced := prepare("spliced payload")
	prepare("file range", spliced)
	named := prepare("named payload")
	if err := v.SetBucketRoots(ctx, "bucket", nil, []block.NamedRoot{{Name: "head", Ref: named}}); err != nil {
		t.Fatal(err)
	}

	// Release all three and collect.
	if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{lone, spliced, named}); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}

	// Only the lone root is gone.
	for _, check := range []struct {
		ref  *block.BlockRef
		want bool
	}{{lone, false}, {spliced, true}, {named, true}} {
		if found, err := v.GetBlockExists(ctx, check.ref); err != nil || found != check.want {
			t.Fatalf("block %s found=%v err=%v, want %v", check.ref.MarshalString(), found, err, check.want)
		}
	}
}

// TestRootRetentionDefersEditsToNextCommit checks that releasing a reader pin,
// marking a root complete and repeating a preparation commit nothing of their
// own, and that the next changing direct transaction makes the released pin
// and the proof durable in its one commit.
func TestRootRetentionDefersEditsToNextCommit(t *testing.T) {
	// Prepare and pin a root, then note the physical commit count.
	v, raw := newPublicationTestVolume(t)
	ctx := t.Context()
	root, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("deferred root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := v.PinBucketRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	commits := func() int {
		raw.mu.Lock()
		defer raw.mu.Unlock()
		return raw.commits
	}
	before := commits()

	// Release the pin, mark the root complete and prepare the root again.
	release()
	if err := v.MarkRootsComplete(ctx, []*block.BlockRef{root}); err != nil {
		t.Fatal(err)
	}
	if found, err := v.RootComplete(ctx, root); err != nil || !found {
		t.Fatalf("pending root proof found=%v err=%v", found, err)
	}
	if _, existed, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("deferred root"), nil); err != nil || !existed {
		t.Fatalf("repeated preparation existed=%v err=%v", existed, err)
	}
	if n := commits() - before; n != 0 {
		t.Fatalf("unchanged and deferred edits made %d commits", n)
	}

	// Prepare another block and verify its commit carries both edits.
	if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("next write"), nil); err != nil {
		t.Fatal(err)
	}
	if n := commits() - before; n != 1 {
		t.Fatalf("next write made %d commits", n)
	}
	node := block_gc.BlockIRI(root)
	proofs, err := v.GetRefGraph().GetOutgoingRefs(ctx, node)
	if err != nil || !slices.Equal(proofs, []string{completeWorldNode}) {
		t.Fatalf("root proof edges=%v err=%v", proofs, err)
	}
	owners, err := v.GetRefGraph().GetIncomingRefs(ctx, node)
	if err != nil || !slices.Equal(owners, []string{block_gc.BucketIRI("bucket")}) {
		t.Fatalf("root owners=%v err=%v", owners, err)
	}
}

// TestRootRetentionSweepDropsPendingProof checks that sweeping a root also
// drops its proof before any commit carried it.
func TestRootRetentionSweepDropsPendingProof(t *testing.T) {
	// Orphan a prepared root, then mark it complete.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	root, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("swept root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{root}); err != nil {
		t.Fatal(err)
	}
	if err := v.MarkRootsComplete(ctx, []*block.BlockRef{root}); err != nil {
		t.Fatal(err)
	}

	// Sweep the root and verify neither block nor proof remains.
	removed, err := v.SweepUnreferenced(ctx, v.GetRefGraph(), []string{block_gc.BlockIRI(root)})
	if err != nil || len(removed) != 1 {
		t.Fatalf("sweep removed=%v err=%v", removed, err)
	}
	if found, err := v.RootComplete(ctx, root); err != nil || found {
		t.Fatalf("swept root kept a proof: %v %v", found, err)
	}
	if found, err := v.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("swept root kept its block: %v %v", found, err)
	}
}

// TestSetBucketRootsHoldsWrittenEntries checks that named roots hold blocks
// written by the same call, after their staging ownership is released.
func TestSetBucketRootsHoldsWrittenEntries(t *testing.T) {
	// Prepare a child the bucket stages.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	child, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write a root referencing it and hold it under two names.
	data := []byte("root")
	root, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: root, Data: data, Refs: []*block.BlockRef{child}}}
	roots := []block.NamedRoot{{Name: "head", Ref: root}, {Name: "span", Ref: root}}
	if err := v.SetBucketRoots(ctx, "bucket", entries, roots); err != nil {
		t.Fatal(err)
	}

	// Collect after releasing staging ownership and check the retained state.
	collect := func(want bool) {
		t.Helper()
		if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}
		for _, ref := range []*block.BlockRef{root, child} {
			if found, err := v.GetBlockExists(ctx, ref); err != nil || found != want {
				t.Fatalf("retention want=%v found=%v err=%v", want, found, err)
			}
		}
	}
	if err := v.ReleaseBucketRoots(ctx, "bucket", []*block.BlockRef{root, child}); err != nil {
		t.Fatal(err)
	}
	collect(true)

	// Release both names in one call and check the graph is collected.
	release := []block.NamedRoot{{Name: "head"}, {Name: "span"}}
	if err := v.SetBucketRoots(ctx, "bucket", nil, release); err != nil {
		t.Fatal(err)
	}
	collect(false)
}
