//go:build !js

package provider_spacewave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/sirupsen/logrus"
)

func TestClassifySessionWSDialErrorReturnsCloudError(t *testing.T) {
	dialErr := errors.New("websocket handshake failed")
	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Body: io.NopCloser(strings.NewReader(
			`{"code":"unknown_session","message":"Session not found","retryable":false}`,
		)),
	}

	err := classifySessionWSDialError(dialErr, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if !isUnauthCloudError(err) {
		t.Fatal("expected unknown_session handshake to be treated as unauthenticated")
	}
	var cloudErr *cloudError
	if !errors.As(err, &cloudErr) {
		t.Fatal("expected cloud error")
	}
	if cloudErr.Code != "unknown_session" {
		t.Fatalf("expected unknown_session code, got %q", cloudErr.Code)
	}
}

func TestClassifySessionWSDialErrorFallsBackForOpaqueHandshakeError(t *testing.T) {
	dialErr := errors.New("websocket handshake failed")
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader("bad gateway")),
	}

	err := classifySessionWSDialError(dialErr, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, dialErr) {
		t.Fatal("expected wrapped dial error")
	}
	var cloudErr *cloudError
	if errors.As(err, &cloudErr) {
		t.Fatal("did not expect opaque handshake failure to become a cloud error")
	}
}

func TestWSTrackerDispatchesBlockStoreNonceEventByResourceID(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	ticket := "ticket-123"
	ticketResp := mustMarshalVT(t, &api.TicketResponse{Ticket: ticket})
	eventPayload := mustMarshalVT(t, &api.BlockStoreNonceEventPayload{
		ResourceId: "space-1",
		Nonce:      42,
	})
	eventMsg := mustMarshalVT(t, &api.SessionMessage{
		Body: &api.SessionMessage_SessionEvent{
			SessionEvent: &api.SessionEvent{
				Type:    "bstore_nonce",
				SoId:    "ignored-so-id",
				Payload: eventPayload,
			},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/ticket":
			_, _ = w.Write(ticketResp)
		case "/api/session/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("accept websocket: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			if err := conn.Write(context.Background(), websocket.MessageBinary, make([]byte, 32)); err != nil {
				t.Fatalf("write challenge: %v", err)
			}
			if _, _, err := conn.Read(context.Background()); err != nil {
				t.Fatalf("read challenge signature: %v", err)
			}
			if err := conn.Write(context.Background(), websocket.MessageBinary, eventMsg); err != nil {
				t.Fatalf("write bstore nonce event: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	tracker := newWSTracker(logrus.NewEntry(logrus.New()), func() *SessionClient {
		return cli
	})
	gotNonce := make(chan uint64, 1)
	tracker.RegisterBlockStoreNonceCallback("space-1", func(nonce uint64) {
		gotNonce <- nonce
	})
	defer tracker.UnregisterBlockStoreNonceCallback("space-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := tracker.runWebSocket(ctx, false)
		done <- err
	}()

	select {
	case nonce := <-gotNonce:
		if nonce != 42 {
			t.Fatalf("block-store nonce callback got %d, want 42", nonce)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for block-store nonce callback")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for wsTracker read loop to stop")
	}
}

func TestWaitForAccountChangedRequiresBroadcast(t *testing.T) {
	bcast := &broadcast.Broadcast{}
	tracker := &wsTracker{accountBcast: bcast}
	ctx := t.Context()

	done := make(chan error, 1)
	go func() {
		done <- tracker.waitForAccountChanged(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("wait returned before account broadcast: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected account broadcast to wake wait")
	}
}

func TestWaitForAccountChangedCancellation(t *testing.T) {
	bcast := &broadcast.Broadcast{}
	tracker := &wsTracker{accountBcast: bcast}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- tracker.waitForAccountChanged(ctx)
	}()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait error = %v, want context canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected cancellation to wake wait")
	}
}
