package transport

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	ws "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	"github.com/sirupsen/logrus"
)

// TestWSSignalPeerResolverWaitsForConnection verifies cancellation before signaling is ready.
func TestWSSignalPeerResolverWaitsForConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ctrl := &wsSignalingCtrl{ready: make(chan struct{})}
	resolver := &wsSignalPeerResolver{
		c:   ctrl,
		dir: signaling.NewSignalPeer("webrtc", peer.ID("local"), peer.ID("remote")),
	}
	if err := resolver.Resolve(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error = %v, want context cancellation while waiting for signaling", err)
	}
}

// TestWSSignalingControllerRefreshesTicketAfterConnectionExpires verifies that
// a disconnected WebSocket reconnects with a newly issued authorization ticket.
func TestWSSignalingControllerRefreshesTicketAfterConnectionExpires(t *testing.T) {
	// Generate the identity used to request tickets from the test cloud.
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	t.Cleanup(cancel)
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Issue unique tickets so a stale URL cannot authorize a later generation.
	var mtx sync.Mutex
	issuedTickets := make([]string, 0, 2)
	authorizedTickets := make([]string, 0, 2)
	validTicket := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Issue the typed cloud response used by the production ticket client.
		if r.Method == http.MethodPost && r.URL.Path == "/api/signal/ticket" {
			mtx.Lock()
			ticket := "ticket-" + strconv.Itoa(len(issuedTickets)+1)
			issuedTickets = append(issuedTickets, ticket)
			validTicket = ticket
			mtx.Unlock()
			data, err := (&api.SignalTicketResponse{Token: ticket}).MarshalVT()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			if _, err := w.Write(data); err != nil {
				t.Error(err)
			}
			return
		}

		// Reject reuse of the ticket whose first connection has expired.
		mtx.Lock()
		ticket := r.URL.Query().Get("tk")
		valid := ticket != "" && ticket == validTicket
		if valid {
			validTicket = ""
		}
		mtx.Unlock()
		if !valid {
			http.Error(w, "expired signaling ticket", http.StatusUnauthorized)
			return
		}

		// Drop the first accepted socket, then end the check at the second handshake.
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		mtx.Lock()
		authorizedTickets = append(authorizedTickets, ticket)
		complete := len(authorizedTickets) == 2
		mtx.Unlock()
		if complete {
			cancel()
		}
	}))
	t.Cleanup(server.Close)

	// Exercise the real ticket client, URL builder, WebSocket dial, and retry loop.
	ctrl := &wsSignalingCtrl{
		le:         logrus.NewEntry(logrus.New()),
		ready:      make(chan struct{}),
		priv:       priv,
		sigID:      "webrtc",
		pid:        pid,
		retryDelay: time.Millisecond,
		url: func(ctx context.Context) (string, error) {
			ticket, err := acquireSignalTicket(ctx, server.URL, priv, pid, "")
			if err != nil {
				return "", err
			}
			return signalWebSocketURL(server.URL, ticket)
		},
	}
	if err := ctrl.Execute(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context cancellation", err)
	}

	// Snapshot endpoint state before checking the generation sequence.
	mtx.Lock()
	gotIssued := slices.Clone(issuedTickets)
	gotAuthorized := slices.Clone(authorizedTickets)
	mtx.Unlock()
	if want := []string{"ticket-1", "ticket-2"}; !slices.Equal(gotIssued, want) {
		t.Fatalf("issued tickets = %v, want %v", gotIssued, want)
	}
	if want := []string{"ticket-1", "ticket-2"}; !slices.Equal(gotAuthorized, want) {
		t.Fatalf("authorized tickets = %v, want %v", gotAuthorized, want)
	}
}

// genTestHandler records SignalPeer resolver values.
type genTestHandler struct {
	directive.ResolverHandler

	mtx     sync.Mutex
	nextID  uint32
	values  map[uint32]directive.Value
	removed map[uint32]func()
	addedCh chan uint32
}

// AddValue records an added value.
func (h *genTestHandler) AddValue(val directive.Value) (uint32, bool) {
	h.mtx.Lock()
	h.nextID++
	id := h.nextID
	h.values[id] = val
	h.mtx.Unlock()
	h.addedCh <- id
	return id, true
}

// RemoveValue removes a value and calls its removed callback.
func (h *genTestHandler) RemoveValue(id uint32) (directive.Value, bool) {
	h.mtx.Lock()
	val, ok := h.values[id]
	delete(h.values, id)
	cb := h.removed[id]
	delete(h.removed, id)
	h.mtx.Unlock()
	if cb != nil {
		cb()
	}
	return val, ok
}

// AddValueRemovedCallback records the value removed callback.
func (h *genTestHandler) AddValueRemovedCallback(id uint32, cb func()) func() {
	h.mtx.Lock()
	h.removed[id] = cb
	h.mtx.Unlock()
	return func() {}
}

// TestWSSignalPeerResolverReplacesValueOnReconnect verifies the resolver
// replaces its value when the signaling connection generation changes.
func TestWSSignalPeerResolverReplacesValueOnReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	ctrl := &wsSignalingCtrl{ready: make(chan struct{})}
	startGen := func() chan struct{} {
		client, err := signaling_rpc_client.NewClient(le, nil, priv, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctrl.mtx.Lock()
		defer ctrl.mtx.Unlock()
		ctrl.client = client
		ctrl.done = make(chan struct{})
		close(ctrl.ready)
		return ctrl.done
	}
	endGen := func() {
		ctrl.mtx.Lock()
		defer ctrl.mtx.Unlock()
		ctrl.client = nil
		close(ctrl.done)
		ctrl.done = nil
		ctrl.ready = make(chan struct{})
	}

	handler := &genTestHandler{
		values:  make(map[uint32]directive.Value),
		removed: make(map[uint32]func()),
		addedCh: make(chan uint32, 4),
	}
	resolver := &wsSignalPeerResolver{
		c:   ctrl,
		dir: signaling.NewSignalPeer("webrtc", peer.ID("local"), peer.ID("remote")),
	}
	errCh := make(chan error, 1)
	go func() { errCh <- resolver.Resolve(ctx, handler) }()

	waitAdded := func() uint32 {
		t.Helper()
		select {
		case id := <-handler.addedCh:
			return id
		case <-ctx.Done():
			t.Fatal("timed out waiting for resolver value")
		}
		return 0
	}

	startGen()
	first := waitAdded()
	endGen()
	startGen()
	second := waitAdded()
	if second == first {
		t.Fatal("expected a new value for the new generation")
	}
	handler.mtx.Lock()
	_, firstPresent := handler.values[first]
	handler.mtx.Unlock()
	if firstPresent {
		t.Fatal("expected stale value to be removed")
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error = %v, want context cancellation", err)
	}
}

// TestWSSignalingGenerationEndsOnCloseFrame verifies that a server close
// frame ends the connection generation without waiting for the ping.
func TestWSSignalingGenerationEndsOnCloseFrame(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Reject the first frame the way the signaling server rejects one it
	// cannot decode.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		if _, _, err := conn.Read(r.Context()); err != nil {
			return
		}
		_ = conn.Close(ws.StatusInvalidFramePayloadData, "invalid signaling frame")
	}))
	t.Cleanup(server.Close)

	ctrl := &wsSignalingCtrl{
		le:    logrus.NewEntry(logrus.New()),
		ready: make(chan struct{}),
		priv:  priv,
		sigID: "webrtc",
		pid:   pid,
		url: func(context.Context) (string, error) {
			return "ws" + strings.TrimPrefix(server.URL, "http"), nil
		},
	}
	start := time.Now()
	err = ctrl.executeGeneration(ctx)
	if elapsed := time.Since(start); elapsed > signalingWebSocketPingInterval/3 {
		t.Fatalf("generation ended after %v with %v, want it to end at the close frame", elapsed, err)
	}
	if err == nil || ctx.Err() != nil {
		t.Fatalf("generation error = %v, want the close frame error", err)
	}
}
