package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestGetBillingSnapshotCachesAndInvalidates(t *testing.T) {
	var stateCalls, usageCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/billing/ba-1/state":
			stateCalls++
			body, err = (&api.BillingStateResponse{
				Status:         s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
				LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
				DisplayName:    "ba-1",
			}).MarshalVT()
		case "/api/billing/ba-1/usage-query":
			usageCalls++
			body, err = (&api.BillingUsageResponse{
				StorageBytes: 123,
				WriteOps:     4,
				ReadOps:      5,
			}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	for range 2 {
		state, usage, err := acc.GetBillingSnapshot(context.Background(), "ba-1")
		if err != nil {
			t.Fatalf("get billing snapshot: %v", err)
		}
		if state.GetDisplayName() != "ba-1" {
			t.Fatalf("unexpected billing state: %+v", state)
		}
		if usage.GetStorageBytes() != 123 {
			t.Fatalf("unexpected billing usage: %+v", usage)
		}
	}

	acc.InvalidateBillingSnapshot("ba-1")

	state, usage, err := acc.GetBillingSnapshot(context.Background(), "ba-1")
	if err != nil {
		t.Fatalf("get billing snapshot after invalidate: %v", err)
	}
	if state.GetDisplayName() != "ba-1" || usage.GetStorageBytes() != 123 {
		t.Fatalf("unexpected billing snapshot after invalidate: state=%+v usage=%+v", state, usage)
	}

	if stateCalls != 2 || usageCalls != 2 {
		t.Fatalf("expected one cached read and one refetch, got state=%d usage=%d", stateCalls, usageCalls)
	}
}

func TestGetBillingSnapshotReturnsClones(t *testing.T) {
	var stateCalls, usageCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/billing/ba-1/state":
			stateCalls++
			body, err = (&api.BillingStateResponse{
				DisplayName: "original-state",
			}).MarshalVT()
		case "/api/billing/ba-1/usage-query":
			usageCalls++
			body, err = (&api.BillingUsageResponse{
				StorageBytes: 123,
			}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	state, usage, err := acc.GetBillingSnapshot(context.Background(), "ba-1")
	if err != nil {
		t.Fatalf("initial billing snapshot: %v", err)
	}
	state.DisplayName = "mutated"
	usage.StorageBytes = 999

	state, usage, err = acc.GetBillingSnapshot(context.Background(), "ba-1")
	if err != nil {
		t.Fatalf("cached billing snapshot: %v", err)
	}
	if state.GetDisplayName() != "original-state" {
		t.Fatalf("expected cached billing state clone, got %+v", state)
	}
	if usage.GetStorageBytes() != 123 {
		t.Fatalf("expected cached billing usage clone, got %+v", usage)
	}
	if stateCalls != 1 || usageCalls != 1 {
		t.Fatalf("expected clone-only reread, got state=%d usage=%d", stateCalls, usageCalls)
	}
}
