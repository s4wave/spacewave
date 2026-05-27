package volume_controller

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
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

func TestBucketHandleSkipsGCTrackingWhenGCDisabled(t *testing.T) {
	handle := buildBucketHandleForGCConfig(t, &Config{GcIntervalDur: "0"})
	if handle.gcOps != nil {
		t.Fatal("expected gcOps to be nil when GC is disabled")
	}
}

func TestBucketHandleUsesGCTrackingWhenGCEnabled(t *testing.T) {
	handle := buildBucketHandleForGCConfig(t, &Config{GcIntervalDur: "1m"})
	if handle.gcOps == nil {
		t.Fatal("expected gcOps when GC is enabled")
	}
}

func buildBucketHandleForGCConfig(t *testing.T, conf *Config) *bucketHandle {
	t.Helper()

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
	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}
	})

	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: "test", Rev: 1}); err != nil {
		t.Fatal(err)
	}

	c := &Controller{
		config: conf,
		volume: ccontainer.NewCContainer[*volumeCtxPair](nil),
	}
	c.volume.SetValue(&volumeCtxPair{
		vol: &bucketHandleGCTestVolume{
			Volume: vol,
			rg:     stubCollectorGraph{},
		},
		ctx: ctx,
	})

	tracker := &bucketHandleTracker{
		c:         c,
		bucketID:  "test",
		handleCtr: ccontainer.NewCContainer[*bucketHandle](nil),
	}
	if err := tracker.execute(ctx); err != nil {
		t.Fatal(err)
	}

	handle, err := tracker.handleCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handle
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

type bucketHandleGCTestVolume struct {
	*common_kvtx.Volume
	rg block_gc.RefGraphOps
}

func (v *bucketHandleGCTestVolume) GetRefGraph() block_gc.RefGraphOps {
	return v.rg
}

var _ volume.Volume = (*bucketHandleGCTestVolume)(nil)
