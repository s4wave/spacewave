package s4wave_appconnector_world_test

import (
	"context"
	"crypto/rand"
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

// connectorHarness is a Space World holding a connector, its token Secret and
// the fake admin API it reads.
type connectorHarness struct {
	// tb is the testbed holding the Space World.
	tb *testbed.Testbed
	// api is the fake admin API the connector reads.
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
