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
	h := testHarness
	sess := h.NewRetainedStateBlankSession(t)
	watcher := h.getPeerWatcher()

	firstAfter := watcher.LatestSequence()
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load first retained-state page: %v", err)
	}

	firstCtx, firstCancel := context.WithTimeout(h.Context(), 2*time.Minute)
	firstObs, err := watcher.WaitForPeerObservationAfter(firstCtx, firstAfter)
	firstCancel()
	if err != nil {
		t.Fatalf("observe first page browser peer: %v", err)
	}

	if err := sess.ReplacePageInCurrentContext(); err != nil {
		t.Fatalf("replace page in current retained-state context: %v", err)
	}

	secondAfter := watcher.LatestSequence()
	if err := h.loadAppPageURL(sess, h.BaseURL()+"/#/"); err != nil {
		t.Fatalf("load second retained-state page: %v", err)
	}

	secondCtx, secondCancel := context.WithTimeout(h.Context(), 45*time.Second)
	secondObs, err := watcher.WaitForPeerObservationAfter(secondCtx, secondAfter)
	secondCancel()

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
			t.Fatalf("determine second page peer: no new observation after retained context reload (%v), and retained peer %s was unavailable: %v", err, firstObs.PeerID, connectErr)
		}
		conn.Release()
	}

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
