//go:build !goscript

package provider_spacewave

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// These tests exercise the native WebRTC session transport startup lifecycle,
// which reaches the signal-ticket HTTP endpoint through startWebRTCControllers.
// The goscript browser build replaces that selector with a no-op (browser
// sessions obtain transport from the web runtime), so the signal-ticket
// lifecycle does not exist there and these tests only apply to the native build.

func TestCreateSessionTransportCancellationAfterReadyClearsCurrentTransport(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	priv, _ := generateTestKeypair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := acc.CreateSessionTransport(ctx, priv, ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}

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

type testSessionTransportStatusError struct {
	statusCode int
}

func (e *testSessionTransportStatusError) Error() string {
	return "transport message changed"
}

func (e *testSessionTransportStatusError) StatusCode() int {
	return e.statusCode
}

func TestMountedSessionReRegistersAfterUnauthorizedTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()

	firstTicketStarted := make(chan struct{})
	releaseFirstTicket := make(chan struct{})
	firstRegistration := make(chan struct{})
	reregistration := make(chan struct{})
	secondTicketStarted := make(chan struct{})
	permitSecondTicket := make(chan struct{})
	var registrations atomic.Int32
	var tickets atomic.Int32

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
			switch tickets.Add(1) {
			case 1:
				close(firstTicketStarted)
				<-releaseFirstTicket
				http.Error(w, "wrapped unauthorized response", http.StatusUnauthorized)
			case 2:
				close(secondTicketStarted)
				<-permitSecondTicket
				response, err := (&api.SignalTicketResponse{Token: "test-token"}).MarshalVT()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(response)
			default:
				http.Error(w, "unexpected extra ticket", http.StatusInternalServerError)
			}
		case "/api/signal/ws":
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
	defer srv.Close()

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
	acc.sessions = keyed.NewKeyedRefCount(acc.buildSessionTracker)
	acc.sessions.SetContext(ctx, false)
	defer acc.sessions.ClearContext()
	_, tkr, existed := acc.sessions.AddKeyRef(sessionID)
	if existed {
		t.Fatal("expected mounted Session tracker to be created")
	}
	defer acc.sessions.RemoveKey(sessionID)
	tkr.ref.SetResult(sessionRef, nil)

	executing := make(chan error, 1)
	go func() {
		executing <- tkr.executeSessionTracker(ctx)
	}()

	mounted, mountedRelease, err := acc.MountSession(ctx, sessionRef, nil)
	if err != nil {
		t.Fatalf("mount Session: %v", err)
	}
	defer mountedRelease()
	if mounted == nil {
		t.Fatal("mounted Session is nil")
	}

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
		t.Fatalf("signal tickets = %d, want two", got)
	}

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

func TestSessionTransportReplacementReportsUncooperativeRoutine(t *testing.T) {
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

	close(releaseRoutine)
	select {
	case <-routineExited:
	case <-ctx.Done():
		t.Fatalf("old routine did not exit after release: %v", ctx.Err())
	}
}

func TestClassifySessionTransportErrorUsesStatusThroughWrappedMessage(t *testing.T) {
	err := fmt.Errorf("message changed: %w", &testSessionTransportStatusError{
		statusCode: http.StatusUnauthorized,
	})
	if !errors.Is(classifySessionTransportError(err), errSessionTransportUnauthorized) {
		t.Fatalf("classification lost unauthorized status through wrapped message: %v", err)
	}
}

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
