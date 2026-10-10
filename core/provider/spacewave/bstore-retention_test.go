package provider_spacewave

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TestBlockDeletionInvalidatesPublicationProofs checks both deletion entry points
// against the proof store shared by ordinary and scoped block handles.
func TestBlockDeletionInvalidatesPublicationProofs(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "batch"}[batch], func(t *testing.T) {
			// Build a BlockStore whose publication proofs live in a separate cache.
			ctx := t.Context()

			// Wrap a mock block store with a publication proof cache.
			cache := newSyncTestKvStore()
			store := &BlockStore{
				store:     block_store.NewStore("test", block_mock.NewMockStore(0)),
				retention: &blockPublicationRetention{store: cache},
			}

			// Put a block and record a false closure proof for its parent.
			ref, _, err := store.PutBlock(ctx, []byte("retained"), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
				return cache.NewTransaction(ctx, true)
			}, func(ctx context.Context, tx kvtx.Tx) error {
				return tx.Set(ctx, []byte("complete-parent"), []byte{1})
			}); err != nil {
				t.Fatal(err)
			}

			// Delete the block through the batch or single-block entry point.
			if batch {
				_, err = store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
			} else {
				err = store.RmBlock(ctx, ref)
			}
			if err != nil {
				t.Fatal(err)
			}

			// The proof for the deleted block's parent must be gone.
			tx, err := cache.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			if _, found, err := tx.Get(ctx, []byte("complete-parent")); err != nil || found {
				t.Fatalf("deletion retained a false closure proof: found=%v, err=%v", found, err)
			}
		})
	}
}
