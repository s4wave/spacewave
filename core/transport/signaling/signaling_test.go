package signaling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/go-websocket"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestSignaling checks that a ticket goes only to an admitted peer that
// signed for the configured service, and that two admitted peers open a
// direct link through signaling: one admitted elsewhere, as a PC by its
// device socket, the other by a peer socket.
func TestSignaling(t *testing.T) {
	// Serve signaling for the peers in online.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	var mtx sync.Mutex
	online := make(map[peer.ID]bool)
	signaling := New(le, func(_ context.Context, pid peer.ID) bool {
		mtx.Lock()
		defer mtx.Unlock()
		return online[pid]
	}, "test-signaling")
	mux := http.NewServeMux()
	mux.HandleFunc(ticketPath, signaling.ServeTicket)
	mux.HandleFunc(socketPath, signaling.ServeSocket)
	mux.HandleFunc(peerPath, signaling.ServePeer)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Give both peers a key.
	keys := make([]crypto.PrivKey, 2)
	ids := make([]peer.ID, 2)
	var err error
	for i := range keys {
		keys[i], _, err = crypto.GenerateEd25519Key(nil)
		if err != nil {
			t.Fatal(err)
		}
		if ids[i], err = peer.IDFromPrivateKey(keys[i]); err != nil {
			t.Fatal(err)
		}
	}

	// request asks for a ticket for the first peer signed under env.
	request := func(env string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, ticketPath, nil)
		if err := transport.SignHTTPRequest(r, nil, keys[0], ids[0], env); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		signaling.ServeTicket(w, r)
		return w
	}

	// A peer admitted nowhere is refused with a status it retries. Once
	// admitted it gets a ticket, but not for a signature made for another
	// service.
	if w := request("test-signaling"); w.Code != http.StatusForbidden {
		t.Fatal("offline peer got", w.Code)
	}
	mtx.Lock()
	online[ids[0]] = true
	mtx.Unlock()
	if w := request("spacewave"); w.Code != http.StatusForbidden {
		t.Fatal("foreign signature got", w.Code)
	}
	w := request("test-signaling")
	var ticket api.SignalTicketResponse
	if err := ticket.UnmarshalVT(w.Body.Bytes()); w.Code != http.StatusOK || err != nil || ticket.GetToken() == "" {
		t.Fatal("admitted peer got", w.Code, w.Body.String(), err)
	}

	// Redeeming a ticket consumes it before the signaling socket starts.
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + socketPath + "?tk=" + ticket.GetToken()
	firstSocket, _, err := websocket.Dial(ctx, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = firstSocket.CloseNow()
	if reused, response, err := websocket.Dial(ctx, endpoint, nil); err == nil {
		_ = reused.CloseNow()
		t.Fatal("a redeemed ticket opened another socket")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("reused ticket: %v %v", response, err)
	}

	// A ticket that expired while its caller waited cannot open a socket.
	expired := request("test-signaling")
	var old api.SignalTicketResponse
	if err := old.UnmarshalVT(expired.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	signaling.mtx.Lock()
	issued := signaling.tickets[old.GetToken()]
	issued.expires = time.Now().Add(-time.Second)
	signaling.tickets[old.GetToken()] = issued
	signaling.mtx.Unlock()
	expiredEndpoint := "ws" + strings.TrimPrefix(server.URL, "http") + socketPath + "?tk=" + old.GetToken()
	if socket, response, err := websocket.Dial(ctx, expiredEndpoint, nil); err == nil {
		_ = socket.CloseNow()
		t.Fatal("an expired ticket opened a socket")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("expired ticket: %v %v", response, err)
	}

	// A signature cannot claim a different peer from the key that signed it.
	forged := httptest.NewRequest(http.MethodPost, ticketPath, nil)
	if err := transport.SignHTTPRequest(forged, nil, keys[0], ids[0], "test-signaling"); err != nil {
		t.Fatal(err)
	}
	forged.Header.Set("X-Peer-ID", ids[1].String())
	refused := httptest.NewRecorder()
	signaling.ServeTicket(refused, forged)
	if refused.Code != http.StatusForbidden {
		t.Fatal("forged peer got", refused.Code)
	}

	// Admit the second by a peer socket its key signed.
	signed := httptest.NewRequest(http.MethodGet, peerPath, nil)
	if err := transport.SignHTTPRequest(signed, nil, keys[1], ids[1], "test-signaling"); err != nil {
		t.Fatal(err)
	}
	socket, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+peerPath, &websocket.DialOptions{HTTPHeader: signed.Header})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.CloseNow()

	// The production transport clients redeem tickets and exchange offers.
	nodes := make([]*transport.SessionTransport, 2)
	stopped := make(chan error, 2)
	for i, key := range keys {
		b, _, err := core.NewCoreBus(ctx, le)
		if err != nil {
			t.Fatal(err)
		}
		node, err := transport.NewSessionTransport(le, b, key, server.URL, "test-signaling")
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = node
		go func() { stopped <- node.Execute(ctx) }()
		if err := node.AwaitReady(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_, release, err := link.EstablishLinkWithPeerEx(ctx, nodes[1].GetChildBus(), ids[1], ids[0], false)
	if err != nil {
		t.Fatal(err)
	}
	if release == nil {
		t.Fatal("signaling did not establish a link")
	}
	release()

	// Both transports must exit after cancellation, releasing their sockets.
	cancel()
	for range nodes {
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("transport did not stop")
		}
	}
}

// The client contract uses these routes; the application mounts the handlers.
const (
	ticketPath = "/api/signal/ticket"
	socketPath = "/api/signal/ws"
	peerPath   = "/api/signal/peer"
)
