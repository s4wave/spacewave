package provider_spacewave

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/transport"
)

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.observed)
	})
	return c.Context.Done()
}

func TestCreateSessionTransportPublishesReadyAndStops(t *testing.T) {
	// Start the initial session transport.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	priv, _ := generateTestKeypair(t)
	ctx := t.Context()

	if err := acc.CreateSessionTransport(ctx, priv, ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}
	if st := acc.GetSessionTransport(); st == nil {
		t.Fatal("expected ready session transport to be published")
	}

	// Verify publication and stop cleanup.
	running, _ := acc.GetTransportSnapshotWithWait()
	if !running {
		t.Fatal("expected transport snapshot to report running")
	}

	acc.StopSessionTransport()
	if st := acc.GetSessionTransport(); st != nil {
		t.Fatal("expected stopped session transport to be cleared")
	}
	running, _ = acc.GetTransportSnapshotWithWait()
	if running {
		t.Fatal("expected stopped transport snapshot to report not running")
	}
}

func TestCreateSessionTransportReplacesExisting(t *testing.T) {
	// Start the first transport for replacement.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	ctx := t.Context()

	firstPriv, _ := generateTestKeypair(t)
	if err := acc.CreateSessionTransport(ctx, firstPriv, ""); err != nil {
		t.Fatalf("first CreateSessionTransport: %v", err)
	}
	first := acc.GetSessionTransport()
	if first == nil {
		t.Fatal("expected first session transport")
	}

	// Start a second transport and verify replacement state.
	secondPriv, _ := generateTestKeypair(t)
	if err := acc.CreateSessionTransport(ctx, secondPriv, ""); err != nil {
		t.Fatalf("second CreateSessionTransport: %v", err)
	}
	second := acc.GetSessionTransport()
	if second == nil {
		t.Fatal("expected second session transport")
	}
	if second == first {
		t.Fatal("expected replacement to publish a new session transport")
	}
	if first.GetChildBus() != nil {
		t.Fatal("expected replaced session transport child bus to be cleared")
	}
}

func TestSessionTransportStateExitBeforeReadyFailsStartup(t *testing.T) {
	sts := &sessionTransportState{}
	sts.setExited(nil)

	err := sts.WaitStarted(context.Background())
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !strings.Contains(err.Error(), "session transport exited before ready") {
		t.Fatalf("startup error = %v, want exited-before-ready failure", err)
	}
}

func TestSessionTransportStateExitWakesPendingStartup(t *testing.T) {
	// Start a transport with a pending readiness waiter.
	acc := NewTestProviderAccount(t, "http://example.invalid")
	priv, _ := generateTestKeypair(t)
	st, err := transport.NewSessionTransport(acc.le, acc.p.b, priv, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sts := &sessionTransportState{transport: st}
	baseCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	observed := make(chan struct{})
	waitCtx := &observedDoneContext{
		Context:  baseCtx,
		observed: observed,
	}

	// Wait for the pending startup before publishing exit.
	done := make(chan error, 1)
	go func() {
		done <- sts.WaitStarted(waitCtx)
	}()
	select {
	case <-observed:
	case <-baseCtx.Done():
		t.Fatalf("waiter did not enter its wait path: %v", baseCtx.Err())
	}
	sts.setExited(context.Canceled)

	// Assert the waiter observes the terminal exit.
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait returned %v, want state cancellation", err)
		}
	case <-baseCtx.Done():
		t.Fatalf("state exit did not wake pending startup: %v", baseCtx.Err())
	}
}

func TestSessionTransportStateExitPreservesStartupError(t *testing.T) {
	sts := &sessionTransportState{}
	sts.setExited(context.Canceled)
	startupErr := errors.New("session transport did not become ready")
	sts.setExited(startupErr)

	err := sts.WaitStarted(context.Background())
	if !errors.Is(err, startupErr) {
		t.Fatalf("startup error = %v, want preserved readiness error", err)
	}
}

func TestSessionTransportCanceledStartupAttemptRemainsCurrent(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	sts := &sessionTransportState{}
	acc.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.sessionTransports = map[string]*sessionTransportState{"": sts}
		broadcast()
	})

	acc.handleSessionTransportExit("", sts, context.Canceled)

	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if sts.exited {
			t.Fatal("pre-ready cancellation must not publish terminal exit")
		}
	})
	var current *sessionTransportState
	acc.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = acc.sessionTransports[""]
	})
	if current != sts {
		t.Fatal("pre-ready cancellation must leave transport state current")
	}
	running, _ := acc.GetTransportSnapshotWithWait()
	if !running {
		t.Fatal("pre-ready cancellation must leave transport eligible for retry")
	}
}
