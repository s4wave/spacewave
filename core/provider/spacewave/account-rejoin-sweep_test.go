package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// TestRunSelfRejoinSweepNoSubscriptionSkipsFetch covers the no-access fast
// path: an account whose subscription state does not permit SO list access
// must short-circuit the sweep with zero HTTP traffic. Hot path for the
// "reconnect during dormant subscription" case where the sweep would
// otherwise fan out a /sobject/list fetch we already know will be rejected.
func TestRunSelfRejoinSweepNoSubscriptionSkipsFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call from rejoin sweep: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	if err := acc.runSelfRejoinSweep(context.Background(), &selfRejoinSweepState{
		generation: 1,
	}); err != nil {
		t.Fatalf("runSelfRejoinSweep: %v", err)
	}
}

// TestRunSelfRejoinSweepEmptyListSingleFetch covers the warm-but-empty path:
// active subscription, server returns an empty SO list. The sweep must issue
// exactly one /sobject/list fetch and exit cleanly without per-SO mount
// traffic. This is the bound that protects against per-mount rejoin storms
// when reconnect fires against an account with no shared objects.
func TestRunSelfRejoinSweepEmptyListSingleFetch(t *testing.T) {
	var (
		mu   sync.Mutex
		hits = make(map[string]int)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/api/sobject/list":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(
		s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	)

	if err := acc.runSelfRejoinSweep(context.Background(), &selfRejoinSweepState{
		generation: 1,
	}); err != nil {
		t.Fatalf("runSelfRejoinSweep: %v", err)
	}

	mu.Lock()
	got := hits["/api/sobject/list"]
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 /sobject/list fetch, got %d (full hit map: %v)", got, hits)
	}
}

func TestRunSelfRejoinSweepProcessesMailboxesWithoutMounting(t *testing.T) {
	var (
		mu   sync.Mutex
		hits = make(map[string]int)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/api/sobject/list":
			_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{
				SharedObjects: []*sobject.SharedObjectListEntry{
					{Ref: sobject.NewSharedObjectRef("spacewave", "test-account", "so-1", SobjectBlockStoreID("so-1"))},
					{Ref: sobject.NewSharedObjectRef("spacewave", "test-account", "so-2", SobjectBlockStoreID("so-2"))},
				},
			}))
		case "/api/sobject/so-1/invite-mailbox",
			"/api/sobject/so-2/invite-mailbox":
			_, _ = w.Write(mustMarshalVT(t, &api.GetMailboxResponse{}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(
		s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	)

	if err := acc.runSelfRejoinSweep(context.Background(), &selfRejoinSweepState{
		generation: 1,
	}); err != nil {
		t.Fatalf("runSelfRejoinSweep: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["/api/sobject/list"] != 1 {
		t.Fatalf("expected 1 /sobject/list fetch, got %d", hits["/api/sobject/list"])
	}
	if hits["/api/sobject/so-1/invite-mailbox"] != 1 {
		t.Fatalf("expected so-1 mailbox fetch, got %d", hits["/api/sobject/so-1/invite-mailbox"])
	}
	if hits["/api/sobject/so-2/invite-mailbox"] != 1 {
		t.Fatalf("expected so-2 mailbox fetch, got %d", hits["/api/sobject/so-2/invite-mailbox"])
	}
}

func TestBuildSelfRejoinSweepStateLockedRequiresBootstrapAndTrigger(t *testing.T) {
	acc := &ProviderAccount{
		sessionClient: &SessionClient{
			SignedHTTPClient: &SignedHTTPClient{
				priv:   testSigner(t),
				peerID: "peer-1",
			},
		},
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	}
	acc.state.lastFetchedEpoch = 7
	acc.state.selfRejoinSweepGeneration = 1

	if got := acc.buildSelfRejoinSweepStateLocked(); got != nil {
		t.Fatalf("expected nil sweep state before bootstrap, got %+v", got)
	}

	acc.state.accountBootstrapFetched = true
	acc.state.selfRejoinSweepGeneration = 0
	if got := acc.buildSelfRejoinSweepStateLocked(); got != nil {
		t.Fatalf("expected nil sweep state before any trigger, got %+v", got)
	}

	acc.state.selfRejoinSweepGeneration = 2
	acc.state.info.Keypairs = []*session.EntityKeypair{{PeerId: "peer-1"}}
	got := acc.buildSelfRejoinSweepStateLocked()
	if got == nil {
		t.Fatal("expected sweep state after bootstrap and trigger")
	}
	if got.fetchedEpoch != 7 {
		t.Fatalf("expected fetched epoch 7, got %d", got.fetchedEpoch)
	}
	if got.generation != 2 {
		t.Fatalf("expected generation 2, got %d", got.generation)
	}
	if len(got.keypairs) != 1 || got.keypairs[0].GetPeerId() != "peer-1" {
		t.Fatalf("unexpected keypairs snapshot: %+v", got.keypairs)
	}
}

func TestBumpSelfRejoinSweepGenerationNoRoutineNoOp(t *testing.T) {
	acc := &ProviderAccount{}
	acc.bumpSelfRejoinSweepGeneration()
	if acc.state.selfRejoinSweepGeneration != 1 {
		t.Fatalf("expected generation 1, got %d", acc.state.selfRejoinSweepGeneration)
	}
}

func TestPrimeSelfRejoinSweepFromUnlockedEntityKeys(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	store := NewEntityKeyStore()
	store.Unlock(pid, priv)
	acc := &ProviderAccount{
		entityKeyStore: store,
	}

	acc.primeSelfRejoinSweepFromUnlockedEntityKeys()

	if acc.state.selfRejoinSweepGeneration != 1 {
		t.Fatalf("expected generation 1, got %d", acc.state.selfRejoinSweepGeneration)
	}
}

func testSigner(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _ := generateTestKeypair(t)
	return priv
}
