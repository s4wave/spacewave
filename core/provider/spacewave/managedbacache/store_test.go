package managedbacache

import (
	"context"
	"errors"
	"testing"

	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestStoreCachesClonesAndInvalidates(t *testing.T) {
	fetcher := &testFetcher{}
	store := NewStore(nil)

	accounts, err := store.Get(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("initial managed BA snapshot: %v", err)
	}
	accounts[0].DisplayName = "mutated"

	accounts, err = store.Get(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("cached managed BA snapshot: %v", err)
	}
	if accounts[0].GetDisplayName() != "Managed One" {
		t.Fatalf("expected cached managed BA clone, got %+v", accounts[0])
	}
	if fetcher.calls != 1 {
		t.Fatalf("expected cached reread before invalidation, got %d calls", fetcher.calls)
	}

	store.Invalidate()

	accounts, err = store.Get(context.Background(), fetcher)
	if err != nil {
		t.Fatalf("refetched managed BA snapshot: %v", err)
	}
	if accounts[0].GetDisplayName() != "Managed Two" {
		t.Fatalf("expected refetched managed BA snapshot, got %+v", accounts[0])
	}
	if fetcher.calls != 2 {
		t.Fatalf("expected refetch after invalidation, got %d calls", fetcher.calls)
	}
}

func TestStoreReturnsFetchError(t *testing.T) {
	want := errors.New("boom")
	store := NewStore(nil)
	_, err := store.Get(context.Background(), &testFetcher{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("expected fetch error %v, got %v", want, err)
	}
}

type testFetcher struct {
	calls int
	err   error
}

func (f *testFetcher) ListManagedBillingAccounts(context.Context) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls++
	displayName := "Managed One"
	if f.calls > 1 {
		displayName = "Managed Two"
	}
	return (&s4wave_provider_spacewave.ListManagedBillingAccountsResponse{
		Accounts: []*s4wave_provider_spacewave.ManagedBillingAccount{{
			Id:          "ba-1",
			DisplayName: displayName,
		}},
	}).MarshalVT()
}
