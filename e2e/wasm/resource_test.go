//go:build !js

package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// TestBrowserPeerErrorClassification classifies startup-race and fatal peer errors.
func TestBrowserPeerErrorClassification(t *testing.T) {
	// Deadline errors are startup races that keep retrying the current peer.
	deadlineErr := errors.New("context deadline exceeded")
	if !isBrowserPeerStartupErr(deadlineErr) {
		t.Fatal("expected deadline errors to be treated as startup races")
	}
	if shouldAbandonBrowserPeer(deadlineErr) {
		t.Fatal("expected deadline errors to keep retrying the current peer")
	}

	// Closed transports are fatal for the current peer.
	closedErr := errors.New("resource client: quic: transport closed")
	if !shouldAbandonBrowserPeer(closedErr) {
		t.Fatal("expected closed transports to abandon the peer")
	}
}

// TestPeerWatcherReturnsNewestObservation proves the watcher returns the newest
// buffered observation.
func TestPeerWatcherReturnsNewestObservation(t *testing.T) {
	// Observe two peers through the watcher.
	pw := &PeerWatcher{pending: make(chan BrowserPeerObservation, 8)}
	pw.observePeer(peer.ID("peer-a"))
	pw.observePeer(peer.ID("peer-b"))

	// Wait for the newest observation.
	obs, err := pw.WaitForPeerObservation(context.Background())
	if err != nil {
		t.Fatalf("WaitForPeerObservation: %v", err)
	}

	// The newest observation must be peer-b with sequence 2 and a timestamp.
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

// TestPeerWatcherObservationAfterSkipsStalePeers proves the after-checkpoint
// watcher skips observations at or before the checkpoint.
func TestPeerWatcherObservationAfterSkipsStalePeers(t *testing.T) {
	// Checkpoint the sequence between two observations.
	pw := &PeerWatcher{pending: make(chan BrowserPeerObservation, 8)}
	pw.observePeer(peer.ID("peer-a"))
	afterSeq := pw.LatestSequence()
	pw.observePeer(peer.ID("peer-b"))

	// Wait for the first observation after the checkpoint.
	obs, err := pw.WaitForPeerObservationAfter(context.Background(), afterSeq)
	if err != nil {
		t.Fatalf("WaitForPeerObservationAfter: %v", err)
	}

	// The observation must be peer-b with a sequence after the checkpoint.
	if obs.PeerID != peer.ID("peer-b") {
		t.Fatalf("expected peer-b after checkpoint, got %q", obs.PeerID)
	}
	if obs.Sequence <= afterSeq {
		t.Fatalf("expected sequence after %d, got %d", afterSeq, obs.Sequence)
	}
	t.Run("preserves-unleased-peer", testPeerWatcherPreservesUnleasedPeer)
}

// testPeerWatcherPreservesUnleasedPeer keeps a reconnect from hiding a new client.
func testPeerWatcherPreservesUnleasedPeer(t *testing.T) {
	// Observe a leased peer, a new client, and the leased peer reconnecting.
	pw := &PeerWatcher{pending: make(chan BrowserPeerObservation, 8)}
	pw.observePeer(peer.ID("leased"))
	after := pw.LatestSequence()
	pw.observePeer(peer.ID("new-client"))
	pw.observePeer(peer.ID("leased"))

	// Resource connections consume every mount and apply their own lease check.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	obs, err := pw.WaitForDistinctPeerObservationAfter(ctx, after)
	if err != nil || obs.PeerID != peer.ID("new-client") {
		t.Fatalf("new client was hidden by reconnect: peer=%q err=%v", obs.PeerID, err)
	}
}

// TestResourceConnectionTimingSnapshot records a connection timing snapshot and
// checks its recorded phases.
func TestResourceConnectionTimingSnapshot(t *testing.T) {
	// Build an observation for the timing records.
	sess := &TestSession{}
	start := time.Now()
	peerID := peer.ID("peer-a")
	obs := BrowserPeerObservation{
		PeerID:     peerID,
		Sequence:   3,
		ObservedAt: start,
	}

	// Record one peer wait, one attempt, a startup reload, and the finish.
	sess.beginResourceConnectionTiming(start)
	sess.recordPeerWaitTiming(start, start.Add(time.Millisecond), obs, nil)
	sess.recordResourceConnectionAttemptTiming(start, start.Add(2*time.Millisecond), peerID, nil)
	sess.recordResourceStartupReload()
	sess.finishResourceConnectionTiming(start.Add(3*time.Millisecond), nil)

	// The snapshot must record the elapsed time and every phase.
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
