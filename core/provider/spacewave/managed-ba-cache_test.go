package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestManagedBAsSnapshotCachesClonesAndInvalidates(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/billing/accounts" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		calls++
		displayName := "Managed One"
		if calls > 1 {
			displayName = "Managed Two"
		}
		body, err := (&s4wave_provider_spacewave.ListManagedBillingAccountsResponse{
			Accounts: []*s4wave_provider_spacewave.ManagedBillingAccount{{
				Id:          "ba-1",
				DisplayName: displayName,
			}},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	accounts, err := acc.GetManagedBAsSnapshot(context.Background())
	if err != nil {
		t.Fatalf("initial managed BA snapshot: %v", err)
	}
	accounts[0].DisplayName = "mutated"

	accounts, err = acc.GetManagedBAsSnapshot(context.Background())
	if err != nil {
		t.Fatalf("cached managed BA snapshot: %v", err)
	}
	if accounts[0].GetDisplayName() != "Managed One" {
		t.Fatalf("expected cached managed BA clone, got %+v", accounts[0])
	}
	if calls != 1 {
		t.Fatalf("expected cached reread before invalidation, got %d calls", calls)
	}

	acc.InvalidateManagedBAsList()

	accounts, err = acc.GetManagedBAsSnapshot(context.Background())
	if err != nil {
		t.Fatalf("refetched managed BA snapshot: %v", err)
	}
	if accounts[0].GetDisplayName() != "Managed Two" {
		t.Fatalf("expected refetched managed BA snapshot, got %+v", accounts[0])
	}
	if calls != 2 {
		t.Fatalf("expected refetch after invalidation, got %d calls", calls)
	}
}

func TestManagedBAsSnapshotMarksAccountUnauthenticatedOnUnknownSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/billing/accounts" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte(`{"code":"unknown_session","message":"Session not found","retryable":false}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY

	_, err := acc.GetManagedBAsSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected managed BA fetch to fail")
	}
	if !isUnauthCloudError(err) {
		t.Fatalf("expected unauth cloud error, got %v", err)
	}
	if acc.GetAccountStatus() != provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
		t.Fatalf("expected unauthenticated status, got %v", acc.GetAccountStatus())
	}
}
