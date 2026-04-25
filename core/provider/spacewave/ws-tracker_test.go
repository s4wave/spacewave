//go:build !js

package provider_spacewave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
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
