//go:build !goscript

package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// These tests exercise the native session transport lifecycle, whose WebRTC
// signaling controller reaches the signal-ticket HTTP endpoint. The goscript
// browser build replaces that selector with a no-op (browser sessions obtain
// transport from the web runtime), so these tests only apply to the native
// build.

// TestCreateSessionTransportCancellationAfterReadyClearsCurrentTransport checks
// that cancellation publishes exit and removes the ready transport.
func TestCreateSessionTransportCancellationAfterReadyClearsCurrentTransport(t *testing.T) {
	// Start a ready transport under a cancelable lifetime.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	priv, _ := generateTestKeypair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := acc.CreateSessionTransport(ctx, priv, ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}

	// Subscribe to both transport exit and account removal before cancellation.
	var sts *sessionTransportState
	acc.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = acc.sessionTransports[""]
	})
	if sts == nil {
		t.Fatal("expected ready session transport state")
	}
	var stateWaitCh <-chan struct{}
	sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		stateWaitCh = getWaitCh()
	})
	running, transportWaitCh := acc.GetTransportSnapshotWithWait()
	if !running {
		t.Fatal("expected transport snapshot to report running")
	}

	// Cancel the transport and require both lifecycle notifications.
	cancel()
	select {
	case <-stateWaitCh:
	case <-time.After(time.Second):
		t.Fatal("transport exit was not published")
	}
	select {
	case <-transportWaitCh:
	case <-time.After(time.Second):
		t.Fatal("transport removal was not published")
	}

	// Confirm that the notifications describe the settled state.
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !sts.exited {
			t.Fatal("expected canceled ready transport to publish exit")
		}
	})
	if st := acc.GetSessionTransport(); st != nil {
		t.Fatal("expected canceled ready session transport to be cleared")
	}
	running, _ = acc.GetTransportSnapshotWithWait()
	if running {
		t.Fatal("expected canceled transport snapshot to report not running")
	}
}

// testSessionTransportStatusError exposes HTTP status independently of its text.
type testSessionTransportStatusError struct {
	// statusCode is the response status preserved by wrappers.
	statusCode int
}

// Error returns text that does not identify the response status.
func (e *testSessionTransportStatusError) Error() string {
	return "transport message changed"
}

// StatusCode returns the response status used for classification.
func (e *testSessionTransportStatusError) StatusCode() int {
	return e.statusCode
}

// TestMountedSessionReRegistersAfterUnauthorizedTransport checks that a mounted
// Session renews its registration and transport after the cloud rejects its
// signal ticket, even when presentation metadata cannot be mirrored.
func TestMountedSessionReRegistersAfterUnauthorizedTransport(t *testing.T) {
	// Control the first rejected ticket and the replacement ticket independently.
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()

	firstTicketStarted := make(chan struct{})
	releaseFirstTicket := make(chan struct{})
	firstRegistration := make(chan struct{})
	reregistration := make(chan struct{})
	secondTicketStarted := make(chan struct{})
	permitSecondTicket := make(chan struct{})
	websocketAccepted := make(chan struct{})
	var websocketAcceptedOnce sync.Once
	var registrations atomic.Int32
	var tickets atomic.Int32

	// Serve registration and signaling while leaving presentation endpoints absent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/account/session/register":
			switch registrations.Add(1) {
			case 1:
				close(firstRegistration)
			case 2:
				close(reregistration)
			}
			response, err := (&api.RegisterSessionResponse{
				AccountId:        "test-account",
				ObservedMetadata: &api.ObservedSessionMetadata{Label: "test"},
			}).MarshalVT()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(response)
		case "/api/signal/ticket":
			ticket := tickets.Add(1)
			switch ticket {
			case 1:
				close(firstTicketStarted)
				<-releaseFirstTicket
				http.Error(w, "wrapped unauthorized response", http.StatusUnauthorized)
				return
			case 2:
				close(secondTicketStarted)
				<-permitSecondTicket
			default:
				http.Error(w, "unexpected extra ticket", http.StatusInternalServerError)
				return
			}
			response, err := (&api.SignalTicketResponse{Token: "test-token"}).MarshalVT()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(response)
		case "/api/signal/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			websocketAcceptedOnce.Do(func() { close(websocketAccepted) })
			<-conn.CloseRead(r.Context()).Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Mount the real Session controller in the in-memory stack.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	_, controllerRef, err := tb.Bus.AddDirective(
		resolver.NewLoadControllerWithConfig(&session_controller.Config{
			VolumeId: tb.Volume.GetID(),
		}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controllerRef.Release()
	sessionController, sessionControllerRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionControllerRef.Release()

	// Register the Session reference consumed by MountSession.
	priv, pid := generateTestKeypair(t)
	sessionID := "session-401"
	sessionRef := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                sessionID,
			ProviderId:        "spacewave",
			ProviderAccountId: "test-account",
		},
	}
	if _, err := sessionController.RegisterSession(ctx, sessionRef, &session.SessionMetadata{}); err != nil {
		t.Fatal(err)
	}

	// Let the keyed container own the tracker and observe its actual exit.
	le := logrus.New().WithField("test", t.Name())
	prov := NewProvider(le, tb.Bus, &Config{Endpoint: srv.URL}, NewProviderInfo("spacewave"), nil, nil)
	acc := &ProviderAccount{
		le:        le,
		p:         prov,
		accountID: "test-account",
		vol:       tb.Volume,
		conf:      prov.conf,
		sfs:       prov.sfs,
		soListCtr: ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
		entityCli: NewEntityClientDirect(prov.httpCli, srv.URL, DefaultSigningEnvPrefix, priv, pid),
	}
	executing := make(chan error, 1)
	acc.sessions = keyed.NewKeyedRefCount(acc.buildSessionTracker,
		keyed.WithExitCb[string, *sessionTracker](func(_ string, _ keyed.Routine, _ *sessionTracker, err error) {
			executing <- err
		}),
	)
	acc.sessions.SetContext(ctx, false)
	defer acc.sessions.ClearContext()

	// Mount through the production API without launching a second tracker.
	mounted, mountedRelease, err := acc.MountSession(ctx, sessionRef, nil)
	if err != nil {
		t.Fatalf("mount Session: %v", err)
	}
	defer mountedRelease()
	if mounted == nil {
		t.Fatal("mounted Session is nil")
	}

	// Reject the initial transport only after initial registration is observed.
	select {
	case <-firstTicketStarted:
	case <-ctx.Done():
		t.Fatalf("first transport ticket did not start: %v", ctx.Err())
	}
	select {
	case <-firstRegistration:
	case <-ctx.Done():
		t.Fatalf("initial Session registration was not observed: %v", ctx.Err())
	}
	close(releaseFirstTicket)

	// Permit replacement signaling after the Session re-registers.
	select {
	case <-reregistration:
	case <-ctx.Done():
		t.Fatalf("Session did not re-register after HTTP 401: %v", ctx.Err())
	}
	close(permitSecondTicket)
	select {
	case <-secondTicketStarted:
	case <-ctx.Done():
		t.Fatalf("second transport ticket did not start: %v", ctx.Err())
	}
	select {
	case <-websocketAccepted:
	case <-ctx.Done():
		t.Fatalf("replacement signaling socket was not accepted: %v", ctx.Err())
	}

	// Require a recovered transport and the three native signaling ticket roles.
	for {
		snapshot, waitCh := acc.GetTransportCompositionSnapshotWithWait(sessionID)
		if snapshot.P2PState == TransportCompositionP2PStateError {
			t.Fatalf("mounted Session transport remained in error state: %s", snapshot.LastError)
		}
		if snapshot.P2PState == TransportCompositionP2PStateNoPeers ||
			snapshot.P2PState == TransportCompositionP2PStateIdle ||
			snapshot.P2PState == TransportCompositionP2PStateActive {
			break
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			t.Fatalf("mounted Session transport did not become live: %v", ctx.Err())
		}
	}
	if got := registrations.Load(); got < 2 {
		t.Fatalf("session registrations = %d, want initial registration and re-registration", got)
	}
	if got := tickets.Load(); got != 2 {
		t.Fatalf("signal tickets = %d, want the rejected ticket and the replacement connection", got)
	}

	// Join the container-owned tracker after canceling its lifetime.
	cancel()
	select {
	case err := <-executing:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("session tracker exit = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session tracker did not exit after test cancellation")
	}
}

// TestSessionTransportReplacementReportsUncooperativeRoutine checks that a stop
// cannot remove a transport whose routine has not exited.
func TestSessionTransportReplacementReportsUncooperativeRoutine(t *testing.T) {
	// Hold the old transport routine past cancellation until explicitly released.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	acc := NewTestProviderAccount(t, "")
	acc.sessionTransports = make(map[string]*sessionTransportState)

	started := make(chan struct{})
	releaseRoutine := make(chan struct{})
	routineExited := make(chan struct{})
	rc := routine.NewRoutineContainer()
	rc.SetRoutine(func(context.Context) error {
		close(started)
		<-releaseRoutine
		close(routineExited)
		return nil
	})
	rc.SetContext(ctx, false)
	sts := &sessionTransportState{
		sessionID: "",
		rc:        rc,
		readyRc:   routine.NewRoutineContainer(),
	}
	acc.sessionTransports[""] = sts

	// Attempt replacement while the old routine is still running.
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("old routine did not start: %v", ctx.Err())
	}
	stopCtx, stopCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err := acc.stopSessionTransportForSession(stopCtx, "", sts)
	stopCancel()
	if err == nil {
		t.Fatal("replacement returned nil while old routine ignored cancellation")
	}
	if got := acc.sessionTransports[""]; got != sts {
		t.Fatal("replacement removed current state after an unconfirmed stop")
	}

	// Release and join the deliberately uncooperative routine.
	close(releaseRoutine)
	select {
	case <-routineExited:
	case <-ctx.Done():
		t.Fatalf("old routine did not exit after release: %v", ctx.Err())
	}
}

// TestSessionTransportUnauthorizedUsesStatusThroughWrappedMessage checks status
// classification independently of wrapper text.
func TestSessionTransportUnauthorizedUsesStatusThroughWrappedMessage(t *testing.T) {
	err := errors.Wrap(&testSessionTransportStatusError{
		statusCode: http.StatusUnauthorized,
	}, "message changed")
	if !sessionTransportUnauthorized(err) {
		t.Fatalf("classification lost unauthorized status through wrapped message: %v", err)
	}
}

// TestCreateSessionTransportConcurrentReplacementKeepsNewTransport checks that
// an old startup cannot clear its ready replacement.
func TestCreateSessionTransportConcurrentReplacementKeepsNewTransport(t *testing.T) {
	// Hold the old transport in startup until its replacement cancels it.
	acc := NewTestProviderAccount(t, "")
	gate := newStartupGate()
	acc.p.transportOptions = []transport.SessionTransportOption{gate.option()}
	oldPriv, _ := generateTestKeypair(t)
	oldDone := make(chan error, 1)
	go func() {
		oldDone <- acc.CreateSessionTransport(context.Background(), oldPriv, "")
	}()
	waitForSessionTransportSignal(t, gate.started, time.Second, "old transport startup")

	// Replace it with a ready transport carrying a different peer identity.
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

	// Join old startup and confirm that its cleanup preserves the replacement.
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
}

// TestCreateSessionTransportCancellationStopsStartup checks that cancellation
// stops a pending startup and clears its transport.
func TestCreateSessionTransportCancellationStopsStartup(t *testing.T) {
	// Hold startup until its context is canceled.
	acc := NewTestProviderAccount(t, "")
	gate := newStartupGate()
	acc.p.transportOptions = []transport.SessionTransportOption{gate.option()}
	priv, _ := generateTestKeypair(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- acc.CreateSessionTransport(ctx, priv, "")
	}()
	waitForSessionTransportSignal(t, gate.started, time.Second, "transport startup")
	cancel()

	// Join startup and require the canceled transport to be removed.
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
}

// startupGate holds the first session transport that starts through it in
// startup until its context ends. Later transports start without waiting.
type startupGate struct {
	started chan struct{}
	first   atomic.Bool
}

func newStartupGate() *startupGate {
	return &startupGate{started: make(chan struct{})}
}

// option returns the local transport option that waits on the gate.
func (g *startupGate) option() transport.SessionTransportOption {
	return transport.WithLocalTransport(func(ctx context.Context, _ *logrus.Entry, _ bus.Bus, _ peer.ID) (*transport_controller.Controller, func(), error) {
		if g.first.Swap(true) {
			return nil, func() {}, nil
		}
		close(g.started)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
}

// waitForSessionTransportSignal waits for a fixture event within the test bound.
func waitForSessionTransportSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}
