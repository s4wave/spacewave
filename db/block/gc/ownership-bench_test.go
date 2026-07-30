package block_gc_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

func BenchmarkGCStoreOpsDeduplicatedParentBatch(b *testing.B) {
	ctx := context.Background()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	vol, err := common_kvtx.NewVolume(
		ctx,
		"gc-deduplicated-owner-benchmark",
		kvKey,
		store_kvtx_inmem.NewStore(),
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := vol.Close(); err != nil {
			b.Fatal(err)
		}
	})

	const batchSize = 128
	entries := make([]*block.PutBatchEntry, batchSize)
	for i := range entries {
		data := append([]byte("deduplicated parent benchmark block "), []byte(strconv.Itoa(i))...)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: data}
	}
	if err := vol.PutBlockBatch(ctx, entries); err != nil {
		b.Fatal(err)
	}
	store := block_gc.NewGCStoreOpsWithParent(
		vol,
		vol.GetRefGraph(),
		block_gc.BucketIRI("benchmark-parent"),
	)

	b.ResetTimer()
	for range b.N {
		if err := store.PutBlockBatch(ctx, entries); err != nil {
			b.Fatal(err)
		}
		if err := store.FlushPending(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
