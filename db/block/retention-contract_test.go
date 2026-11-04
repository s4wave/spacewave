package block_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	blockmock "github.com/s4wave/spacewave/db/block/mock"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// TestHyperlineReadRetention compares real decoded hits with retained pointer hits.
func TestHyperlineReadRetention(t *testing.T) {
	// Publish a production Okra page to an ordinary block store.
	ctx := t.Context()
	store := blockmock.NewMockStore(0)
	bt, cs, err := okra.BuildTreeWithEntries(ctx, store, nil, nil, func(yield func(okra.BuildEntry) bool) {
		yield(okra.BuildEntry{Key: []byte("key"), ValueIsBlob: true, ValueBlob: blob.NewRawBlob([]byte("body"))})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bt.Write(ctx, true); err != nil {
		t.Fatal(err)
	}
	root, err := block.UnmarshalBlock[okra.Root](ctx, cs)
	if err != nil {
		t.Fatal(err)
	}
	ref := root.GetRootPageRef()

	// Warm the same immutable node before either read acquires its own live hold.
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	tx, source := block.NewTransaction(store, nil, ref, nil)
	tx.SetReadOnly()
	tx.SetDecodedBlockCache(cache)
	reader := block.NewReadCursor(ctx, source, ref)
	if reader == nil {
		t.Fatal("missing immutable reader")
	}
	if _, err := block.UnmarshalReadBlock[okra.Page](ctx, *reader); err != nil {
		t.Fatal(err)
	}
	block.EnableRefHolds()

	// Both forms of a cache-served read must retain the page for Hyperline views.
	for _, retained := range []bool{false, true} {
		name := "ordinary_cursor"
		if retained {
			name = "retained_reader"
		}
		t.Run(name, func(t *testing.T) {
			hold := block.NewRefHold()
			defer hold.Release()
			seen := 0
			block.SetDecodedBlockHitObserver(func(got *block.BlockRef) {
				if got.EqualsRef(ref) {
					seen++
				}
			})
			defer block.SetDecodedBlockHitObserver(nil)
			heldCtx := block.WithRefHold(ctx, hold)
			if retained {
				_, err = block.UnmarshalReadBlock[okra.Page](heldCtx, *reader)
			} else {
				_, err = block.UnmarshalBlock[okra.Page](heldCtx, reader.Cursor())
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("observer=%d hold=%d held=%v", seen, hold.Len(), block.Held(ref.GetHash()))
			if seen != 1 || hold.Len() != 1 || !block.Held(ref.GetHash()) {
				t.Fatal("cache-served read bypassed the eviction observer or live-view hold")
			}
		})
	}
}
