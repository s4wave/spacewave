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
	// Serve only the account state endpoint, which carries emails and sessions.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/state":
			_, _ = w.Write(mustMarshalVT(t, &api.AccountStateResponse{
				Epoch:              7,
				SubscriptionStatus: sdk.BillingStatus_BillingStatus_ACTIVE,
				LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_ACTIVE,
				Emails:             []*api.AccountEmailInfo{{Email: "a@example.com", Primary: true}},
				Sessions:           []*api.AccountSessionInfo{{PeerId: "peer-live"}},
			}))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Build an account with a cached store and a signing session client.
	acc := NewTestProviderAccount(t, srv.URL)
	cacheStore := newAccountFetcherCacheStore()
	acc.objStore = cacheStore
	key, id := generateTestKeypair(t)
	acc.sessionClient = NewSessionClientSigner(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, id.String(), func(_ context.Context, data []byte) ([]byte, error) {
		return key.Sign(data)
	})

	// Seed stale cached coverage for the account.
	acc.state.info = &api.AccountStateResponse{Epoch: 7, SubscriptionStatus: sdk.BillingStatus_BillingStatus_NONE}
	acc.state.lastFetchedEpoch = 7
	acc.state.epoch = 1

	// Start the account fetcher and stop it with the test context.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- acc.accountFetcher(ctx) }()
	defer func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("account fetcher: %v", err)
		}
	}()

	// The live fetch must refresh the cached coverage and persist it.
	waitForAccountBootstrapFetched(t, acc, time.Second)
	state, err := acc.GetAccountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.GetSubscriptionStatus() != sdk.BillingStatus_BillingStatus_ACTIVE {
		t.Fatal("cached inactive coverage survived the live refresh")
	}

	// The same response publishes the account's emails and sessions.
	acc.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !acc.state.cachedEmailsValid || len(acc.state.cachedEmails) != 1 {
			t.Errorf("emails = %v, want the fetched email", acc.state.cachedEmails)
		}
		if !acc.state.sessionsValid || len(acc.state.sessions) != 1 {
			t.Errorf("sessions = %v, want the fetched session", acc.state.sessions)
		}
	})

	// Verify the refreshed coverage was committed to the cache store.
	waitForAccountFetcherSignal(t, cacheStore.committed, time.Second, "refreshed coverage cache")
	cached, err := acc.loadAccountStateCache(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached.GetState().GetSubscriptionStatus() != sdk.BillingStatus_BillingStatus_ACTIVE {
		t.Fatal("refreshed coverage was not persisted")
	}
}
