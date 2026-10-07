package s4wave_appconnector_world_test

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
	spacewave_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_appconnector "github.com/s4wave/spacewave/sdk/appconnector"
	s4wave_appconnector_world "github.com/s4wave/spacewave/sdk/appconnector/world"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/s4wave/spacewave/testbed"
)

const (
	connectorKey = "connectors/hyperline"
	statsPath    = "/admin/stats"
	bigPath      = "/admin/big"
	statsBody    = `{"users":3}`
	apiToken     = "hyperline-admin-token"
	maxBodyBytes = 32
)

// fakeAdminAPI is a local HTTP server that stands in for the application's
// admin API. It accepts only the API token until the token is revoked.
type fakeAdminAPI struct {
	server   *httptest.Server
	requests atomic.Int32
	revoked  atomic.Bool
}

func newFakeAdminAPI(t *testing.T) *fakeAdminAPI {
	// Report failures at the caller.
	t.Helper()

	// Serve each path behind the bearer token check.
	api := &fakeAdminAPI{}
	mux := http.NewServeMux()
	serve := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			api.requests.Add(1)
			if api.revoked.Load() || r.Header.Get("Authorization") != "Bearer "+apiToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc(statsPath, serve("application/json", statsBody))
	mux.HandleFunc(bigPath, serve("text/plain", strings.Repeat("x", 2*maxBodyBytes)))
	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)
	return api
}

// connectorHarness is a Space World holding a connector, its token Secret and
// the fake admin API it reads.
type connectorHarness struct {
	tb  *testbed.Testbed
	api *fakeAdminAPI
	// sessionPeerID is the session peer granted to read the token Secret.
	sessionPeerID peer.ID
}

// newConnectorHarness creates the token Secret and an AppConnector reading both
// admin paths. Only the harness's session peer is granted the token Secret.
func newConnectorHarness(ctx context.Context, t *testing.T) *connectorHarness {
	// Report failures at the caller.
	t.Helper()

	// Start the testbed and the fake admin API.
	tb, soProvider := setupSecretTest(ctx, t)
	api := newFakeAdminAPI(t)

	// Store the API token as a Secret of the API token kind.
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/hyperline-token",
		DisplayName: "Hyperline admin token",
		Kind:        s4wave_secret.SecretKindAPIToken,
		ContentType: s4wave_secret.APITokenContentType,
		Value:       []byte(apiToken),
		Timestamp:   time.Unix(100, 0),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	_, sessionPub, sessionPeerID := makePeer(t)
	if _, err := s4wave_secret.AddSecretParticipant(
		ctx,
		tb.Bus,
		secret,
		sessionPeerID.String(),
		sessionPub,
		sobject.SOParticipantRole_SOParticipantRole_READER,
		"",
	); err != nil {
		t.Fatalf("AddSecretParticipant: %v", err)
	}

	// Create the connector, polling fast enough for the test to observe cycles.
	op := s4wave_appconnector.NewCreateAppConnectorOp(
		connectorKey,
		"Hyperline admin",
		api.server.URL,
		[]*s4wave_appconnector.AppRead{
			{Name: "stats", Path: statsPath},
			{Name: "big", Path: bigPath},
		},
		"secrets/hyperline-token",
		20,
		maxBodyBytes,
		time.Unix(100, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	return &connectorHarness{tb: tb, api: api, sessionPeerID: sessionPeerID}
}

// execute runs the connector's process as the Space plugin controller does.
// Cancelling ctx revokes the binding. The returned channel receives the error
// that ended the process.
func (h *connectorHarness) execute(ctx context.Context, t *testing.T, sessionPeerID peer.ID) <-chan error {
	// Report failures at the caller.
	t.Helper()

	// Build the connector's process as the daemon does for a binding.
	ctx = objecttype.WithSessionPeerID(ctx, sessionPeerID)
	invoker, cleanup, err := s4wave_appconnector_world.AppConnectorType.GetFactory()(
		ctx,
		h.tb.Logger,
		h.tb.Bus,
		h.tb.BusEngine,
		h.tb.WorldState,
		connectorKey,
	)
	if err != nil {
		t.Fatalf("connector factory: %v", err)
	}
	if invoker == nil {
		t.Fatal("connector factory returned no process")
	}
	t.Cleanup(cleanup)

	// Run Execute and report the error that ends the stream.
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	strm, err := s4wave_process.NewSRPCPersistentExecutionServiceClient(client).Execute(ctx, &s4wave_process.ExecuteRequest{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		for {
			if _, err := strm.Recv(); err != nil {
				done <- err
				return
			}
		}
	}()
	return done
}

// awaitSnapshot waits until the snapshot satisfies ready and returns it.
func (h *connectorHarness) awaitSnapshot(
	ctx context.Context,
	t *testing.T,
	ready func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool,
) map[string]*s4wave_appconnector.AppReadSnapshot {
	t.Helper()
	ws := h.tb.WorldState
	for {
		// Read the seqno first so a write after the read wakes the wait.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			t.Fatalf("GetSeqno: %v", err)
		}
		snapshot, err := world.LookupObjectBody[*s4wave_appconnector.AppSnapshot](
			ctx,
			ws,
			s4wave_appconnector.SnapshotObjectKey(connectorKey),
			s4wave_appconnector.NewAppSnapshotBlock,
		)
		if err != nil {
			t.Fatalf("read snapshot: %v", err)
		}
		reads := make(map[string]*s4wave_appconnector.AppReadSnapshot, len(snapshot.GetReads()))
		for _, read := range snapshot.GetReads() {
			reads[read.GetName()] = read
		}
		if ready(reads) {
			return reads
		}
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatalf("wait for snapshot: %v", err)
		}
	}
}

// TestConnectorKeepsSnapshotCurrent checks a good fetch, a revoked token that
// keeps the last good body, and the process ending with its binding.
func TestConnectorKeepsSnapshotCurrent(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Start the connector under a context that stands for its binding.
	h := newConnectorHarness(ctx, t)
	execCtx, revokeBinding := context.WithCancel(ctx)
	defer revokeBinding()
	done := h.execute(execCtx, t, h.sessionPeerID)

	// The snapshot records the good response with its content type.
	good := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetFetchedAt() != nil
	})["stats"]
	if string(good.GetBody()) != statsBody || good.GetContentType() != "application/json" || good.GetStatus() != http.StatusOK || good.GetError() != "" {
		t.Fatalf("good read = %+v", good)
	}

	// A revoked token records an error and leaves the last good body.
	h.api.revoked.Store(true)
	failed := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetError() != ""
	})["stats"]
	if failed.GetError() != "unexpected status 401" {
		t.Fatalf("error = %q", failed.GetError())
	}
	if string(failed.GetBody()) != statsBody || !failed.GetFetchedAt().AsTime().Equal(good.GetFetchedAt().AsTime()) {
		t.Fatalf("failed read lost the last good response: %+v", failed)
	}

	// Revoking the binding ends the process and the polling.
	revokeBinding()
	if err := <-done; err == nil {
		t.Fatal("process ended without an error")
	}
	requests := h.api.requests.Load()
	time.Sleep(200 * time.Millisecond)
	if got := h.api.requests.Load(); got != requests {
		t.Fatalf("requests after revocation = %d, want %d", got, requests)
	}
}

// TestConnectorLimitsBodySize checks that an oversized body is an error and
// is never stored.
func TestConnectorLimitsBodySize(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Run the connector against a body larger than the limit.
	h := newConnectorHarness(ctx, t)
	h.execute(ctx, t, h.sessionPeerID)

	// The oversized read records an error and stores no body.
	reads := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["big"].GetError() != ""
	})
	big := reads["big"]
	if big.GetError() != "response exceeds 32 bytes" || len(big.GetBody()) != 0 {
		t.Fatalf("big read = %+v", big)
	}
}

// TestConnectorCannotReadTokenWithoutGrant checks that a session peer outside
// the token Secret's grants sends no request and records an error.
func TestConnectorCannotReadTokenWithoutGrant(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Run the connector as a peer the token Secret does not grant.
	h := newConnectorHarness(ctx, t)
	_, _, ungrantedPeerID := makePeer(t)
	h.execute(ctx, t, ungrantedPeerID)

	// The denial is recorded and the API is never called.
	reads := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetError() != ""
	})
	if !strings.Contains(reads["stats"].GetError(), s4wave_secret.ErrPayloadAccessDenied.Error()) {
		t.Fatalf("error = %q", reads["stats"].GetError())
	}
	if got := h.api.requests.Load(); got != 0 {
		t.Fatalf("requests without a grant = %d, want 0", got)
	}
}

// setupSecretTest starts a testbed with a local shared object provider.
func setupSecretTest(ctx context.Context, t *testing.T) (*testbed.Testbed, sobject.SharedObjectProvider) {
	// Start the testbed.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register a local provider for the test peer.
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

	// Open the provider account and obtain its SharedObject feature.
	accountID := "test-account-" + sobject.NewSOOperationLocalID()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provAccRef.Release)
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err)
	}
	return tb, soProvider
}

// makePeer generates an Ed25519 peer.
func makePeer(t *testing.T) (spacewave_crypto.PrivKey, spacewave_crypto.PubKey, peer.ID) {
	// Generate the key and derive its peer ID.
	t.Helper()
	priv, pub, err := spacewave_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub, peerID
}
