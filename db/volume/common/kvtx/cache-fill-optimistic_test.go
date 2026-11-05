package kvtx

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// TestCacheFillOptimistic runs cache fills on a store with optimistic
// transactions while other writers commit. A fill reading without the writer
// lets another write commit meanwhile; another write appending to the logs
// and counts every write appends to forces no rerun; and a commit that
// removes a block the fill found stored forces a rerun that stores it afresh,
// so the fill never reports a block stored that a later commit removed.
func TestCacheFillOptimistic(t *testing.T) {
	// Open a Volume over a store with optimistic transactions.
	ctx := t.Context()
	db, err := s4db.Open(filepath.Join(t.TempDir(), "fill.s4db"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	v, err := NewVolume(ctx, "test/fill", store_kvkey.NewDefaultKVKey(), db, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	put := func(bucket string, data []byte) *block.BlockRef {
		t.Helper()
		ref, _, err := v.PrepareOwnedBlock(ctx, bucket, data, &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// fill fills data into bucket "fill", pausing its first attempt after
	// the put so between can commit, and reports its attempts.
	fill := func(data []byte, between func()) (attempts int, existed bool) {
		t.Helper()
		paused, resume := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- v.prepareOwned(ctx, v.withDirectFill, bucketOwner("fill"), len(data), claimBucket, func(store block.StoreOps) error {
				attempts++
				var err error
				_, existed, err = store.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true, CacheFill: true})
				if err != nil || attempts != 1 {
					return err
				}
				paused <- struct{}{}
				<-resume
				return nil
			})
		}()
		select {
		case <-paused:
		case err := <-done:
			t.Fatalf("fill ended before its first attempt paused: %v", err)
		}

		// Commit between while the fill reads; a fill holding the writer
		// would block it.
		committed := make(chan struct{})
		go func() {
			between()
			close(committed)
		}()
		select {
		case <-committed:
		case <-time.After(5 * time.Second):
			t.Fatal("a write waited for a fill that was reading")
		}
		close(resume)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		return attempts, existed
	}

	// A write appending to the young log and the counts meanwhile forces no
	// rerun.
	if attempts, _ := fill([]byte("filled alone"), func() { put("other", []byte("written meanwhile")) }); attempts != 1 {
		t.Fatalf("fill beside an unrelated write ran %d attempts, want 1", attempts)
	}

	// A block removed after the fill found it makes the fill run again and
	// store it afresh, tracked as a fill's blocks are, in the unreferenced
	// queue.
	data := []byte("found then removed")
	ref := put("other", data)
	attempts, existed := fill(data, func() {
		err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			if err := rg.RemoveRef(ctx, bucketOwner("other"), block_gc.BlockIRI(ref)); err != nil {
				return err
			}
			return blocks.RmBlock(ctx, ref)
		})
		if err != nil {
			t.Error(err)
		}
	})
	if attempts != 2 || existed {
		t.Fatalf("fill of a removed block ran %d attempts, existed %v; want 2 attempts storing it afresh", attempts, existed)
	}
	got, found, err := v.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("filled block: found %v, data %q, err %v", found, got, err)
	}
	owners, err := v.GetRefGraph().GetIncomingRefs(ctx, block_gc.BlockIRI(ref))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(owners, block_gc.NodeUnreferenced) {
		t.Fatalf("filled block owners %v; want it in the unreferenced queue", owners)
	}
}
