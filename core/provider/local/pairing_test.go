//go:build !goscript

package provider_local_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	websocket "github.com/aperturerobotics/go-websocket"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func handleTestSignaling(w http.ResponseWriter, r *http.Request, signalTickets chan<- struct{}) bool {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/signal/ticket":
		if signalTickets != nil {
			select {
			case signalTickets <- struct{}{}:
			default:
			}
		}
		data, err := (&api.SignalTicketResponse{Token: "test-token"}).MarshalVT()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/api/signal/ws":
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return true
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		<-r.Context().Done()
		return true
	default:
		return false
	}
}

func signedSignalTicketMatches(r *http.Request, expectedEnvPrefix string, expectedPeerID peer.ID) bool {
	if r.Header.Get("X-Peer-ID") != expectedPeerID.String() {
		return false
	}
	timestampMs, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
	if err != nil {
		return false
	}
	bodyHash := sha256.Sum256(nil)
	bodyHashHex := hex.EncodeToString(bodyHash[:])
	if r.Header.Get("X-Sw-Hash") != bodyHashHex {
		return false
	}
	payload, err := (&api.SigningPayload{
		EnvPrefix:     expectedEnvPrefix,
		Method:        http.MethodPost,
		Path:          "/api/signal/ticket",
		TimestampMs:   timestampMs,
		ContentLength: 0,
		BodyHashHex:   bodyHashHex,
	}).MarshalVT()
	if err != nil {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
	if err != nil {
		return false
	}
	pub, err := expectedPeerID.ExtractPublicKey()
	if err != nil {
		return false
	}
	valid, err := pub.Verify(payload, signature)
	return err == nil && valid
}

// newPairingRelayServer creates a test HTTP server that handles pairing
// relay requests. POST /api/pair returns 201. GET /api/pair/<code> returns
// the given remotePeerID.
func newPairingRelayServer(remotePeerID peer.ID) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleTestSignaling(w, r, nil) {
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/pair":
			if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
				t := http.StatusUnsupportedMediaType
				http.Error(w, http.StatusText(t), t)
				return
			}
			var req api.PairingRequest
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t := http.StatusBadRequest
				http.Error(w, http.StatusText(t), t)
				return
			}
			if err := req.UnmarshalVT(body); err != nil {
				t := http.StatusBadRequest
				http.Error(w, http.StatusText(t), t)
				return
			}
			if req.GetCode() == "" || req.GetPeerId() == "" {
				t := http.StatusBadRequest
				http.Error(w, http.StatusText(t), t)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/pair/TESTCODE":
			resp := &api.PairingResponse{PeerId: remotePeerID.String()}
			data, _ := resp.MarshalVT()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func newSignedPairingRelayServer(t *testing.T, expectedEnvPrefix string, expectedPeerID peer.ID) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handleTestSignaling(w, r, nil) {
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/pair" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
			http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
			return
		}
		if signed := r.Header.Get("X-Signed-Headers"); signed != "" {
			http.Error(w, "unexpected signed headers", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		var req api.PairingRequest
		if err := req.UnmarshalVT(body); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if req.GetPeerId() != expectedPeerID.String() {
			http.Error(w, "peer mismatch", http.StatusForbidden)
			return
		}
		bodyHash := sha256.Sum256(body)
		bodyHashHex := hex.EncodeToString(bodyHash[:])
		if got := r.Header.Get("X-Sw-Hash"); got != bodyHashHex {
			http.Error(w, "body hash mismatch", http.StatusBadRequest)
			return
		}
		timestampMs, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
		if err != nil {
			http.Error(w, "bad timestamp", http.StatusBadRequest)
			return
		}
		payload := &api.SigningPayload{
			EnvPrefix:     expectedEnvPrefix,
			Method:        http.MethodPost,
			Path:          "/api/pair",
			TimestampMs:   timestampMs,
			ContentLength: int64(len(body)),
			BodyHashHex:   bodyHashHex,
		}
		payloadBytes, err := payload.MarshalVT()
		if err != nil {
			http.Error(w, "payload marshal failed", http.StatusInternalServerError)
			return
		}
		signature, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Signature"))
		if err != nil {
			http.Error(w, "bad signature encoding", http.StatusBadRequest)
			return
		}
		pub, err := expectedPeerID.ExtractPublicKey()
		if err != nil {
			http.Error(w, "bad peer id", http.StatusBadRequest)
			return
		}
		valid, err := pub.Verify(payloadBytes, signature)
		if err != nil {
			http.Error(w, "signature verification failed", http.StatusBadRequest)
			return
		}
		if !valid {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
}

func TestGeneratePairingCodeSignsRelayRequestWithSigningEnvPrefix(t *testing.T) {
	for _, tc := range []struct {
		name      string
		envPrefix string
	}{
		{name: "prod", envPrefix: "spacewave"},
		{name: "staging", envPrefix: "spacewave-staging"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			_, _, acc, sess, release := setupProviderAndSession(ctx, t)
			defer release()

			if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
				t.Fatal(err)
			}
			defer acc.StopSessionTransport()

			srv := newSignedPairingRelayServer(t, tc.envPrefix, sess.GetPeerId())
			defer srv.Close()

			code, err := acc.GeneratePairingCode(ctx, srv.URL, tc.envPrefix, sess.GetPrivKey(), sess.GetPeerId())
			if err != nil {
				t.Fatal(err)
			}
			if code == "" {
				t.Fatal("expected non-empty pairing code")
			}
		})
	}
}

// TestPairingCreatesTransport verifies that GeneratePairingCode creates
// the session transport if it is not already running.
func TestPairingCreatesTransport(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Pre-create transport without signaling (test relay is HTTP only).
	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	srv := newPairingRelayServer("")
	defer srv.Close()

	code, err := acc.GeneratePairingCode(ctx, srv.URL, "", sess.GetPrivKey(), sess.GetPeerId())
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
	code2, err := acc.GeneratePairingCode(ctx, srv.URL, "", sess.GetPrivKey(), sess.GetPeerId())
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

	got, err := acc.CompletePairing(ctx, srv.URL, "", "TESTCODE", sess.GetPrivKey(), sess.GetPeerId())
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

// TestCompletePairingReplacesEmptyTransportWithSignaling verifies that the
// deployed short-code path starts signaling after accepting a code.
func TestCompletePairingReplacesEmptyTransportWithSignaling(t *testing.T) {
	ctx := t.Context()

	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()
	defer acc.StopSessionTransport()

	remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
	if err != nil {
		t.Fatal(err)
	}

	signalTickets := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/signal/ticket" &&
			!signedSignalTicketMatches(r, "spacewave-staging", sess.GetPeerId()) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if handleTestSignaling(w, r, signalTickets) {
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/pair/TESTCODE" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := &api.PairingResponse{PeerId: remotePeerID.String()}
		data, err := resp.MarshalVT()
		if err != nil {
			t.Errorf("marshal pairing response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	got, err := acc.CompletePairing(ctx, srv.URL, "spacewave-staging", "TESTCODE", sess.GetPrivKey(), sess.GetPeerId())
	if err != nil {
		t.Fatal(err)
	}
	if got != remotePeerID {
		t.Fatalf("got peer %s, want %s", got.String(), remotePeerID.String())
	}

	// CompletePairing starts signaling in the background, so the ticket request
	// lands after it returns. Wait on the test context rather than a private
	// deadline: the test timeout already bounds a hang, and a second stopwatch
	// only decides how loaded a runner has to be before a correct run fails.
	select {
	case <-signalTickets:
	case <-ctx.Done():
		t.Fatal("expected CompletePairing to request a signaling ticket")
	}
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

	code, err := acc.GeneratePairingCode(ctx, srv.URL, "", sess.GetPrivKey(), sess.GetPeerId())
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

	if acc.GetSessionTransport() == nil {
		if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
			t.Fatal(err)
		}
	}

	_, err = acc.CompletePairing(ctx, srv2.URL, "", "TESTCODE", sess.GetPrivKey(), sess.GetPeerId())
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
