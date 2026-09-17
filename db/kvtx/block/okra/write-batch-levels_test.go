package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

func TestWriteBatchLevelsMatchIndependentRebuild(t *testing.T) {
	for _, seed := range []uint64{1, 13, 97} {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			ctx := t.Context()
			store := newOkraTestStore()
			btx, root := block.NewTransaction(store, nil, nil, nil)
			tx, err := NewTxWithInlineValues(ctx, root, nil, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			refTx, refRoot := block.NewTransaction(store, nil, nil, nil)
			rebuilt, err := NewTxWithInlineValues(ctx, refRoot, nil, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer rebuilt.Discard()
			expected := make(map[string][]byte)
			rng := rand.New(rand.NewPCG(seed, seed+5))
			for round := range 40 {
				var changes []kvtx.WriteBatchEntry
				if round == 0 {
					for i := range 2048 {
						changes = append(changes, kvtx.WriteBatchEntry{Key: []byte(fmt.Sprintf("k-%05d", i)), Value: []byte(fmt.Sprintf("v-%05d", i))})
					}
				} else if round%10 == 0 {
					// Delete adjacent page-boundary nodes as well as interior keys. The
					// correct result must include unchanged predecessors once, not twice.
					it := tx.Iterate(ctx, nil, true, false)
					if err := it.Seek(nil); err != nil {
						t.Fatal(err)
					}
					for it.Valid() {
						if rng.IntN(2) == 0 {
							changes = append(changes, kvtx.WriteBatchEntry{Key: bytes.Clone(it.Key()), Delete: true})
						}
						it.Next()
					}
					if err := it.Err(); err != nil {
						t.Fatal(err)
					}
					it.Close()
				} else {
					for i := range 128 {
						changes = append(changes, kvtx.WriteBatchEntry{Key: []byte(fmt.Sprintf("k-%05d", rng.IntN(4096))), Value: bytes.Repeat([]byte{byte(round), byte(i)}, rng.IntN(150)), Delete: rng.IntN(4) == 0})
					}
				}
				for _, e := range changes {
					if e.Delete {
						delete(expected, string(e.Key))
					} else {
						expected[string(e.Key)] = bytes.Clone(e.Value)
					}
				}
				if err := tx.ApplyWriteBatch(ctx, changes); err != nil {
					t.Fatalf("round %d: %v", round, err)
				}
				keys := make([]string, 0, len(expected))
				for k := range expected {
					keys = append(keys, k)
				}
				slices.Sort(keys)
				// ReplaceAll uses the independent bottom-up builder, not the incremental
				// window propagation under test.
				if err := rebuilt.ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
					for _, k := range keys {
						if !yield([]byte(k), expected[k]) {
							return
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				if tx.root.GetHeight() != rebuilt.root.GetHeight() || tx.root.GetSize() != rebuilt.root.GetSize() || !bytes.Equal(tx.root.GetRootHash(), rebuilt.root.GetRootHash()) {
					t.Fatalf("round %d differs from independent builder", round)
				}
				if round%5 == 0 {
					got, _, err := btx.Write(ctx, false)
					if err != nil {
						t.Fatal(err)
					}
					want, _, err := refTx.Write(ctx, false)
					if err != nil {
						t.Fatal(err)
					}
					if !got.EqualsRef(want) {
						t.Fatalf("round %d durable encoding differs", round)
					}
				}
			}
		})
	}
}
