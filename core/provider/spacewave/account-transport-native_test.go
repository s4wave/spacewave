//go:build !goscript

package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests exercise the native WebRTC session transport startup lifecycle,
// which reaches the signal-ticket HTTP endpoint through startWebRTCControllers.
// The goscript browser build replaces that selector with a no-op (browser
// sessions obtain transport from the web runtime), so the signal-ticket
// lifecycle does not exist there and these tests only apply to the native build.

func TestCreateSessionTransportConcurrentReplacementKeepsNewTransport(t *testing.T) {
	requested := make(chan struct{})
	unexpectedPath := make(chan string, 1)
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/signal/ticket" {
			select {
			case unexpectedPath <- r.URL.Path:
			default:
			}
			http.NotFound(w, r)
			return
		}
		once.Do(func() { close(requested) })
		<-r.Context().Done()
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	oldPriv, _ := generateTestKeypair(t)
	oldDone := make(chan error, 1)
	go func() {
		oldDone <- acc.CreateSessionTransport(context.Background(), oldPriv, srv.URL)
	}()
	waitForSessionTransportSignal(t, requested, time.Second, "old signal ticket request")

	newPriv, _ := generateTestKeypair(t)
	if err := acc.CreateSessionTransport(context.Background(), newPriv, ""); err != nil {
		t.Fatalf("new CreateSessionTransport: %v", err)
	}
	current := acc.GetSessionTransport()
	if current == nil {
		t.Fatal("expected newer ready session transport to remain current")
	}
	if !current.GetPeerID().MatchesPrivateKey(newPriv) {
		t.Fatalf("current transport peer = %q, want replacement peer", current.GetPeerID().String())
	}

	select {
	case err := <-oldDone:
		if err == nil {
			t.Fatal("expected old CreateSessionTransport to fail after replacement")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for old CreateSessionTransport to exit")
	}
	if acc.GetSessionTransport() != current {
		t.Fatal("old CreateSessionTransport cleared the newer transport")
	}
	assertNoUnexpectedSessionTransportPath(t, unexpectedPath)
}

func TestCreateSessionTransportEarlyExitClearsCurrentTransport(t *testing.T) {
	unexpectedPath := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/signal/ticket" {
			select {
			case unexpectedPath <- r.URL.Path:
			default:
			}
			http.NotFound(w, r)
			return
		}
		http.Error(w, "no ticket", http.StatusInternalServerError)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	priv, _ := generateTestKeypair(t)
	err := acc.CreateSessionTransport(context.Background(), priv, srv.URL)
	if err == nil {
		t.Fatal("expected session transport startup failure")
	}
	if !strings.Contains(err.Error(), "session transport failed to start") {
		t.Fatalf("startup error = %v, want wrapped startup failure", err)
	}
	if st := acc.GetSessionTransport(); st != nil {
		t.Fatal("expected failed session transport to be cleared")
	}
	assertNoUnexpectedSessionTransportPath(t, unexpectedPath)
}

func TestCreateSessionTransportCancellationStopsStartup(t *testing.T) {
	requested := make(chan struct{})
	unexpectedPath := make(chan string, 1)
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/signal/ticket" {
			select {
			case unexpectedPath <- r.URL.Path:
			default:
			}
			http.NotFound(w, r)
			return
		}
		once.Do(func() { close(requested) })
		<-r.Context().Done()
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	priv, _ := generateTestKeypair(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- acc.CreateSessionTransport(ctx, priv, srv.URL)
	}()
	waitForSessionTransportSignal(t, requested, time.Second, "signal ticket request")
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("CreateSessionTransport after cancel = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateSessionTransport cancellation")
	}
	if st := acc.GetSessionTransport(); st != nil {
		t.Fatal("expected canceled session transport to be cleared")
	}
	assertNoUnexpectedSessionTransportPath(t, unexpectedPath)
}

func waitForSessionTransportSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func assertNoUnexpectedSessionTransportPath(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case path := <-ch:
		t.Fatalf("unexpected path: %s", path)
	default:
	}
}
