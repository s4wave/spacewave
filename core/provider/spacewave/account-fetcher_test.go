package provider_spacewave

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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
