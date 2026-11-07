package block_store_s3

import (
	"context"
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestMergePackIndex merges an index into one that already lists a pack of it
// unindexed and holds a pack of its own, then merges again.
func TestMergePackIndex(t *testing.T) {
	eachIndexVolume(t, testMergePackIndex)
}

func testMergePackIndex(t *testing.T, kind string) {
	ctx := t.Context()
	newIndex := func() *packIndex {
		return newPackIndex(newIndexKV(t, kind))
	}
	keysOf := func(id string) [][]byte {
		var keys [][]byte
		for i := range 3000 {
			keys = append(keys, []byte(fmt.Sprintf("block-key-pad-%s-%010d", id, i)))
		}
		return keys
	}
	entry := func(id string) *packfile.PackfileEntry {
		return &packfile.PackfileEntry{Id: id}
	}

	// The source indexed packs a, b and c, and lists d unindexed.
	src := newIndex()
	for _, id := range []string{"a", "b", "c"} {
		if err := src.addWritten(ctx, entry(id), keysOf(id)); err != nil {
			t.Fatal(err)
		}
	}
	src.buffered = append(src.buffered, entry("d"))
	if err := src.flush(ctx); err != nil {
		t.Fatal(err)
	}

	// The destination indexed its own pack z and lists b unindexed.
	dst := newIndex()
	if err := dst.addWritten(ctx, entry("z"), keysOf("z")); err != nil {
		t.Fatal(err)
	}
	dst.buffered = append(dst.buffered, entry("b"))
	if err := dst.flush(ctx); err != nil {
		t.Fatal(err)
	}

	// Merge, then reload the destination as its pack store would.
	read := func(ctx context.Context, fn func(tx kvtx.Tx) error) error {
		tx, err := src.store.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer tx.Discard()
		return fn(tx)
	}
	p, err := MergePackIndex(ctx, dst.store, read, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Packs != 3 || p.Keys != 9000 {
		t.Fatalf("merged %d packs and %d keys, want 3 and 9000", p.Packs, p.Keys)
	}
	got := newPackIndex(dst.store)
	if _, _, err := got.load(ctx); err != nil {
		t.Fatal(err)
	}

	// Every pack appears once, b keeps its number, and d stays unindexed.
	if len(got.rows) != 5 || len(got.ids) != 5 {
		t.Fatalf("%d packs in %d numbers, want 5 in 5", len(got.rows), len(got.ids))
	}
	if got.rows["b"].num != dst.rows["b"].num {
		t.Fatalf("b moved from %d to %d", dst.rows["b"].num, got.rows["b"].num)
	}
	for id, indexed := range map[string]bool{"a": true, "b": true, "c": true, "d": false, "z": true} {
		if got.Indexed(id) != indexed {
			t.Fatalf("pack %s indexed %v, want %v", id, got.Indexed(id), indexed)
		}
	}

	// Every key locates in its own pack under the destination's numbers.
	for _, id := range []string{"a", "b", "c", "z"} {
		for _, key := range keysOf(id) {
			at, ok, err := got.Locate(ctx, key)
			if err != nil || !ok || at != id {
				t.Fatalf("key %s located in %q, %v, %v", key, at, ok, err)
			}
		}
	}

	// A second merge finds nothing to add.
	p, err = MergePackIndex(ctx, dst.store, read, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Packs != 0 || p.Keys != 0 {
		t.Fatalf("second merge added %d packs and %d keys", p.Packs, p.Keys)
	}
}
