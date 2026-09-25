package block_store_s3

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestDeletePackStore deletes every object of one store and none of another
// store's in the same bucket.
func TestDeletePackStore(t *testing.T) {
	ctx := t.Context()
	bucket, client := newFakeBucket(t, map[string]string{"q/packs/other": "x"})
	store := newTestPackStore(t, client)
	for _, data := range []string{"one", "two"} {
		if _, _, err := store.PutBlock(ctx, []byte(data), nil); err != nil {
			t.Fatal(err)
		}
	}

	if err := DeletePackStore(ctx, client, "bucket", "p/"); err != nil {
		t.Fatal(err)
	}
	if keys := bucket.keys(); !slices.Equal(keys, []string{"q/packs/other"}) {
		t.Fatalf("keys = %v; want only the other store's object", keys)
	}

	// A fresh store finds no blocks, and a rerun deletes nothing more.
	ref, err := block.BuildBlockRef([]byte("one"), nil)
	if err != nil {
		t.Fatal(err)
	}
	exists, err := newTestPackStore(t, client).GetBlockExists(ctx, ref)
	if err != nil || exists {
		t.Fatalf("exists = %v, %v; want the block gone", exists, err)
	}
	if err := DeletePackStore(ctx, client, "bucket", "p/"); err != nil {
		t.Fatal(err)
	}
}
