package kvtx_block_iavl

import (
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// TestIteratorSeekAfterExhaustion reuses exhausted public iterators without
// crossing their prefix bounds or fetching unavailable value blocks.
func TestIteratorSeekAfterExhaustion(t *testing.T) {
	// Create real memory block storage and a shared raw value reference.
	ctx := t.Context()
	store := block_store_inmem.NewInmemBlock(kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), hash.HashType_HashType_BLAKE3, true)
	valueRef, _, err := store.PutBlock(ctx, []byte("value"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Persist a tree whose bounded prefix has neighbors in both directions.
	keys := []string{"0", "a/0", "a/1", "a/2", "b/0", "\xff\x00", "\xff\x01"}
	built, _, err := BuildTree(store, nil, nil, func(yield func([]byte, *block.BlockRef) bool) {
		for _, key := range keys {
			if !yield([]byte(key), valueRef) {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := built.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Remove only the raw value so key traversal must remain independent of it.
	if err := store.RmBlock(ctx, valueRef); err != nil {
		t.Fatal(err)
	}
	_, cursor := block.NewTransaction(store, nil, root, nil)
	tx, err := NewTx(ctx, cursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)

	// Cover nil-stack prefix stops and capacity-preserving natural exhaustion.
	for _, tc := range []struct {
		name     string
		prefix   string
		want     []string
		explicit string
		below    string
		above    string
	}{
		{name: "bounded", prefix: "a/", want: keys[1:4], explicit: "a/1", below: "a/", above: "a/\xff"},
		{name: "unbounded", want: keys, explicit: "a/1", below: "\x01", above: "\xff\xff"},
		{name: "high-bytes", prefix: "\xff", want: keys[5:], explicit: "\xff\x00", below: "\xff", above: "\xff\xff"},
		{name: "missing", prefix: "c/", explicit: "c/1", below: "c/", above: "c/\xff"},
	} {
		for _, reverse := range []bool{false, true} {
			// Exercise each prefix with a separate iterator in both directions.
			direction := "forward"
			if reverse {
				direction = "reverse"
			}
			t.Run(tc.name+"/"+direction, func(t *testing.T) {
				// Open the public iterator and consume the selected prefix completely.
				iterator := tx.Iterate(ctx, []byte(tc.prefix), true, reverse)
				t.Cleanup(iterator.Close)
				want := slices.Clone(tc.want)
				if reverse {
					slices.Reverse(want)
				}
				iterator.Next()
				assertIteratorKeys(t, iterator, want)
				if err := iterator.Err(); err != nil {
					t.Fatal(err)
				}

				// Restart the same exhausted iterator at its directional prefix bound.
				if err := iterator.Seek(nil); err != nil {
					t.Fatal(err)
				}
				if iterator.Valid() {
					if _, err := iterator.Value(); !errors.Is(err, block.ErrNotFound) {
						t.Fatalf("unavailable value error = %v, want %v", err, block.ErrNotFound)
					}
				}
				assertIteratorKeys(t, iterator, want)

				// An exact key remains inclusive after the restarted scan also exhausts.
				if err := iterator.Seek([]byte(tc.explicit)); err != nil {
					t.Fatal(err)
				}
				var tail []string
				if index := slices.Index(want, tc.explicit); index >= 0 {
					tail = want[index:]
				}
				assertIteratorKeys(t, iterator, tail)

				// A missing target produces ordinary absence and still permits another reset.
				absent := tc.above
				if reverse {
					absent = tc.below
				}
				if err := iterator.Seek([]byte(absent)); err != nil {
					t.Fatal(err)
				}
				assertIteratorKeys(t, iterator, nil)
				if err := iterator.Seek(nil); err != nil {
					t.Fatal(err)
				}
				assertIteratorKeys(t, iterator, want)
				if err := iterator.Err(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
