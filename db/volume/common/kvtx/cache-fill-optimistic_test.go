package kvtx

import (
	"path/filepath"
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
// lets another write commit meanwhile; an unrelated write forces no rerun,
// even a commit that journals holders while the fill stores a block naming a
// stored child, since a fill reads no liveness bookkeeping; and a commit that
// stores the block a fill found absent forces a rerun that finds it stored,
// so two writers never both give birth to one block, and the block keeps its
// child list.
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

	// fill fills data naming refs, pausing its first attempt after the put
	// so between can commit, and reports its attempts.
	fill := func(data []byte, refs []*block.BlockRef, between func()) (attempts int, existed bool) {
		t.Helper()
		paused, resume := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			entry := &block.PutBatchEntry{Data: data, Refs: refs, RefsKnown: true}
			done <- v.withDirectFill(ctx, func(p *physicalTx) error {
				attempts++
				found, err := block_gc.LandFill(ctx, p.rec, p.blocks, []*block.PutBatchEntry{entry})
				if err != nil || attempts != 1 {
					if err == nil {
						existed = found[0]
					}
					return err
				}
				existed = found[0]
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

	// An unrelated write meanwhile forces no rerun.
	if attempts, _ := fill([]byte("filled alone"), nil, func() { put("other", []byte("written meanwhile")) }); attempts != 1 {
		t.Fatalf("fill beside an unrelated write ran %d attempts, want 1", attempts)
	}

	// Nor does a commit that journals holders while the fill stores a block
	// naming a stored child.
	child := put("other", []byte("stored child"))
	moved := put("other", []byte("moved root"))
	attempts, _ := fill([]byte("filled interior"), []*block.BlockRef{child}, func() {
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: moved}}); err != nil {
			t.Error(err)
		}
	})
	if attempts != 1 {
		t.Fatalf("fill beside a journaling commit ran %d attempts, want 1", attempts)
	}

	// A block another writer stores after the fill found it absent makes the
	// fill run again and find it stored.
	data := []byte("absent then stored")
	var ref *block.BlockRef
	attempts, existed := fill(data, nil, func() { ref = put("other", data) })
	if attempts != 2 || !existed {
		t.Fatalf("fill of a block stored meanwhile ran %d attempts, existed %v; want 2 attempts finding it stored", attempts, existed)
	}
	got, found, err := v.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("filled block: found %v, data %q, err %v", found, got, err)
	}
	listed, err := v.HasChildLists(ctx, []*block.BlockRef{ref})
	if err != nil || !listed[0] {
		t.Fatalf("filled block listed %v err %v; want its child list kept", listed, err)
	}
}
