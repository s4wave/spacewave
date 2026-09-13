package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	sdk "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// TestAccountFetcherRefreshesCachedCoverageWithSigner verifies that a restarted
// account discovers restored coverage without a server epoch change or raw key.
func TestAccountFetcherRefreshesCachedCoverageWithSigner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:              7,
				SubscriptionStatus: sdk.BillingStatus_BillingStatus_ACTIVE,
				LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
			}))
		case "/api/account/emails":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountEmailsResponse{}))
		case "/api/account/sessions":
			_, _ = w.Write(mustMarshalVT(t, &api.ListAccountSessionsResponse{}))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	cacheStore := newAccountFetcherCacheStore()
	acc.objStore = cacheStore
	key, id := generateTestKeypair(t)
	acc.sessionClient = NewSessionClientSigner(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, id.String(), func(_ context.Context, data []byte) ([]byte, error) {
		return key.Sign(data)
	})
	acc.state.info = &api.AccountStateResponse{Epoch: 7, SubscriptionStatus: sdk.BillingStatus_BillingStatus_NONE}
	acc.state.lastFetchedEpoch = 7
	acc.state.epoch = 1
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- acc.accountFetcher(ctx) }()
	defer func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("account fetcher: %v", err)
		}
	}()

	waitForAccountBootstrapFetched(t, acc, time.Second)
	state, err := acc.GetAccountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.GetSubscriptionStatus() != sdk.BillingStatus_BillingStatus_ACTIVE {
		t.Fatal("cached inactive coverage survived the live refresh")
	}
	waitForAccountFetcherSignal(t, cacheStore.committed, time.Second, "refreshed coverage cache")
	cached, err := acc.loadAccountStateCache(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached.GetState().GetSubscriptionStatus() != sdk.BillingStatus_BillingStatus_ACTIVE {
		t.Fatal("refreshed coverage was not persisted")
	}
}
