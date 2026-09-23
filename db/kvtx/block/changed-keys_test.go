package kvtx_block

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestChangedKeys checks inserts, removals, value edits, tree rotations, and bounded fallback.
func TestChangedKeys(t *testing.T) {
	for _, impl := range []KVImplType{KVImplType_KV_IMPL_TYPE_IAVL, KVImplType_KV_IMPL_TYPE_OKRA, KVImplType_KV_IMPL_TYPE_OKRA_INLINE} {
		t.Run(impl.String(), func(t *testing.T) {
			ctx := t.Context()
			store := newSelectorOkraStore()
			var root *block.BlockRef
			values := make(map[string]string)
			rng := rand.New(rand.NewPCG(4, 9))
			for round := range 12 {
				previous := root
				btx, cursor := block.NewTransaction(store, nil, root, nil)
				if root == nil {
					cursor.SetBlock(NewKeyValueStore(impl), true)
				}
				writer, err := BuildKvTransaction(ctx, cursor, true)
				if err != nil {
					t.Fatal(err)
				}
				changed := make(map[string]bool)
				count := 1 + rng.IntN(10)
				if round == 0 {
					count = 1024
				}
				for i := 0; i < count; i++ {
					key := fmt.Sprintf("key/%04d", rng.IntN(1500))
					if round == 0 {
						key = fmt.Sprintf("key/%04d", i)
					}
					if round != 0 && rng.IntN(3) == 0 {
						if _, exists := values[key]; exists {
							changed[key] = true
						}
						delete(values, key)
						err = writer.Delete(ctx, []byte(key))
					} else {
						value := fmt.Sprintf("round-%d-value-%d", round, i)
						values[key] = value
						changed[key] = true
						err = writer.Set(ctx, []byte(key), []byte(value))
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := writer.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				writer.Discard()
				root, _, err = btx.Write(ctx, true)
				if err != nil {
					t.Fatal(err)
				}
				if previous == nil {
					continue
				}

				_, before := block.NewTransaction(store, nil, previous, nil)
				_, after := block.NewTransaction(store, nil, root, nil)
				readCtx, counter := block.WithReadCounter(ctx)
				keys, complete, err := ChangedKeys(readCtx, before, after, 10000)
				if err != nil || !complete {
					t.Fatalf("compare: complete=%t err=%v", complete, err)
				}
				var got, want []string
				for _, key := range keys {
					got = append(got, string(key))
				}
				for key := range changed {
					want = append(want, key)
				}
				slices.Sort(want)
				if !slices.Equal(got, want) {
					t.Fatalf("round %d: keys=%v want=%v", round, got, want)
				}
				t.Logf("round %d: changed=%d block reads=%d", round, len(keys), counter.Snapshot().BlockReadCount)
				if len(want) > 1 {
					if _, complete, err := ChangedKeys(ctx, before, after, 1); err != nil || complete {
						t.Fatalf("limit: complete=%t err=%v", complete, err)
					}
				}
			}
		})
	}
}
