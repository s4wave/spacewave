//go:build !goscript

package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/routine"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

func TestGCCleanupRunnerCoalescesTriggerDuringSweep(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")

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
	if err := acc.gcCleanupRunner.Wait(context.Background()); err != nil {
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

func TestDeleteSharedObjectTriggersGCCleanup(t *testing.T) {
	const soID = "so-cleanup"
	srv := newDeleteSharedObjectTestServer(t, soID)
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- acc.runGCCleanup(ctx)
	}()

	if err := acc.DeleteSharedObject(context.Background(), soID); err != nil {
		t.Fatalf("delete shared object: %v", err)
	}
	if err := acc.gcCleanupRunner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := acc.gcCleanupRunner.CompletedGeneration(); got != 1 {
		t.Fatalf("expected completed generation 1, got %d", got)
	}
}

func TestHandleAccountSONotifyDeleteTriggersGCCleanup(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- acc.runGCCleanup(ctx)
	}()

	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "delete",
	})
	if err := acc.gcCleanupRunner.Wait(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := acc.gcCleanupRunner.CompletedGeneration(); got != 1 {
		t.Fatalf("expected completed generation 1, got %d", got)
	}
}

func TestGCCleanupRoutineLifecycle(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.gcCleanup = routine.NewRoutineContainer()
	acc.gcCleanup.SetRoutine(acc.runGCCleanup)

	ctx, cancel := context.WithCancel(context.Background())
	acc.gcCleanup.SetContext(ctx, true)
	acc.triggerGCCleanup()
	cancel()
	acc.gcCleanup.ClearContext()
}

func TestRemoveSharedObjectGCRefsRemovesSpacewaveBucketRoots(t *testing.T) {
	ctx := context.Background()
	vol := newGCCleanupTestVolume(t)
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.vol = vol
	soID := "so-1"
	bucketID := BlockStoreBucketID(acc.accountID, SobjectBlockStoreID(soID))
	bucketIRI := block_gc.BucketIRI(bucketID)
	providerIRI := ProviderIRI(acc.p.info.GetProviderId())
	rg := vol.GetRefGraph()
	gcOps := block_gc.NewGCStoreOps(vol, rg)
	if err := gcOps.AddGCRef(ctx, block_gc.NodeGCRoot, bucketIRI); err != nil {
		t.Fatalf("add root ref: %v", err)
	}
	if err := gcOps.AddGCRef(ctx, providerIRI, bucketIRI); err != nil {
		t.Fatalf("add provider ref: %v", err)
	}

	acc.removeSharedObjectGCRefs(ctx, soID, acc.le)

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

func newDeleteSharedObjectTestServer(t *testing.T, soID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/sobject/"+soID+"/delete" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
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
