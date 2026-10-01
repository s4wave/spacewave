package transport_test

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/transport"
)

// TestSessionTransportBusLifetime keeps sibling Sessions independent and
// invalidates a borrowed route when its actual transport owner stops.
func TestSessionTransportBusLifetime(t *testing.T) {
	// Start the session transport under a cancellable context.
	ctx, tb, session := newTestSessionTransport(t, "")
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- session.Execute(ctx) }()
	if err := session.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Resolve the session's own transport bus and capture invalidation.
	invalidated := make(chan struct{}, 1)
	borrowed, release, err := transport.ResolveSessionBus(ctx, tb.Bus, session.GetPeerID(), func() {
		select {
		case invalidated <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Assert the session lookup selected the isolated transport.
	if borrowed != session.GetChildBus() || borrowed == tb.Bus {
		t.Fatal("Session lookup did not select its isolated transport")
	}

	// Resolve another identity's bus and assert it did not take the session's.
	other, releaseOther, err := transport.ResolveSessionBus(ctx, tb.Bus, tb.Volume.GetPeerID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOther()
	if other != tb.Bus {
		t.Fatal("another identity acquired the Session's transport")
	}

	// Cancel the session and confirm it shuts down cleanly.
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("transport shutdown: %v", err)
	}

	// Confirm the closed transport invalidated the borrowed route.
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("closed transport retained its borrowed route")
	}
}
