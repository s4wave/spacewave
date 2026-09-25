package provider_local

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/session"

	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func newPairingTransportAccount(ctx context.Context, t *testing.T) (*ProviderAccount, crypto.PrivKey, func()) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	acc := &ProviderAccount{
		t:            &providerAccountTracker{p: &Provider{b: tb.Bus}},
		le:           logrus.New().WithField("test", t.Name()),
		lifecycleCtx: ctx,
	}
	release := func() {
		acc.StopSessionTransport()
		tb.Release()
	}
	return acc, privKey, release
}

func startTestSessionTransport(
	ctx context.Context,
	t *testing.T,
	acc *ProviderAccount,
	sessionKey crypto.PrivKey,
	signalingURL string,
) *sessionTransportState {
	t.Helper()
	st, err := transport.NewSessionTransport(
		acc.le,
		acc.t.p.b,
		sessionKey,
		signalingURL,
		"",
		transport.WithStartupRetry(),
	)
	if err != nil {
		t.Fatal(err)
	}
	rc := routine.NewRoutineContainer(routine.WithRetry(providerBackoff))
	sts := &sessionTransportState{transport: st, rc: rc}
	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = sts
		bcast()
	})
	return sts
}

func pairingEngineForTest(t *testing.T, sess session.Session) *pairing.Engine {
	t.Helper()
	engine, err := sess.(pairing.Session).GetPairingEngine()
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func waitForPairingStatus(ctx context.Context, t *testing.T, engine *pairing.Engine, status pairing.Status) pairing.Snapshot {
	t.Helper()
	for {
		snapshot, wait := engine.Snapshot()
		if snapshot.Status == status {
			return snapshot
		}
		if snapshot.Status == pairing.StatusFailed {
			t.Fatalf("pairing failed: %s", snapshot.ErrMsg)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("pairing did not reach %v: %s", status, snapshot.ErrMsg)
		case <-wait:
		}
	}
}

// TestUnauthorizedSignalingStopsSessionTransport checks that a rejected
// session identity ends the transport and removes it from the account.
func TestUnauthorizedSignalingStopsSessionTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown session", http.StatusUnauthorized)
	}))
	defer server.Close()

	sts, _, err := acc.ensureSessionTransport(ctx, sessionKey, server.URL, "")
	if sts == nil {
		t.Fatalf("ensureSessionTransport returned no transport state: %v", err)
	}
	for {
		var exited bool
		var exitErr error
		var waitCh <-chan struct{}
		sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			exited, exitErr = sts.exited, sts.err
			waitCh = getWaitCh()
		})
		if exited {
			var statusErr interface{ StatusCode() int }
			if !errors.As(exitErr, &statusErr) || statusErr.StatusCode() != http.StatusUnauthorized {
				t.Fatalf("transport exited with %v, want the unauthorized ticket error", exitErr)
			}
			break
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			t.Fatalf("transport did not exit after the ticket was rejected: %v", ctx.Err())
		}
	}
	if acc.GetSessionTransport() != nil {
		t.Fatal("failed transport remained current")
	}
}

func TestTransportStartupCancellationReturnsCancellation(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	transportCtx, transportCancel := context.WithCancel(ctx)
	defer transportCancel()
	sts := startTestSessionTransport(transportCtx, t, acc, sessionKey, "")

	waitCtx, waitCancel := context.WithCancel(ctx)
	waitCancel()
	if err := acc.waitSessionTransportReady(waitCtx, sts); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup cancellation returned %v", err)
	}
}

func TestSupersededTransportStartupReturnsSuperseded(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	transportCtx, transportCancel := context.WithCancel(ctx)
	defer transportCancel()
	sts := startTestSessionTransport(transportCtx, t, acc, sessionKey, "")
	sts.setReplaced()

	err := acc.waitSessionTransportReady(ctx, sts)
	if !errors.Is(err, errSessionTransportSuperseded) {
		t.Fatalf("superseded startup returned %v", err)
	}
	acc.stopSessionTransportState(sts)
}

func TestCreateSessionTransportCancellationAfterReadyRecreatesCurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	sts, err := acc.createSessionTransport(ctx, sessionKey, "")
	if err != nil {
		t.Fatalf("createSessionTransport: %v", err)
	}
	var stateWaitCh, providerWaitCh <-chan struct{}
	sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		stateWaitCh = getWaitCh()
	})
	acc.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		providerWaitCh = getWaitCh()
	})

	cancel()
	select {
	case <-stateWaitCh:
	case <-time.After(time.Second):
		t.Fatal("ready transport did not publish exit after owner cancellation")
	}
	select {
	case <-providerWaitCh:
	case <-time.After(time.Second):
		t.Fatal("ready transport was not removed after owner cancellation")
	}

	newCtx, newCancel := context.WithTimeout(context.Background(), time.Second)
	defer newCancel()
	if err := acc.EnsureSessionTransport(newCtx, sessionKey, ""); err != nil {
		t.Fatalf("EnsureSessionTransport after cancellation: %v", err)
	}
	current := acc.GetSessionTransport()
	if current == nil || current == sts.transport {
		t.Fatal("expected cancellation to recreate the current transport")
	}
}

func TestSessionTransportReadyCommitRejectsReplacement(t *testing.T) {
	sts := &sessionTransportState{}
	sts.setReplaced()

	if sts.setReady() {
		t.Fatal("replaced transport committed readiness")
	}
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if sts.ready {
			t.Fatal("replaced transport recorded readiness")
		}
	})
}

// TestCreateSessionTransportOutlivesCaller proves that a session mounted by an
// enrollment RPC does not bind its transport to the RPC deadline.
func TestCreateSessionTransportOutlivesCaller(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	callerCtx, cancelCaller := context.WithCancel(ctx)
	if err := acc.CreateSessionTransport(callerCtx, sessionKey, ""); err != nil {
		t.Fatal(err)
	}
	cancelCaller()

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for {
		var current *sessionTransportState
		var changed <-chan struct{}
		acc.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			current = acc.sessionTransport
			changed = getWaitCh()
		})
		if current == nil {
			t.Fatal("session transport stopped with its mounting caller")
		}
		select {
		case <-changed:
		case <-timer.C:
			return
		}
	}
}

// TestEnsureConfiguredSessionTransportOutlivesCaller proves an enrollment RPC
// follows the session tracker's transport rather than creating an RPC-owned
// replacement when the requested signaling configuration differs.
func TestEnsureConfiguredSessionTransportOutlivesCaller(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	callerCtx, cancelCaller := context.WithCancel(ctx)
	if err := acc.EnsureConfiguredSessionTransport(callerCtx, sessionKey); err != nil {
		t.Fatal(err)
	}
	cancelCaller()

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for {
		var current *sessionTransportState
		var changed <-chan struct{}
		acc.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			current = acc.sessionTransport
			changed = getWaitCh()
		})
		if current == nil {
			t.Fatal("configured transport stopped with its caller")
		}
		select {
		case <-changed:
		case <-timer.C:
			return
		}
	}
}

func TestEnsureConfiguredSessionTransportDoesNotReplaceMountedTransport(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()
	acc.t.p.signalingURL = "https://spacewave.app"
	if err := acc.CreateSessionTransport(ctx, sessionKey, ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()
	mounted := acc.GetSessionTransport()
	if mounted == nil {
		t.Fatal("expected mounted session transport")
	}
	if err := acc.EnsureConfiguredSessionTransport(ctx, sessionKey); err != nil {
		t.Fatal(err)
	}
	if got := acc.GetSessionTransport(); got != mounted {
		t.Fatal("configured ensure replaced the mounted session transport")
	}
}
