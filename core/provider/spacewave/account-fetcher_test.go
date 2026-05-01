package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
)

func TestApplyFetchedAccountState_BootstrapDoesNotConsumeFutureServerEpoch(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.epoch = 1

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:        0,
		KeypairCount: 1,
	}, nil, nil)

	if acc.state.epoch != 0 {
		t.Fatalf("expected settled epoch 0 after bootstrap fetch, got %d", acc.state.epoch)
	}
	if acc.state.lastFetchedEpoch != 0 {
		t.Fatalf("expected last fetched epoch 0 after bootstrap fetch, got %d", acc.state.lastFetchedEpoch)
	}

	acc.setEpoch(1)
	if acc.state.epoch != 1 {
		t.Fatalf("expected remote epoch 1 to trigger after bootstrap fetch, got %d", acc.state.epoch)
	}
}

func TestApplyFetchedAccountState_PreservesConcurrentInvalidation(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.epoch = 2

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:        0,
		KeypairCount: 1,
	}, nil, nil)

	if acc.state.epoch != 2 {
		t.Fatalf("expected concurrent invalidation epoch 2 to be preserved, got %d", acc.state.epoch)
	}
	if acc.state.lastFetchedEpoch != 0 {
		t.Fatalf("expected last fetched epoch 0 after stale fetch, got %d", acc.state.lastFetchedEpoch)
	}
}

func TestApplyFetchedAccountState_PreservesAccountSObjectBindings(t *testing.T) {
	acc := &ProviderAccount{}

	state := &api.AccountStateResponse{
		Epoch: 3,
		AccountSobjectBindings: []*api.AccountSObjectBinding{
			{
				Purpose: "account-settings",
				SoId:    "so-123",
				State:   api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED,
			},
		},
	}
	acc.applyFetchedAccountState(3, state, nil, nil)

	if acc.state.info == nil {
		t.Fatal("expected fetched account state to be stored")
	}
	if len(acc.state.info.GetAccountSobjectBindings()) != 1 {
		t.Fatalf("expected 1 account sobject binding, got %d", len(acc.state.info.GetAccountSobjectBindings()))
	}
	binding := acc.state.info.GetAccountSobjectBindings()[0]
	if binding.GetPurpose() != "account-settings" {
		t.Fatalf("expected binding purpose account-settings, got %q", binding.GetPurpose())
	}
	if binding.GetSoId() != "so-123" {
		t.Fatalf("expected binding so id so-123, got %q", binding.GetSoId())
	}
	if binding.GetState() != api.AccountSObjectBindingState_ACCOUNT_SOBJECT_BINDING_STATE_RESERVED {
		t.Fatalf("expected reserved binding state, got %v", binding.GetState())
	}
}

func TestApplyFetchedAccountState_SetsReadyStatus(t *testing.T) {
	acc := &ProviderAccount{}

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:          1,
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}, nil, nil)

	if acc.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_READY {
		t.Fatalf("expected ready status after successful fetch, got %v", acc.state.status)
	}
}

func TestApplyFetchedAccountState_PreservesDeletedStatus(t *testing.T) {
	acc := &ProviderAccount{}

	acc.applyFetchedAccountState(1, &api.AccountStateResponse{
		Epoch:          1,
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_DELETED,
	}, nil, nil)

	if acc.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
		t.Fatalf("expected deleted status after deleted fetch, got %v", acc.state.status)
	}
}

func TestAccountFetcherResumesAfterUnauthStatusClears(t *testing.T) {
	var stateHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			if stateHits.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"unknown_session","message":"Session not found"}`))
				return
			}
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:          1,
				LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
			}))
		case "/api/account/emails":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountEmailsResponse{}))
		case "/api/account/sessions":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountSessionsResponse{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc.state.epoch = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- acc.accountFetcher(ctx)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	for {
		if waitCtx.Err() != nil {
			t.Fatalf("timed out waiting for unauthenticated status; stateHits=%d", stateHits.Load())
		}
		var status provider.ProviderAccountStatus
		var ch <-chan struct{}
		acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status = acc.state.status
			ch = getWaitCh()
		})
		if status == provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
			break
		}
		select {
		case <-waitCtx.Done():
		case <-ch:
		}
	}

	acc.SetAccountStatus(provider.ProviderAccountStatus_ProviderAccountStatus_READY)
	for {
		if waitCtx.Err() != nil {
			t.Fatalf("timed out waiting for bootstrap fetch; stateHits=%d", stateHits.Load())
		}
		var fetched bool
		var ch <-chan struct{}
		acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			fetched = acc.state.accountBootstrapFetched
			ch = getWaitCh()
		})
		if fetched {
			break
		}
		select {
		case <-waitCtx.Done():
		case <-ch:
		}
	}

	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("accountFetcher() = %v, want context canceled", err)
	}
	if got := stateHits.Load(); got != 2 {
		t.Fatalf("stateHits = %d, want 2", got)
	}
}
