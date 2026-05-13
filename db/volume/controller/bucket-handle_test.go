package volume_controller

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

func TestBucketHandleBeginReadOperationUsesScopedStore(t *testing.T) {
	ctx := context.Background()
	vol, err := common_kvtx.NewVolume(
		ctx,
		"test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	scopedStore := &bucketHandleScopedReadStore{
		data: []byte("scoped"),
	}
	handle := &bucketHandle{
		v: &bucketHandleScopedReadVolume{
			Volume: vol,
			scoped: scopedStore,
		},
		bucketConf: &bucket.Config{Id: "test"},
	}

	scoped, release, err := handle.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	data, found, err := scoped.GetBlock(ctx, &block.BlockRef{})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected scoped store block to be found")
	}
	if !bytes.Equal(data, scopedStore.data) {
		t.Fatalf("GetBlock data = %q, want %q", data, scopedStore.data)
	}
	if scopedStore.getBlockCalls != 1 {
		t.Fatalf("scoped GetBlock calls = %d, want 1", scopedStore.getBlockCalls)
	}
}

type bucketHandleScopedReadVolume struct {
	*common_kvtx.Volume
	scoped block.StoreOps
}

func (v *bucketHandleScopedReadVolume) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return v.scoped, func() {}, nil
}

type bucketHandleScopedReadStore struct {
	block.NopStoreOps
	data          []byte
	getBlockCalls int
}

func (s *bucketHandleScopedReadStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	s.getBlockCalls++
	return s.data, true, nil
}
