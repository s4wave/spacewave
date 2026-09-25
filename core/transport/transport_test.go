package transport_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	websocket "github.com/aperturerobotics/go-websocket"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/routine"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	transport_webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// signalTicketStatusError matches the HTTP status of a rejected ticket request.
type signalTicketStatusError interface {
	StatusCode() int
}

func newTestSessionTransport(
	t *testing.T,
	signalingURL string,
	opts ...transport.SessionTransportOption,
) (context.Context, *testbed.Testbed, *transport.SessionTransport) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(
		logrus.New().WithField("test", t.Name()),
		tb.Bus,
		privKey,
		signalingURL,
		"",
		opts...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, tb, st
}

// TestSessionTransportReadyWithStalledSignaling checks that readiness covers
// only local controllers: a signal ticket request that never answers does not
// delay it.
func TestSessionTransportReadyWithStalledSignaling(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, _, st := newTestSessionTransport(t, server.URL)
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- st.Execute(ctx)
	}()

	if err := st.AwaitReady(ctx); err != nil {
		t.Fatalf("AwaitReady returned %v while signaling stalled", err)
	}
	select {
	case <-requestStarted:
	case <-ctx.Done():
		t.Fatalf("signaling did not request a ticket: %v", ctx.Err())
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute returned %v after cancellation, want %v", err, context.Canceled)
	}
	if st.GetChildBus() != nil {
		t.Fatal("child bus remained after Execute returned")
	}
}

// TestSessionTransportSignalingRetriesTicket checks that signaling retries a
// failed ticket request in the background and then connects.
func TestSessionTransportSignalingRetriesTicket(t *testing.T) {
	var ticketRequests atomic.Int32
	wsAccepted := make(chan struct{}, 1)
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
			_, _ = w.Write(data)
		case r.Method == http.MethodGet && r.URL.Path == "/api/signal/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			select {
			case wsAccepted <- struct{}{}:
			default:
			}
			<-conn.CloseRead(r.Context()).Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, _, st := newTestSessionTransport(t, server.URL, transport.WithStartupRetry())
	rc := routine.NewRoutineContainer(routine.WithBackoff(&cbackoff.ZeroBackOff{}))
	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)
	defer rc.ClearContext()

	if err := st.AwaitReady(ctx); err != nil {
		t.Fatalf("transport did not become ready: %v", err)
	}
	select {
	case <-wsAccepted:
	case <-ctx.Done():
		t.Fatalf("signaling did not connect after a failed ticket request: %v", ctx.Err())
	}
	if n := ticketRequests.Load(); n < 2 {
		t.Fatalf("signaling made %d ticket requests, want at least 2", n)
	}
}

// TestSessionTransportUnauthorizedTicketFailsTransport checks that a rejected
// session identity ends the transport with the ticket's unauthorized error.
func TestSessionTransportUnauthorizedTicketFailsTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown session", http.StatusUnauthorized)
	}))
	defer server.Close()

	for _, retry := range []bool{false, true} {
		t.Run("retry="+strconv.FormatBool(retry), func(t *testing.T) {
			var opts []transport.SessionTransportOption
			if retry {
				opts = append(opts, transport.WithStartupRetry())
			}
			ctx, _, st := newTestSessionTransport(t, server.URL, opts...)

			var statusErr signalTicketStatusError
			executeErr := st.Execute(ctx)
			if retry && executeErr != nil {
				t.Fatalf("retrying Execute returned %v, want nil after failure", executeErr)
			}
			if !retry && (!errors.As(executeErr, &statusErr) || statusErr.StatusCode() != http.StatusUnauthorized) {
				t.Fatalf("Execute returned %v, want the unauthorized ticket error", executeErr)
			}
			if err := st.Err(); !errors.As(err, &statusErr) || statusErr.StatusCode() != http.StatusUnauthorized {
				t.Fatalf("Err returned %v, want the unauthorized ticket error", err)
			}
			if err := st.AwaitReady(ctx); !errors.As(err, &statusErr) {
				t.Fatalf("AwaitReady returned %v, want the unauthorized ticket error", err)
			}
			if st.GetChildBus() != nil {
				t.Fatal("child bus remained after the transport failed")
			}
			if err := st.Execute(ctx); retry != (err == nil) {
				t.Fatalf("Execute after failure returned %v", err)
			}
		})
	}
}

func TestSessionTransportReadyClosesReady(t *testing.T) {
	ctx, _, st := newTestSessionTransport(t, "")
	ctx, cancel := context.WithCancel(ctx)

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
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute returned %v after cancellation, want %v", err, context.Canceled)
	}
}

func TestSessionTransportRepeatedWaitersObserveReady(t *testing.T) {
	ctx, _, st := newTestSessionTransport(t, "")
	ctx, cancel := context.WithCancel(ctx)

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
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute returned %v after cancellation, want %v", err, context.Canceled)
	}
}

type establishLinkSpy struct {
	count      atomic.Int32
	loadCount  atomic.Int32
	bridgedSig chan struct{}
}

func (s *establishLinkSpy) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch di.GetDirective().(type) {
	case link.EstablishLinkWithPeer:
		s.count.Add(1)
	case resolver.LoadControllerWithConfig:
		if _, ok := di.GetDirective().(resolver.LoadControllerWithConfig).GetLoadControllerConfig().(*transport_webrtc.Config); ok {
			s.loadCount.Add(1)
		}
	case link_solicit.SolicitProtocol:
		select {
		case s.bridgedSig <- struct{}{}:
		default:
		}
	}
	return nil, nil
}

func (*establishLinkSpy) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/establish-link-spy", controller.MustParseVersion("0.0.1"), "establish link spy")
}

func (*establishLinkSpy) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (*establishLinkSpy) Close() error { return nil }

// TestSessionTransportKeepsProtocolsOnChildBus prevents session protocol
// controllers from splitting attached values across duplicate parent directives.
func TestSessionTransportKeepsProtocolsOnChildBus(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	spy := &establishLinkSpy{bridgedSig: make(chan struct{}, 1)}
	if _, err := tb.Bus.AddController(ctx, spy, nil); err != nil {
		t.Fatal(err)
	}
	localKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remoteKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	localID, err := peer.IDFromPrivateKey(localKey)
	if err != nil {
		t.Fatal(err)
	}
	remoteID, err := peer.IDFromPrivateKey(remoteKey)
	if err != nil {
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(logrus.NewEntry(logrus.New()), tb.Bus, localKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- st.Execute(ctx) }()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	_, ref, err := st.GetChildBus().AddDirective(link.NewEstablishLinkWithPeer(localID, remoteID), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	_, bridgeRef, err := st.GetChildBus().AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("test/bridge"), nil, "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bridgeRef.Release()
	_, loadRef, err := st.GetChildBus().AddDirective(
		resolver.NewLoadControllerWithConfig(&transport_webrtc.Config{}), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer loadRef.Release()
	select {
	case <-spy.bridgedSig:
		t.Fatal("parent bus observed a session SolicitProtocol directive")
	case <-time.After(100 * time.Millisecond):
	}
	if got := spy.count.Load(); got != 0 {
		t.Fatalf("parent bus observed %d EstablishLinkWithPeer directives", got)
	}
	if got := spy.loadCount.Load(); got != 0 {
		t.Fatalf("parent bus observed %d session controller load directives", got)
	}
	cancel()
	<-done
}
