package billingcache

import (
	"context"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestStoreCachesAndInvalidates(t *testing.T) {
	fetcher := &testFetcher{
		state: &api.BillingStateResponse{DisplayName: "ba-1"},
		usage: &api.BillingUsageResponse{StorageBytes: 123},
	}
	store := NewStore(nil)

	for range 2 {
		state, usage, err := store.Get(context.Background(), "ba-1", fetcher)
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

	store.Invalidate("ba-1")

	state, usage, err := store.Get(context.Background(), "ba-1", fetcher)
	if err != nil {
		t.Fatalf("get billing snapshot after invalidate: %v", err)
	}
	if state.GetDisplayName() != "ba-1" || usage.GetStorageBytes() != 123 {
		t.Fatalf("unexpected billing snapshot after invalidate: state=%+v usage=%+v", state, usage)
	}

	if fetcher.stateCalls != 2 || fetcher.usageCalls != 2 {
		t.Fatalf("expected one cached read and one refetch, got state=%d usage=%d", fetcher.stateCalls, fetcher.usageCalls)
	}
}

func TestStoreReturnsClones(t *testing.T) {
	fetcher := &testFetcher{
		state: &api.BillingStateResponse{DisplayName: "original-state"},
		usage: &api.BillingUsageResponse{StorageBytes: 123},
	}
	store := NewStore(nil)

	state, usage, err := store.Get(context.Background(), "ba-1", fetcher)
	if err != nil {
		t.Fatalf("initial billing snapshot: %v", err)
	}
	state.DisplayName = "mutated"
	usage.StorageBytes = 999

	state, usage, err = store.Get(context.Background(), "ba-1", fetcher)
	if err != nil {
		t.Fatalf("cached billing snapshot: %v", err)
	}
	if state.GetDisplayName() != "original-state" {
		t.Fatalf("expected cached billing state clone, got %+v", state)
	}
	if usage.GetStorageBytes() != 123 {
		t.Fatalf("expected cached billing usage clone, got %+v", usage)
	}
	if fetcher.stateCalls != 1 || fetcher.usageCalls != 1 {
		t.Fatalf("expected clone-only reread, got state=%d usage=%d", fetcher.stateCalls, fetcher.usageCalls)
	}
}

type testFetcher struct {
	state *api.BillingStateResponse
	usage *api.BillingUsageResponse

	stateCalls int
	usageCalls int
}

func (f *testFetcher) GetBillingState(context.Context, string) ([]byte, error) {
	f.stateCalls++
	return f.state.MarshalVT()
}

func (f *testFetcher) GetBillingUsage(context.Context, string) ([]byte, error) {
	f.usageCalls++
	return f.usage.MarshalVT()
}
