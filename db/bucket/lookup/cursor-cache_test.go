package bucket_lookup_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

func TestCursorUnmarshalReusesDecodedBlocksForResourceLifetime(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "resource-cache"})
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		store,
		nil,
		&bucket.ObjectRef{RootRef: ref},
		nil,
		nil,
	)
	defer cursor.Release()

	opCtx, counter := block.WithReadCounter(ctx)
	first, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"

	second, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "resource-cache" {
		t.Fatalf("resource cache clone msg = %q, want resource-cache", got)
	}

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 1 ||
		snapshot.DecodedBlockUnmarshalCount != 1 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 1 ||
		snapshot.DecodedBlockCacheHitCount != 1 ||
		snapshot.DecodedBlockCloneCount != 1 {
		t.Fatalf("unexpected decoded cache counters: %+v", snapshot)
	}
}
