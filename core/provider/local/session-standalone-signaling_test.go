package provider_local_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
)

// ticketRequest records the signing headers of one signal ticket request.
type ticketRequest struct {
	peerID    string
	timestamp string
	bodyHash  string
	signature string
}

// signalingServer serves the trusted cloud signaling endpoints used by
// standalone local sessions: POST /api/signal/ticket and GET /api/signal/ws.
// It records ticket requests so the test can observe ticket signing.
type signalingServer struct {
	server     *httptest.Server
	tickets    atomic.Int32
	first      atomic.Pointer[ticketRequest]
	wsAccepted chan struct{}
}

func newSignalingServer() *signalingServer {
	s := &signalingServer{wsAccepted: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/signal/ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.tickets.Add(1)
		s.first.CompareAndSwap(nil, &ticketRequest{
			peerID:    r.Header.Get("X-Peer-ID"),
			timestamp: r.Header.Get("X-Timestamp"),
			bodyHash:  r.Header.Get("X-Sw-Hash"),
			signature: r.Header.Get("X-Signature"),
		})
		data, err := (&api.SignalTicketResponse{Token: "test-token"}).MarshalVT()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/signal/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		s.wsAccepted <- struct{}{}
		defer conn.Close(websocket.StatusNormalClosure, "")
		<-r.Context().Done()
	})
	s.server = httptest.NewServer(mux)
	return s
}

// awaitReadySessionTransport follows account transport replacements until the
// current transport becomes ready.
func awaitReadySessionTransport(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount) *transport.SessionTransport {
	t.Helper()
	for {
		_, waitCh := acc.GetTransportSnapshotWithWait()
		st := acc.GetSessionTransport()
		if st == nil {
			select {
			case <-ctx.Done():
				t.Fatalf("session transport did not start: %v", ctx.Err())
			case <-waitCh:
			}
			continue
		}
		select {
		case <-ctx.Done():
			t.Fatalf("session transport did not become ready: %v", ctx.Err())
		case <-waitCh:
			continue
		case <-st.Ready():
			if acc.GetSessionTransport() == st {
				return st
			}
		}
	}
}

// TestStandaloneSessionSignaling is the in-memory end-to-end regression for
// standalone local session signaling. A standalone local session (no linked
// cloud account) uses the trusted signaling URL persisted in the provider
// configuration: with a URL it signs its own ticket request with the session
// keypair under the configured signing environment and connects the WebRTC
// signaling WebSocket without any cloud account credential; with an empty
// URL it stays without signaling. A daemon restart re-runs the same mount
// path, so a configured URL reconnects after restart.
func TestStandaloneSessionSignaling(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("standalone signaling test requires native HTTP server and WebSocket support")
	}
	for _, prefix := range []string{"", "spacewave-staging"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			// Empty signaling URL keeps the session without WebRTC signaling.
			{
				sig := newSignalingServer()
				defer sig.server.Close()

				_, _, acc, _, release := setupProviderAndSession(ctx, t, "")
				defer release()

				st := awaitReadySessionTransport(ctx, t, acc)
				if got := st.GetStartupStage(); got != "ready" {
					t.Fatalf("startup stage = %q, want ready", got)
				}
				if n := sig.tickets.Load(); n != 0 {
					t.Fatalf("empty signaling URL requested %d tickets, want 0", n)
				}
			}

			// A configured signaling URL drives ticket signing and the signaling
			// WebSocket through the existing WebRTC transport.
			sig := newSignalingServer()
			defer sig.server.Close()

			_, _, acc, sess, release := setupProviderAndSession(ctx, t, sig.server.URL, prefix)
			defer release()

			awaitReadySessionTransport(ctx, t, acc)
			if n := sig.tickets.Load(); n == 0 {
				t.Fatal("standalone session did not request a signal ticket")
			}

			req := sig.first.Load()
			if want := sess.GetPeerId().String(); req.peerID != want {
				t.Fatalf("ticket request peer ID = %q, want session peer %s", req.peerID, want)
			}
			timestampMs, err := strconv.ParseInt(req.timestamp, 10, 64)
			if err != nil {
				t.Fatalf("parse ticket timestamp %q: %v", req.timestamp, err)
			}
			wantPrefix := prefix
			if wantPrefix == "" {
				wantPrefix = "spacewave"
			}
			payload, err := (&api.SigningPayload{
				EnvPrefix:     wantPrefix,
				Method:        http.MethodPost,
				Path:          "/api/signal/ticket",
				TimestampMs:   timestampMs,
				ContentLength: 0,
				BodyHashHex:   req.bodyHash,
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal signing payload: %v", err)
			}
			pub, err := sess.GetPeerId().ExtractPublicKey()
			if err != nil {
				t.Fatalf("extract session public key: %v", err)
			}
			signature, err := base64.StdEncoding.DecodeString(req.signature)
			if err != nil {
				t.Fatalf("decode ticket signature: %v", err)
			}
			valid, err := pub.Verify(payload, signature)
			if err != nil || !valid {
				t.Fatalf("ticket signature not signed by the session keypair: valid=%v err=%v", valid, err)
			}

			select {
			case <-sig.wsAccepted:
			case <-ctx.Done():
				t.Fatal("session transport did not connect to the signaling WebSocket")
			}
		})
	}
}
