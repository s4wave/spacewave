//go:build !js

package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

func TestBrowserPeerErrorClassification(t *testing.T) {
	deadlineErr := errors.New("context deadline exceeded")
	if !isBrowserPeerStartupErr(deadlineErr) {
		t.Fatal("expected deadline errors to be treated as startup races")
	}
	if shouldAbandonBrowserPeer(deadlineErr) {
		t.Fatal("expected deadline errors to keep retrying the current peer")
	}

	closedErr := errors.New("resource client: quic: transport closed")
	if !shouldAbandonBrowserPeer(closedErr) {
		t.Fatal("expected closed transports to abandon the peer")
	}
}

func TestPeerWatcherReturnsNewestObservation(t *testing.T) {
	pw := &PeerWatcher{pending: make(chan BrowserPeerObservation, 8)}

	pw.observePeer(peer.ID("peer-a"))
	pw.observePeer(peer.ID("peer-b"))

	obs, err := pw.WaitForPeerObservation(context.Background())
	if err != nil {
		t.Fatalf("WaitForPeerObservation: %v", err)
	}
	if obs.PeerID != peer.ID("peer-b") {
		t.Fatalf("expected newest peer-b observation, got %q", obs.PeerID)
	}
	if obs.Sequence != 2 {
		t.Fatalf("expected sequence 2, got %d", obs.Sequence)
	}
	if obs.ObservedAt.IsZero() {
		t.Fatal("expected observation timestamp")
	}
}

func TestResourceConnectionTimingSnapshot(t *testing.T) {
	sess := &TestSession{}
	start := time.Now()
	peerID := peer.ID("peer-a")
	obs := BrowserPeerObservation{
		PeerID:     peerID,
		Sequence:   3,
		ObservedAt: start,
	}

	sess.beginResourceConnectionTiming(start)
	sess.recordPeerWaitTiming(start, start.Add(time.Millisecond), obs, nil)
	sess.recordResourceConnectionAttemptTiming(start, start.Add(2*time.Millisecond), peerID, nil)
	sess.recordResourceStartupReload()
	sess.finishResourceConnectionTiming(start.Add(3*time.Millisecond), nil)

	timing := sess.ResourceConnectionTiming()
	if timing.Elapsed() != 3*time.Millisecond {
		t.Fatalf("expected 3ms elapsed timing, got %s", timing.Elapsed())
	}
	if len(timing.PeerWaits) != 1 || timing.PeerWaits[0].ObservationSequence != 3 {
		t.Fatalf("unexpected peer waits: %#v", timing.PeerWaits)
	}
	if len(timing.Attempts) != 1 || timing.Attempts[0].PeerID != peerID {
		t.Fatalf("unexpected attempts: %#v", timing.Attempts)
	}
	if timing.StartupReloads != 1 {
		t.Fatalf("expected one startup reload, got %d", timing.StartupReloads)
	}
}
