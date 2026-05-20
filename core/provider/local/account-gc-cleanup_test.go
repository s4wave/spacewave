//go:build !goscript

package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

func TestLocalGCCleanupRunnerCoalescesTriggerDuringSweep(t *testing.T) {
	acc := &ProviderAccount{
		le: logrus.New().WithField("test", t.Name()),
	}

	var calls atomic.Int32
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	acc.gcCleanupCollect = func(ctx context.Context) (*block_gc.Stats, error) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			case <-firstRelease:
			}
		case 2:
			close(secondStarted)
		}
		return &block_gc.Stats{}, nil
	}
	acc.gcCleanupRunner = acc.newGCCleanupRunner()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- acc.runGCCleanup(ctx)
	}()

	acc.triggerGCCleanup()
	<-firstStarted
	acc.triggerGCCleanup()
	close(firstRelease)
	<-secondStarted
	if err := acc.WaitGCCleanup(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two serialized cleanup sweeps, got %d", got)
	}
	if got := acc.gcCleanupRunner.CompletedGeneration(); got != 2 {
		t.Fatalf("expected completed generation 2, got %d", got)
	}
}

func TestRemoveSharedObjectGCRefsRemovesLocalBucketRoots(t *testing.T) {
	ctx := context.Background()
	vol := newGCCleanupTestVolume(t)
	rg := vol.GetRefGraph()
	providerID := "provider-1"
	accountID := "account-1"
	blockStoreID := SobjectBlockStoreID("so-1")
	bucketID := BlockStoreBucketID(providerID, accountID, blockStoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)
	providerIRI := ProviderIRI(providerID)
	gcOps := block_gc.NewGCStoreOps(vol, rg)
	if err := gcOps.AddGCRef(ctx, block_gc.NodeGCRoot, bucketIRI); err != nil {
		t.Fatalf("add root ref: %v", err)
	}
	if err := gcOps.AddGCRef(ctx, providerIRI, bucketIRI); err != nil {
		t.Fatalf("add provider ref: %v", err)
	}

	acc := &ProviderAccount{
		le:  logrus.New().WithField("test", t.Name()),
		vol: vol,
	}
	acc.removeSharedObjectGCRefs(ctx, providerID, bucketID, acc.le)

	rootRefs, err := rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		t.Fatalf("get root refs: %v", err)
	}
	if len(rootRefs) != 0 {
		t.Fatalf("expected root ref removed, got %v", rootRefs)
	}
	providerRefs, err := rg.GetOutgoingRefs(ctx, providerIRI)
	if err != nil {
		t.Fatalf("get provider refs: %v", err)
	}
	if len(providerRefs) != 0 {
		t.Fatalf("expected provider ref removed, got %v", providerRefs)
	}
	unreferenced, err := rg.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced)
	if err != nil {
		t.Fatalf("get unreferenced refs: %v", err)
	}
	if len(unreferenced) != 1 || unreferenced[0] != bucketIRI {
		t.Fatalf("expected bucket marked unreferenced, got %v", unreferenced)
	}
}

func newGCCleanupTestVolume(t *testing.T) *common_kvtx.Volume {
	t.Helper()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("new kv key: %v", err)
	}
	vol, err := common_kvtx.NewVolume(
		context.Background(),
		"alpha/gc-cleanup-test",
		kvKey,
		store_kvtx_inmem.NewStore(),
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("new volume: %v", err)
	}
	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Fatalf("close volume: %v", err)
		}
	})
	return vol
}
