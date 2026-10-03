package kvtx_block_iavl

import (
	"context"
	"math/rand"
	"testing"
)

// TestDeletePreservesRemainingKeys checks lookup and iteration after each
// deletion from a persisted tree, including deletion of subtree minima.
func TestDeletePreservesRemainingKeys(t *testing.T) {
	// Open a persisted tree and track the keys removed from it.
	ctx := context.Background()
	tree := buildBenchTree(t, 128)
	tx := newBenchReadTx(t, ctx, tree)
	defer tx.Discard()
	removed := make(map[string]bool)

	// Delete the tree keys in a reproducible random order.
	for _, index := range rand.New(rand.NewSource(4)).Perm(len(tree.keys)) {
		// Remove the selected key and record its deletion.
		key := tree.keys[index]
		if err := tx.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
		removed[string(key)] = true

		// Verify each candidate lookup reflects the recorded deletions.
		for _, candidate := range tree.keys {
			_, found, err := tx.Get(ctx, candidate)
			if err != nil || found == removed[string(candidate)] {
				t.Fatalf("after deleting %q, lookup %q: found=%v removed=%v error=%v", key, candidate, found, removed[string(candidate)], err)
			}
		}

		// Verify iteration visits exactly the surviving keys.
		iter := tx.Iterate(ctx, nil, true, false)
		count := 0
		for iter.Next() {
			if removed[string(iter.Key())] {
				t.Fatalf("deleted key still iterates: %q", iter.Key())
			}
			count++
		}
		if err := iter.Err(); err != nil {
			t.Fatal(err)
		}
		iter.Close()
		if count != len(tree.keys)-len(removed) {
			t.Fatalf("iteration count=%d", count)
		}
	}
}
