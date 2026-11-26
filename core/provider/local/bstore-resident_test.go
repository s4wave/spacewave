package provider_local

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestWarmResidentSkipsLocalBlocks reads four listed blocks through
// skipLocal, two of which the local volume holds: the bucket is asked for the
// other two alone, since a placed store's reads serve the first two from the
// volume and would never reach copies the warm-up filled.
func TestWarmResidentSkipsLocalBlocks(t *testing.T) {
	ctx := t.Context()
	newStore := func() block.StoreOps {
		return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0)
	}
	bucket, volume := newStore(), newStore()

	// Put four blocks in the bucket and the first two in the volume too.
	var refs []*block.BlockRef
	for i := range 4 {
		data := []byte{'b', byte(i)}
		ref, _, err := bucket.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if _, _, err := volume.PutBlock(ctx, data, nil); err != nil {
				t.Fatal(err)
			}
		}
		refs = append(refs, ref)
	}

	// Read the list through skipLocal, recording what the bucket is asked for.
	var asked []*block.BlockRef
	read := func(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
		for _, ref := range refs {
			asked = append(asked, ref)
			data, _, err := bucket.GetBlock(ctx, ref)
			if err != nil {
				return err
			}
			if err := fn(ref, &block.StoredBlock{Data: data}); err != nil {
				return err
			}
		}
		return nil
	}
	var skipped, fetched int
	err := skipLocal(volume, read, &skipped)(ctx, refs, func(*block.BlockRef, *block.StoredBlock) error {
		fetched++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only the blocks the volume lacks came from the bucket.
	if skipped != 2 || fetched != 2 || len(asked) != 2 {
		t.Fatalf("skipped %d, fetched %d, asked %d; want 2, 2, 2", skipped, fetched, len(asked))
	}
	for _, ref := range asked {
		if !ref.EqualVT(refs[2]) && !ref.EqualVT(refs[3]) {
			t.Fatalf("the bucket was asked for %s, which the volume holds", ref.MarshalString())
		}
	}
}
