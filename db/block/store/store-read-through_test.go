package block_store_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/net/hash"
)

func newReadThroughTestBlockStore() *block_store_kvtx.KVTxBlock {
	return block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}

func TestStoreReadThroughWritebackUsesWritablePrimary(t *testing.T) {
	ctx := context.Background()
	primary := newReadThroughTestBlockStore()
	lower := newReadThroughTestBlockStore()
	data := []byte("read-through writeback")
	ref, _, err := lower.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, found, err := primary.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("primary before read found=%v err=%v, want absent", found, err)
	}

	var lowerSource block.StoreOps = lower
	store := block_store.NewStoreReadThrough(
		func() block.StoreOps { return primary },
		func() block.StoreOps { return lowerSource },
		true,
	)
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := scoped.GetBlock(ctx, ref)
	release()
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("scoped lower read = %q/%v/%v", got, found, err)
	}

	lowerSource = nil
	got, found, err = primary.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("primary writeback = %q/%v/%v", got, found, err)
	}
}
