//go:build !js && !tinygo

package transport_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_frame "github.com/s4wave/spacewave/net/signaling/rpc/frame"
	signaling_rpc_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

// TestSessionTransportWebRTCRefusesStream checks that a WebRTC link, which no
// link gate guards, still refuses the streams of a peer the authorizer
// refuses, and that the refusal reaches the dialer.
func TestSessionTransportWebRTCRefusesStream(t *testing.T) {
	// Build a listener that refuses every peer and a dialer that admits any,
	// both signaling through one relay.
	signalingURL := newRelaySignalingServer(t)
	refusal := errors.New("peer is not an active session of this account")
	ctx, _, listener := newTestSessionTransport(t, signalingURL, transport.WithPeerAuthorizer(
		func(context.Context, peer.ID) error { return refusal },
	))
	_, _, dialer := newTestSessionTransport(t, signalingURL, transport.WithPeerAuthorizer(
		func(context.Context, peer.ID) error { return nil },
	))
	for _, st := range []*transport.SessionTransport{listener, dialer} {
		go func() { _ = st.Execute(ctx) }()
		if err := st.AwaitReady(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Report each refusal on its stream; no stream may reach the handler.
	const protocolID = protocol.ID("test/refuse")
	admitted := make(chan stream.Stream, 1)
	release, err := listener.ServeAuthorized(
		protocolID,
		holdStreamHandler(admitted),
		func(_ context.Context, ms link.MountedStream, err error) {
			_, _ = ms.GetStream().Write([]byte(err.Error()))
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Open a stream over WebRTC; the first write delivers it to the listener.
	ms, msRel, err := link.OpenStreamWithPeerEx(
		ctx,
		dialer.GetChildBus(),
		protocolID,
		dialer.GetPeerID(),
		listener.GetPeerID(),
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer msRel()
	strm := ms.GetStream()
	defer strm.Close()
	if _, err := strm.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}

	// The dialer reads the reason, and the handler never sees the stream.
	got := make([]byte, len(refusal.Error()))
	if _, err := io.ReadFull(strm, got); err != nil {
		t.Fatalf("read refusal: %v", err)
	}
	if string(got) != refusal.Error() {
		t.Fatalf("refusal = %q, want %q", got, refusal)
	}
	select {
	case <-admitted:
		t.Fatal("refused stream reached the handler")
	default:
	}
}

// signalingPeerKey keys the WebSocket caller's peer ID in a signaling context.
type signalingPeerKey struct{}

// newRelaySignalingServer serves signal tickets and the signaling WebSocket
// and returns its URL. A ticket names the peer that requested it, and the
// WebSocket identifies its caller by the ticket, as the cloud does.
func newRelaySignalingServer(t *testing.T) string {
	// Relay signaling between the peers identified by their tickets.
	t.Helper()
	relay := signaling_rpc_server.NewServerWithIdentify(
		logrus.New().WithField("test", t.Name()),
		func(ctx context.Context) (peer.ID, error) {
			pid, ok := ctx.Value(signalingPeerKey{}).(peer.ID)
			if !ok {
				return "", errors.New("signaling caller has no ticket")
			}
			return pid, nil
		},
	)
	mux := srpc.NewMux()
	if err := signaling_rpc.SRPCRegisterSignaling(mux, relay); err != nil {
		t.Fatal(err)
	}

	// Issue each peer its own ID as the ticket.
	routes := http.NewServeMux()
	routes.HandleFunc("POST /api/signal/ticket", func(w http.ResponseWriter, r *http.Request) {
		data, err := (&api.SignalTicketResponse{Token: r.Header.Get("X-Peer-ID")}).MarshalVT()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})

	// Serve the relay over the WebSocket of the ticket's peer.
	routes.HandleFunc("GET /api/signal/ws", func(w http.ResponseWriter, r *http.Request) {
		// Identify the caller by its ticket.
		pid, err := peer.IDB58Decode(r.URL.Query().Get("tk"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Serve signaling calls until the peer hangs up.
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := context.WithValue(r.Context(), signalingPeerKey{}, pid)
		_ = signaling_rpc_frame.NewConn(ctx, conn).ReadPump(signaling_rpc_frame.NewServerAccept(ctx, mux))
	})
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server.URL
}
