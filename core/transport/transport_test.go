package transport_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/routine"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestSessionTransportStartupTimeoutNamesStage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-releaseRequest:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(releaseRequest)
		server.Close()
	}()

	st, err := transport.NewSessionTransport(
		logrus.New().WithField("test", t.Name()),
		tb.Bus,
		privKey,
		server.URL,
		"",
		transport.WithStartupTimeout(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()

	select {
	case <-requestStarted:
	case <-ctx.Done():
		t.Fatalf("stalled signaling request did not start: %v", ctx.Err())
	}

	err = st.AwaitReady(ctx)
	if err == nil {
		t.Fatal("expected session transport startup error")
	}
	if !strings.Contains(err.Error(), "session transport did not become ready") {
		t.Fatalf("startup error does not name readiness failure: %v", err)
	}
	if !strings.Contains(err.Error(), "webrtc-controllers") {
		t.Fatalf("startup error does not name stalled stage: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session transport did not stop after startup timeout")
	}
	if st.GetChildBus() != nil {
		t.Fatal("expected transport to be nil after stop")
	}
}

func TestSessionTransportStartupBudgetStartsAtExecute(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()

	st, err := transport.NewSessionTransport(
		logrus.New().WithField("test", t.Name()),
		tb.Bus,
		privKey,
		server.URL,
		"",
		transport.WithStartupTimeout(100*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	executeDone := make(chan error, 1)
	go func() {
		executeDone <- st.Execute(ctx)
	}()
	select {
	case <-requestStarted:
	case <-ctx.Done():
		t.Fatalf("startup request did not begin: %v", ctx.Err())
	}
	select {
	case <-requestCanceled:
		if ctx.Err() != nil {
			t.Fatalf("parent context canceled before owner startup budget: %v", ctx.Err())
		}
	case <-ctx.Done():
		t.Fatalf("owner startup budget did not cancel Execute: %v", ctx.Err())
	}

	err = st.AwaitReady(ctx)
	if err == nil {
		t.Fatal("expected admitted startup timeout")
	}
	if !strings.Contains(err.Error(), "session transport did not become ready") {
		t.Fatalf("startup error = %v, want admitted timeout", err)
	}
	select {
	case executeErr := <-executeDone:
		if !errors.Is(executeErr, context.Canceled) {
			t.Fatalf("Execute returned %v after owner timeout, want %v", executeErr, context.Canceled)
		}
	case <-ctx.Done():
		t.Fatalf("Execute did not stop after owner timeout: %v", ctx.Err())
	}
}

func TestSessionTransportRetriesStartupAttemptWithoutRecreation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var ticketRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/signal/ticket":
			if ticketRequests.Add(1) == 1 {
				http.Error(w, "transient signaling failure", http.StatusServiceUnavailable)
				return
			}
			data, err := (&api.SignalTicketResponse{Token: "test-token"}).MarshalVT()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case r.Method == http.MethodGet && r.URL.Path == "/api/signal/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	st, err := transport.NewSessionTransport(
		logrus.New().WithField("test", t.Name()),
		tb.Bus,
		privKey,
		server.URL,
		"",
		transport.WithStartupTimeout(time.Second),
		transport.WithStartupRetry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	rc := routine.NewRoutineContainer(routine.WithBackoff(&cbackoff.ZeroBackOff{}))
	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)
	defer rc.ClearContext()

	if err := st.AwaitReady(ctx); err != nil {
		t.Fatalf("transport did not become ready after retry: %v", err)
	}
	if ticketRequests.Load() < 2 {
		t.Fatalf("startup retry did not make a second signaling request: %d", ticketRequests.Load())
	}
	if st.GetChildBus() == nil {
		t.Fatal("startup retry lost the transport child bus")
	}
}

func newTestSessionTransport(
	t *testing.T,
	signalingURL string,
	timeout time.Duration,
) (context.Context, context.CancelFunc, *testbed.Testbed, *transport.SessionTransport) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	tb, err := testbed.Default(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		cancel()
		tb.Release()
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(
		logrus.New().WithField("test", t.Name()),
		tb.Bus,
		privKey,
		signalingURL,
		"",
		transport.WithStartupTimeout(timeout),
	)
	if err != nil {
		cancel()
		tb.Release()
		t.Fatal(err)
	}
	return ctx, cancel, tb, st
}

func TestSessionTransportStartupFailurePublishesError(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		http.Error(w, "test rejection", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel, tb, st := newTestSessionTransport(t, server.URL, time.Second)
	defer func() {
		cancel()
		tb.Release()
	}()

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()
	select {
	case <-requestStarted:
	case <-ctx.Done():
		t.Fatalf("stalled signaling request did not start: %v", ctx.Err())
	}

	err := st.AwaitReady(ctx)
	if err == nil {
		t.Fatal("expected session transport startup error")
	}
	if !strings.Contains(err.Error(), "session transport failed to start at webrtc-controllers") {
		t.Fatalf("startup error does not name failed stage: %v", err)
	}
	select {
	case executeErr := <-done:
		if executeErr == nil {
			t.Fatal("expected Execute to return the startup error")
		}
	case <-ctx.Done():
		t.Fatalf("session transport did not publish startup failure: %v", ctx.Err())
	}
}

func TestSessionTransportStartupTimeoutBeforeExecute(t *testing.T) {
	ctx, cancel, tb, st := newTestSessionTransport(t, "", 20*time.Millisecond)
	defer func() {
		cancel()
		tb.Release()
	}()

	err := st.AwaitReady(ctx)
	if err == nil {
		t.Fatal("expected startup timeout before Execute")
	}
	if !strings.Contains(err.Error(), "session transport did not become ready") {
		t.Fatalf("startup error does not name readiness failure: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()
	select {
	case executeErr := <-done:
		if executeErr != context.Canceled {
			t.Fatalf("Execute returned %v after timeout admission, want %v", executeErr, context.Canceled)
		}
	case <-ctx.Done():
		t.Fatalf("Execute did not observe timeout admission: %v", ctx.Err())
	}
}

func TestSessionTransportReadyWinsStartupTimeout(t *testing.T) {
	ctx, cancel, tb, st := newTestSessionTransport(t, "", time.Second)
	defer func() {
		cancel()
		tb.Release()
	}()

	readyCh := st.Ready()
	select {
	case <-readyCh:
		t.Fatal("Ready closed before startup completed")
	default:
	}

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatalf("AwaitReady returned startup error: %v", err)
	}
	select {
	case <-readyCh:
	default:
		t.Fatal("Ready remained open after AwaitReady succeeded")
	}
	cancel()
	select {
	case executeErr := <-done:
		if executeErr != context.Canceled {
			t.Fatalf("Execute returned %v after cancellation, want %v", executeErr, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("session transport did not stop after cancellation")
	}
}

func TestSessionTransportRepeatedWaitersObserveReady(t *testing.T) {
	ctx, cancel, tb, st := newTestSessionTransport(t, "", time.Second)
	defer func() {
		cancel()
		tb.Release()
	}()

	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()
	const waiterCount = 8
	results := make(chan error, waiterCount)
	for range waiterCount {
		go func() {
			results <- st.AwaitReady(ctx)
		}()
	}
	for range waiterCount {
		if err := <-results; err != nil {
			t.Fatalf("repeated waiter returned startup error: %v", err)
		}
	}
	cancel()
	select {
	case executeErr := <-done:
		if executeErr != context.Canceled {
			t.Fatalf("Execute returned %v after cancellation, want %v", executeErr, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("session transport did not stop after cancellation")
	}
}
