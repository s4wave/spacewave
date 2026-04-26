//go:build todo_flake

// See issues/2026/20260425-provider-local-session-transport-deadlock.org.
// Gated behind todo_flake because setupProviderAndSession races the
// sessionTracker against the test body and deadlocks under load.

package provider_local_test

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// newPairingRelayServer creates a test HTTP server that handles pairing
// relay requests. POST /pair returns 201. GET /pair/<code> returns the
// given remotePeerID.
func newPairingRelayServer(remotePeerID peer.ID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			resp := &api.PairingResponse{PeerId: remotePeerID.String()}
			data, _ := resp.MarshalJSON()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

// TestPairingCreatesTransport verifies that GeneratePairingCode creates
// the session transport if it is not already running.
func TestPairingCreatesTransport(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	if acc.GetSessionTransport() != nil {
		t.Fatal("expected no transport before pairing")
	}

	// Pre-create transport without signaling (test relay is HTTP only).
	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	srv := newPairingRelayServer("")
	defer srv.Close()

	code, err := acc.GeneratePairingCode(ctx, srv.URL, sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if code == "" {
		t.Fatal("expected non-empty pairing code")
	}

	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected transport to be running after GeneratePairingCode")
	}
	if st.GetChildBus() == nil {
		t.Fatal("expected child bus to be non-nil")
	}
	if st.GetPeerID() != sess.GetPeerId() {
		t.Fatalf("transport peer %s != session peer %s", st.GetPeerID().String(), sess.GetPeerId().String())
	}

	// Calling again should reuse existing transport.
	code2, err := acc.GeneratePairingCode(ctx, srv.URL, sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if code2 == "" {
		t.Fatal("expected non-empty code on second call")
	}
	if acc.GetSessionTransport() != st {
		t.Fatal("expected same transport on second call")
	}

	acc.StopSessionTransport()
}

// TestCompletePairingWaitsForLink verifies that CompletePairing ensures
// transport is running and sets up a link watch for the remote peer.
func TestCompletePairingWaitsForLink(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Generate a fake remote peer ID for the relay to return.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}

	srv := newPairingRelayServer(remotePeerID)
	defer srv.Close()

	// Pre-create transport without signaling (test relay is HTTP only).
	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	got, err := acc.CompletePairing(ctx, srv.URL, "TESTCODE", sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if got != remotePeerID {
		t.Fatalf("got peer %s, want %s", got.String(), remotePeerID.String())
	}

	// Verify transport is running.
	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected transport to be running after CompletePairing")
	}
	if st.GetChildBus() == nil {
		t.Fatal("expected child bus to be non-nil")
	}

	// Verify pairing state tracks the remote peer.
	if acc.GetPairingRemotePeerID() != remotePeerID {
		t.Fatalf("pairing remote peer %s != expected %s",
			acc.GetPairingRemotePeerID().String(), remotePeerID.String())
	}

	// Verify the link channel exists (directive was added).
	if acc.GetPairingLinkCh() == nil {
		t.Fatal("expected pairing link channel to be non-nil")
	}

	acc.ClearPairingState()
	acc.StopSessionTransport()
}

// TestWatchPairingStatus verifies that pairing state transitions are
// tracked through the broadcast and reflected in snapshots.
func TestWatchPairingStatus(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Initial state: idle.
	snap := acc.GetPairingSnapshot()
	if snap.Status != provider_local.PairingStatusIdle {
		t.Fatalf("expected idle, got %d", snap.Status)
	}

	// Pre-create transport without signaling (test relay is HTTP only).
	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	// Generate pairing code: status transitions to CODE_GENERATED.
	srv := newPairingRelayServer("")
	defer srv.Close()

	code, err := acc.GeneratePairingCode(ctx, srv.URL, sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}

	snap = acc.GetPairingSnapshot()
	if snap.Status != provider_local.PairingStatusCodeGenerated {
		t.Fatalf("expected CODE_GENERATED, got %d", snap.Status)
	}
	if snap.Code != code {
		t.Fatalf("expected code %q, got %q", code, snap.Code)
	}

	// Complete pairing: status transitions to WAITING_FOR_PEER.
	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := newPairingRelayServer(remotePeerID)
	defer srv2.Close()

	_, err = acc.CompletePairing(ctx, srv2.URL, "TESTCODE", sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}

	snap = acc.GetPairingSnapshot()
	if snap.Status != provider_local.PairingStatusWaitingForPeer {
		t.Fatalf("expected WAITING_FOR_PEER, got %d", snap.Status)
	}
	if snap.RemotePeerID != remotePeerID {
		t.Fatalf("expected remote peer %s, got %s", remotePeerID.String(), snap.RemotePeerID.String())
	}

	// Set failed: status transitions to FAILED.
	acc.SetPairingFailed("test error")
	snap = acc.GetPairingSnapshot()
	if snap.Status != provider_local.PairingStatusFailed {
		t.Fatalf("expected FAILED, got %d", snap.Status)
	}
	if snap.ErrMsg != "test error" {
		t.Fatalf("expected error %q, got %q", "test error", snap.ErrMsg)
	}

	// Clear: back to idle.
	acc.ClearPairingState()
	snap = acc.GetPairingSnapshot()
	if snap.Status != provider_local.PairingStatusIdle {
		t.Fatalf("expected idle after clear, got %d", snap.Status)
	}

	acc.StopSessionTransport()
}
