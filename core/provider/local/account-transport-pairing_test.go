package provider_local

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
		t:  &providerAccountTracker{p: &Provider{b: tb.Bus}},
		le: logrus.New().WithField("test", t.Name()),
	}
	acc.setPairingContext(ctx)
	release := func() {
		acc.StopSessionTransport()
		acc.ClearPairingState()
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
	startupTimeout time.Duration,
) *sessionTransportState {
	t.Helper()
	st, err := transport.NewSessionTransport(
		acc.le,
		acc.t.p.b,
		sessionKey,
		signalingURL,
		"",
		transport.WithStartupTimeout(startupTimeout),
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

func waitForPairingStatus(
	ctx context.Context,
	t *testing.T,
	acc *ProviderAccount,
	status PairingStatus,
) PairingSnapshot {
	t.Helper()
	for {
		var (
			waitCh <-chan struct{}
			snap   PairingSnapshot
		)
		acc.GetPairingBroadcast().HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			snap = acc.GetPairingSnapshot()
		})
		if snap.Status == status {
			return snap
		}
		select {
		case <-ctx.Done():
			t.Fatalf("pairing status did not become %d: %v", status, ctx.Err())
		case <-waitCh:
		}
	}
}

func TestTerminalTransportStartupFailurePublishesPairingStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	var ticketRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/signal/ticket" {
			ticketRequests.Add(1)
		}
		http.Error(w, "terminal test signaling failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	acc.SetPairingCode("TESTCODE", sessionKey)
	sts := startTestSessionTransport(ctx, t, acc, sessionKey, server.URL, 1500*time.Millisecond)
	err := acc.waitSessionTransportReady(ctx, sts)
	if err == nil {
		t.Fatal("expected terminal startup failure")
	}
	if ticketRequests.Load() < 2 {
		t.Fatalf("startup retry did not retain attempt ownership: %d requests", ticketRequests.Load())
	}

	snap := waitForPairingStatus(ctx, t, acc, PairingStatusSignalingFailed)
	if snap.ErrMsg != err.Error() {
		t.Fatalf("pairing error %q != startup error %q", snap.ErrMsg, err)
	}
}

func TestTransportStartupCancellationDoesNotPublishPairingFailure(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	acc.SetPairingCode("TESTCODE", sessionKey)
	transportCtx, transportCancel := context.WithCancel(ctx)
	defer transportCancel()
	sts := startTestSessionTransport(transportCtx, t, acc, sessionKey, "", time.Second)

	waitCtx, waitCancel := context.WithCancel(ctx)
	waitCancel()
	if err := acc.waitSessionTransportReady(waitCtx, sts); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup cancellation returned %v", err)
	}
	snap := acc.GetPairingSnapshot()
	if snap.Status == PairingStatusSignalingFailed {
		t.Fatalf("caller cancellation published signaling failure: %q", snap.ErrMsg)
	}
}

func TestSupersededTransportStartupDoesNotPublishPairingFailure(t *testing.T) {
	ctx := t.Context()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	acc.SetPairingCode("TESTCODE", sessionKey)
	transportCtx, transportCancel := context.WithCancel(ctx)
	defer transportCancel()
	sts := startTestSessionTransport(transportCtx, t, acc, sessionKey, "", time.Second)
	sts.setReplaced()

	err := acc.waitSessionTransportReady(ctx, sts)
	if !errors.Is(err, errSessionTransportSuperseded) {
		t.Fatalf("superseded startup returned %v", err)
	}
	acc.stopSessionTransportState(sts)
	snap := acc.GetPairingSnapshot()
	if snap.Status == PairingStatusSignalingFailed {
		t.Fatalf("supersession published signaling failure: %q", snap.ErrMsg)
	}
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
