//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
	"time"
)

// TestRetainedStatePagePeerProbe records whether a fresh page opened in a
// retained BrowserContext gets a new browser peer or can reuse the retained
// peer.
func TestRetainedStatePagePeerProbe(t *testing.T) {
	// Open a retained blank session and its peer watcher.
	h := harness(t)
	sess := h.NewRetainedStateBlankSession(t)
	watcher := h.getPeerWatcher()

	// Load the app and record the watcher sequence.
	firstAfter := watcher.LatestSequence()
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load first retained-state page: %v", err)
	}

	// Wait for the first peer observation.
	firstCtx, firstCancel := context.WithTimeout(h.Context(), 2*time.Minute)
	firstObs, err := watcher.WaitForPeerObservationAfter(firstCtx, firstAfter)
	firstCancel()
	if err != nil {
		t.Fatalf("observe first page browser peer: %v", err)
	}

	// Replace the page in the retained context.
	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current retained-state context: %v", err)
	}

	// Reload the app and record the next watcher sequence.
	secondAfter := watcher.LatestSequence()
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load second retained-state page: %v", err)
	}

	// Wait for a second peer observation.
	secondCtx, secondCancel := context.WithTimeout(h.Context(), 45*time.Second)
	secondObs, err := watcher.WaitForPeerObservationAfter(secondCtx, secondAfter)
	secondCancel()

	// Accept the retained peer when the second observation times out.
	secondPeer := firstObs.PeerID
	secondSeq := uint64(0)
	source := "retained-peer-connect"
	if err == nil {
		secondPeer = secondObs.PeerID
		secondSeq = secondObs.Sequence
		source = "peer-observation"
	} else {
		connectCtx, connectCancel := context.WithTimeout(h.Context(), 15*time.Second)
		conn, connectErr := h.tryConnectSession(connectCtx, firstObs.PeerID)
		connectCancel()
		if connectErr != nil {
			t.Logf(
				"retained-state page peer probe: first_peer=%s first_sequence=%d second_peer= second_sequence=0 source=unavailable reused_retained_peer=false observation_error=%v retained_connect_error=%v",
				firstObs.PeerID,
				firstObs.Sequence,
				err,
				connectErr,
			)
			return
		}
		conn.Release()
	}

	// Log the peer probe result.
	t.Logf(
		"retained-state page peer probe: first_peer=%s first_sequence=%d second_peer=%s second_sequence=%d source=%s reused_retained_peer=%t",
		firstObs.PeerID,
		firstObs.Sequence,
		secondPeer,
		secondSeq,
		source,
		firstObs.PeerID == secondPeer,
	)
}
