package kvtx

import (
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// stageBuild writes a child and a root referencing it through a new stage.
func stageBuild(t *testing.T, v *Volume, name string) (string, func(), *block.BlockRef, *block.BlockRef) {
	// Open the stage.
	t.Helper()
	ctx := t.Context()
	stage, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write the child, then the root that adopts it.
	child, _, err := v.PrepareStagedBlock(ctx, stage, []byte(name+" child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := v.PrepareStagedBlock(ctx, stage, []byte(name+" root"), &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err)
	}
	return stage, release, root, child
}

// collectBlocks sweeps unowned blocks and checks which of refs remain.
func collectBlocks(t *testing.T, v *Volume, want bool, refs ...*block.BlockRef) {
	// Reap released owners and sweep what they held.
	t.Helper()
	ctx := t.Context()
	if err := v.ReapRootPins(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}

	// Check which blocks survived.
	for _, ref := range refs {
		if found, err := v.GetBlockExists(ctx, ref); err != nil || found != want {
			t.Fatalf("block %s want=%v found=%v err=%v", ref.MarshalString(), want, found, err)
		}
	}
}

func TestStageOwnsAndReleasesUnadoptedBuild(t *testing.T) {
	// Build a root and child through a stage.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	stage, release, root, child := stageBuild(t, v, "unadopted")

	// The stage owns the root, the root owns the child, and no bucket does.
	for ref, owner := range map[*block.BlockRef]string{root: stage, child: block_gc.BlockIRI(root)} {
		owners, err := v.GetRefGraph().GetIncomingRefs(ctx, block_gc.BlockIRI(ref))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(owners, []string{owner}) {
			t.Fatalf("owners of %s: %v, want %s", ref.MarshalString(), owners, owner)
		}
	}

	// The open stage retains the build.
	collectBlocks(t, v, true, root, child)

	// Releasing the stage lets the sweep collect the build.
	release()
	release()
	collectBlocks(t, v, false, root, child)

	// A released stage cannot own new writes.
	if _, _, err := v.PrepareStagedBlock(ctx, stage, []byte("late"), nil); !errors.Is(err, block.ErrStageReleased) {
		t.Fatalf("write after release: %v", err)
	}
}

func TestStageReleaseKeepsAdoptedBuild(t *testing.T) {
	// Build through a stage and adopt the root from a named bucket root.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	_, release, root, child := stageBuild(t, v, "adopted")
	parent, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("parent"), &block.PutOpts{Refs: []*block.BlockRef{root}})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "bucket", "head", parent); err != nil {
		t.Fatal(err)
	}

	// The release leaves the adopted build to its parent.
	release()
	collectBlocks(t, v, true, parent, root, child)
}

func TestStageReleaseRootsKeepsStageOpen(t *testing.T) {
	// Build two roots through one stage.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	stage, release, superseded, supersededChild := stageBuild(t, v, "superseded")
	defer release()
	adopted, _, err := v.PrepareStagedBlock(ctx, stage, []byte("adopted root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Adopt the second root from a named bucket root.
	parent, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("parent"), &block.PutOpts{Refs: []*block.BlockRef{adopted}})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "bucket", "head", parent); err != nil {
		t.Fatal(err)
	}

	// Releasing both roots keeps the adopted one and collects the other.
	if err := v.ReleaseStageRoots(ctx, stage, []*block.BlockRef{superseded, adopted}); err != nil {
		t.Fatal(err)
	}
	collectBlocks(t, v, true, parent, adopted)
	collectBlocks(t, v, false, superseded, supersededChild)

	// The stage still owns new writes.
	if _, _, err := v.PrepareStagedBlock(ctx, stage, []byte("next build"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestStageReleasedByCloseAndCrash(t *testing.T) {
	// Close the process owner and verify its stage's build is collected.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	_, _, root, child := stageBuild(t, v, "closed")
	if err := v.closeRootPins(); err != nil {
		t.Fatal(err)
	}
	collectBlocks(t, v, false, root, child)

	// Simulate a crash: the owner's lease ends while its edges remain.
	v.rootPinsClosed = false
	_, _, root, child = stageBuild(t, v, "crashed")
	if err := v.rootPinLease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	v.rootPinLease = nil
	v.rootPinsClosed = true
	collectBlocks(t, v, false, root, child)
}
