package kvtx

import (
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func TestRootRetentionOwnersReadersAndAbandonedPins(t *testing.T) {
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
	if err := v.MarkRootsComplete(ctx, []block.RootProof{{Ref: root, Domain: "objects"}}); err != nil {
		t.Fatal(err)
	}
	if found, err := v.RootComplete(ctx, root, "metadata"); err != nil || found {
		t.Fatalf("proof crossed decoder domains: %v %v", found, err)
	}
	for _, name := range []string{"head", "fork"} {
		if err := v.SetBucketRoot(ctx, "bucket", name, root); err != nil {
			t.Fatal(err)
		}
	}
	collect := func(want bool) {
		t.Helper()
		if err := v.ReapRootPins(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}
		for _, ref := range []*block.BlockRef{root, child} {
			if found, err := v.GetBlockExists(ctx, ref); err != nil || found != want {
				t.Fatalf("retention want=%v found=%v err=%v", want, found, err)
			}
		}
	}
	if err := v.SetBucketRoot(ctx, "bucket", "head", nil); err != nil {
		t.Fatal(err)
	}
	collect(true)
	one, err := v.PinBucketRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := v.PinBucketRoot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "bucket", "fork", nil); err != nil {
		t.Fatal(err)
	}
	one()
	one()
	collect(true)
	two()
	collect(false)
	if found, err := v.RootComplete(ctx, root, "objects"); err != nil || found {
		t.Fatalf("collected root kept a completion proof: %v %v", found, err)
	}

	// Simulate process exit: the lease goes away while its persistent owner
	// remains. The next sweep must distinguish it from a live reader.
	root, _, err = v.PrepareOwnedBlock(ctx, "bucket", []byte("abandoned root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "bucket", "head", root); err != nil {
		t.Fatal(err)
	}
	if _, err := v.PinBucketRoot(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "bucket", "head", nil); err != nil {
		t.Fatal(err)
	}
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
	if found, err := v.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("abandoned root retained: %v %v", found, err)
	}
}

func TestPublicationRootHandoffSurvivesSupersessionAndCrash(t *testing.T) {
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	first := publicationFor(t, "retained-head", "", "first")
	first.RootName, first.Root = "world", first.Entries[0].Ref
	receipt := submitPublication(t, v, first)
	if err := awaitPublication(t, receipt); err != nil {
		t.Fatal(err)
	}
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
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	root, _, err := v.PrepareOwnedBlock(ctx, "deleted-bucket", []byte("retained root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoot(ctx, "deleted-bucket", "head", root); err != nil {
		t.Fatal(err)
	}
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
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	var roots []*block.BlockRef
	for _, data := range []string{"first root", "second root"} {
		root, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
		if err := v.SetBucketRoot(ctx, "bucket", data, root); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoot(ctx, "bucket", data, nil); err != nil {
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

	// A held pin retains its root; the released root is collected.
	release, err := v.PinBucketRoot(ctx, roots[0])
	if err != nil {
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
