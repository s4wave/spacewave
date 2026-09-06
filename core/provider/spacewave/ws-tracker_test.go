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

// TestClassifySessionWSDialErrorReturnsCloudError preserves structured authentication failures.
func TestClassifySessionWSDialErrorReturnsCloudError(t *testing.T) {
	dialErr := errors.New("websocket handshake failed")
	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Body: io.NopCloser(strings.NewReader(
			`{"code":"unknown_session","message":"Session not found","retryable":false}`,
		)),
	}

	// Preserve the response classification and its underlying dial error.
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

// TestClassifySessionWSDialErrorFallsBackForOpaqueHandshakeError retains the original error when the server has no typed response.
func TestClassifySessionWSDialErrorFallsBackForOpaqueHandshakeError(t *testing.T) {
	dialErr := errors.New("websocket handshake failed")
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader("bad gateway")),
	}

	// Preserve the response classification and its underlying dial error.
	err := classifySessionWSDialError(dialErr, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, dialErr) {
		t.Fatal("expected wrapped dial error")
	}
	if _, ok := errors.AsType[*cloudError](err); ok {
		t.Fatal("did not expect opaque handshake failure to become a cloud error")
	}
}

// TestWSTrackerDispatchesBlockStoreNonceEventByResourceID routes block-store events using the resource ID in their payload.
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

	// Serve the ticket and signed websocket exchange used by the tracker.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/ticket":
			_, _ = w.Write(ticketResp)
		case "/api/session/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Errorf("accept websocket: %v", err)
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			if err := conn.Write(r.Context(), websocket.MessageBinary, make([]byte, 32)); err != nil {
				t.Errorf("write challenge: %v", err)
				return
			}
			if _, _, err := conn.Read(r.Context()); err != nil {
				t.Errorf("read challenge signature: %v", err)
				return
			}
			if err := conn.Write(r.Context(), websocket.MessageBinary, eventMsg); err != nil {
				t.Errorf("write bstore nonce event: %v", err)
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	// Register the callback for the payload resource, independently of the envelope.
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	tracker := newWSTracker(logrus.NewEntry(logrus.New()), func() *SessionClient {
		return cli
	})
	gotNonce := make(chan uint64, 1)
	tracker.RegisterBlockStoreNonceCallback("space-1", func(nonce uint64) {
		gotNonce <- nonce
	})
	t.Cleanup(func() { tracker.UnregisterBlockStoreNonceCallback("space-1") })

	// Bind the pending read loop to a cancelable test lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		_, err := tracker.runWebSocket(ctx, false)
		done <- err
	}()

	// Require the expected event without leaving a blocked test on failure.
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

// TestWaitForAccountChangedRequiresBroadcast blocks until the account publishes a change.
func TestWaitForAccountChangedRequiresBroadcast(t *testing.T) {
	bcast := &broadcast.Broadcast{}
	tracker := &wsTracker{accountBcast: bcast}
	ctx := t.Context()

	// Observe the blocking call from the test driver.
	done := make(chan error, 1)
	go func() {
		done <- tracker.waitForAccountChanged(ctx)
	}()

	// Require the expected event without leaving a blocked test on failure.
	select {
	case err := <-done:
		t.Fatalf("wait returned before account broadcast: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// Wake the already-blocked account waiter through its normal notification.
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

// TestWaitForAccountChangedCancellation releases a pending account wait on cancellation.
func TestWaitForAccountChangedCancellation(t *testing.T) {
	bcast := &broadcast.Broadcast{}
	tracker := &wsTracker{accountBcast: bcast}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- tracker.waitForAccountChanged(ctx)
	}()

	// Cancel the wait and require the caller to receive cancellation.
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
