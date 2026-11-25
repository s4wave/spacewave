package provider_local

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// TestWarmResidentSkipsLocalBlocks lists four blocks, two of which the local
// volume holds, and warms the fill cache through skipLocal: the bucket is
// asked for the other two alone, since a placed store's reads serve the first
// two from the volume and would never reach their cached copies.
func TestWarmResidentSkipsLocalBlocks(t *testing.T) {
	ctx := t.Context()
	newStore := func() block.StoreOps {
		return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	}
	bucket, volume := newStore(), newStore()

	// Put four blocks in the bucket and the first two in the volume too.
	var refs []*block.BlockRef
	var keys []string
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
		key, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		refs, keys = append(refs, ref), append(keys, string(key))
	}

	// Save the list of all four.
	cache, err := block_store_fillcache.Open(ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "blocks.db"), block_store_fillcache.Options{LimitBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if err := cache.SaveResident(ctx, "scope", slices.Values(keys)); err != nil {
		t.Fatal(err)
	}

	// Warm through skipLocal, recording what the bucket is asked for.
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
	var skipped int
	counts, err := cache.WarmResident(ctx, "scope", skipLocal(volume, read, &skipped))
	if err != nil {
		t.Fatal(err)
	}

	// Only the blocks the volume lacks came from the bucket.
	if skipped != 2 || counts.Fetched != 2 || len(asked) != 2 {
		t.Fatalf("skipped %d, fetched %d, asked %d; want 2, 2, 2", skipped, counts.Fetched, len(asked))
	}
	for _, ref := range asked {
		if !ref.EqualVT(refs[2]) && !ref.EqualVT(refs[3]) {
			t.Fatalf("the bucket was asked for %s, which the volume holds", ref.MarshalString())
		}
	}
}
