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
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two serialized cleanup sweeps, got %d", got)
	}
}

func TestDeleteSharedObjectTriggersGCCleanup(t *testing.T) {
	const soID = "so-cleanup"
	srv := newDeleteSharedObjectTestServer(t, soID)
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if err := acc.DeleteSharedObject(context.Background(), soID); err != nil {
		t.Fatalf("delete shared object: %v", err)
	}

	var generation uint64
	acc.gcCleanupBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		generation = acc.gcCleanupGeneration
	})
	if generation != 1 {
		t.Fatalf("expected cleanup generation 1, got %d", generation)
	}
}

func TestHandleAccountSONotifyDeleteTriggersGCCleanup(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")

	acc.handleAccountSONotify(context.Background(), "so-1", &api.SONotifyEventPayload{
		ChangeType: "delete",
	})

	var generation uint64
	acc.gcCleanupBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		generation = acc.gcCleanupGeneration
	})
	if generation != 1 {
		t.Fatalf("expected cleanup generation 1, got %d", generation)
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

func newDeleteSharedObjectTestServer(t *testing.T, soID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/sobject/"+soID+"/delete" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
}
