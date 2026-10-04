package webrtc

import (
	"errors"
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
)

// TestMalformedCandidateDoesNotKillTracker pins defect A: a candidate that
// Pion rejects must be logged and skipped, not returned as a fatal error that
// exits the session tracker routine.
func TestMalformedCandidateDoesNotKillTracker(t *testing.T) {
	// Create the answerer peer connection and its matching offer.
	answerPC, offerDesc := newOfferForAnswerer(t)

	// Configure an ICE applier that rejects its first candidate.
	applied := 0
	f := &fenceIngest{
		tracker: &sessionTracker{
			w:       &WebRTC{conf: &Config{}},
			le:      newFenceTestLogger(),
			offerer: false,
		},
		sess: &session{pc: answerPC},
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error {
				applied++
				if applied == 1 {
					// Simulate the browser-side addIceCandidate failure seen
					// in production ("Error processing ICE candidate").
					return errors.New("JavaScript error: Failed to execute 'addIceCandidate' on 'RTCPeerConnection': Error processing ICE candidate")
				}
				return nil
			},
		},
	}
	if err := f.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: offerDigest(offerDesc.SDP),
	}, nil); err != nil {
		t.Fatal(err.Error())
	}

	// First candidate is rejected by the applier...
	if err := f.ingest(nil, newTestIceSignal(t, "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host", offerDigest(offerDesc.SDP))); err != nil {
		t.Fatalf("rejected candidate was treated as fatal: %v", err)
	}

	// ...and the tracker keeps applying subsequent candidates.
	if err := f.ingest(nil, newTestIceSignal(t, "candidate:2 1 udp 2130706431 10.5.0.3 54505 typ host", offerDigest(offerDesc.SDP))); err != nil {
		t.Fatalf("tracker did not recover after a rejected candidate: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied %d candidates, want 2", applied)
	}
}

// TestCandidatesBufferedBeforeActiveOffer pins defect B (session fence):
// candidates that arrive before any offer generation is active are buffered
// with their offer id, and flushed once a matching offer is applied.
func TestCandidatesBufferedBeforeActiveOffer(t *testing.T) {
	// Create the answerer session and retain its offer generation identity.
	answerPC, offerDesc := newOfferForAnswerer(t)
	offerID := offerDigest(offerDesc.SDP)

	// Configure an ICE applier that records buffered candidate delivery.
	applied := 0
	f := &fenceIngest{
		tracker: &sessionTracker{
			w:       &WebRTC{conf: &Config{}},
			le:      newFenceTestLogger(),
			offerer: false,
		},
		sess: &session{pc: answerPC},
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error {
				applied++
				return nil
			},
		},
	}

	// Candidates trickle in before any offer is active (len(active) == 0).
	// They must be buffered, not dropped.
	if err := f.ingest(nil, newTestIceSignal(t, "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host", offerID)); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.ingest(nil, newTestIceSignal(t, "candidate:2 1 udp 2130706431 10.5.0.3 54505 typ host", offerID)); err != nil {
		t.Fatal(err.Error())
	}
	if applied != 0 {
		t.Fatalf("candidates applied with no remote description: %d", applied)
	}
	if len(f.pendingICE) != 2 {
		t.Fatalf("buffered %d candidates, want 2", len(f.pendingICE))
	}

	// The offer lands; its candidates must be flushed and applied.
	if err := f.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: offerID,
	}, nil); err != nil {
		t.Fatal(err.Error())
	}
	if applied != 2 {
		t.Fatalf("applied %d buffered candidates, want 2", applied)
	}
	if len(f.pendingICE) != 0 {
		t.Fatalf("buffer not drained: %d candidates left", len(f.pendingICE))
	}
}

// TestRestartLoopConverges reproduces the production starvation loop: the
// offerer trickles candidates under generation G while the answerer's tracker
// restarts before any offer is active. Candidates trickled before the restart
// must be carried into the successor via adoption, and candidates trickled
// after the restart must be buffered and applied when the G offer lands, so
// the session converges instead of dropping the entire candidate set forever.
func TestRestartLoopConverges(t *testing.T) {
	// Create the answerer session and identify the negotiation generation.
	answerPC, offerDesc := newOfferForAnswerer(t)
	genID := offerDigest(offerDesc.SDP)

	// Prepare the candidate set that must survive tracker restart.
	trickled := []string{
		"candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host",
		"candidate:2 1 udp 2130706431 10.5.0.3 54505 typ host",
		"candidate:3 1 udp 2130706431 10.5.0.4 54506 typ host",
	}

	// Generation one: trickles for G arrive while no offer is active. Under
	// the old fence they were dropped here ("dropping stale ice candidate").
	// Now they buffer.
	trackerA := &sessionTracker{
		w:       &WebRTC{conf: &Config{}},
		le:      newFenceTestLogger(),
		offerer: false,
	}
	sessA := &session{pc: answerPC}
	fA := &fenceIngest{tracker: trackerA, sess: sessA}
	for _, c := range trickled {
		if err := fA.ingest(nil, newTestIceSignal(t, c, genID)); err != nil {
			t.Fatalf("pre-offer trickle returned %v, want buffering", err)
		}
	}
	if len(fA.pendingICE) != len(trickled) {
		t.Fatalf("generation one buffered %d candidates, want %d",
			len(fA.pendingICE), len(trickled))
	}

	// The tracker restarts. The successor adopts the session: production
	// hands sess.pendingRemoteIce to the successor with the session itself.
	// The test replays the same handover into the successor's ingest state.
	trackerB := &sessionTracker{
		w:       &WebRTC{conf: &Config{}},
		le:      newFenceTestLogger(),
		offerer: false,
	}
	trackerB.adoptSession(sessA)

	// Count candidate applications in the adopted successor session.
	applied := 0
	fB := &fenceIngest{
		tracker: trackerB,
		sess:    sessA,
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error {
				applied++
				return nil
			},
		},
	}
	fB.pendingICE = append(fB.pendingICE, fA.pendingICE...)
	fA.pendingICE = nil

	// The G offer lands on the successor; the buffered G candidates flush.
	if err := fB.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: genID,
	}, nil); err != nil {
		t.Fatal(err.Error())
	}
	if applied != len(trickled) {
		t.Fatalf("flush applied %d candidates, want %d: the restart starved ICE",
			applied, len(trickled))
	}
	if len(fB.pendingICE) != 0 {
		t.Fatalf("successor buffer not drained: %d candidates left", len(fB.pendingICE))
	}
	if sessA.pc.RemoteDescription() == nil {
		t.Fatal("successor never applied the offer")
	}
}
