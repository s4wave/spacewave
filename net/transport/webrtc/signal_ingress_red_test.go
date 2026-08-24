package webrtc

// These tests pin the per-peer signal-ingress generation fence. SDP and ICE
// signals carry the SHA-256 of the offer SDP bytes they belong to, and
// material whose digest does not match the active generation is dropped
// before Pion state is touched. Matching-generation behavior, including the
// existing fatal role and Pion error paths, stays unchanged.

import (
	"crypto/sha256"
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"
)

// newFenceTestLogger returns a quiet logger for fence tests.
func newFenceTestLogger() *logrus.Entry {
	return logrus.NewEntry(logrus.New())
}

// fenceIngest drives ingestRemoteSignal with persistent per-session state so a
// test can feed a sequence of signals the way the execute loop would.
type fenceIngest struct {
	tracker     *sessionTracker
	sess        *session
	lastApplied string
	pendingICE  []pion_webrtc.ICECandidateInit
	applier     *remoteICECandidateApplier
}

// ingest applies one received SDP/candidate signal pair.
func (f *fenceIngest) ingest(sdp *WebRtcSdp, ice *WebRtcIce) error {
	phase := ""
	return f.tracker.ingestRemoteSignal(
		f.sess,
		sdp,
		ice,
		0,
		&f.lastApplied,
		f.applier,
		&f.pendingICE,
		func(*WebRtcSignal) {},
		newFenceTestLogger(),
		&phase,
	)
}

// offerDigest computes the SHA-256 generation identity of an offer's SDP bytes.
func offerDigest(sdp string) []byte {
	sum := sha256.Sum256([]byte(sdp))
	return sum[:]
}

// newOfferForAnswerer builds one valid offer and an answerer-side peer
// connection. The caller applies the offer through fenceIngest.ingest so the
// session under test records its active generation itself.
func newOfferForAnswerer(t *testing.T) (*pion_webrtc.PeerConnection, *pion_webrtc.SessionDescription) {
	t.Helper()
	offerPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	answerPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		offerPC.Close()
		answerPC.Close()
	})
	if _, err := offerPC.CreateDataChannel(dataChannelID, nil); err != nil {
		t.Fatal(err.Error())
	}
	offerDesc, err := offerPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := offerPC.SetLocalDescription(offerDesc); err != nil {
		t.Fatal(err.Error())
	}
	return answerPC, &offerDesc
}

// newTestIceSignal builds one ICE candidate message with the given offer id.
func newTestIceSignal(t *testing.T, candidate string, offerID []byte) *WebRtcIce {
	t.Helper()
	mlineIndex := uint16(0)
	ice, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMLineIndex: &mlineIndex,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	ice.OfferId = offerID
	return ice
}

// newNegotiatedPair builds an offerer-side peer connection holding a local
// offer and the matching answer the answerer would return.
func newNegotiatedPair(t *testing.T) (offerPC, answerPC *pion_webrtc.PeerConnection, offerDesc, answerDesc *pion_webrtc.SessionDescription) {
	t.Helper()
	offerPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	answerPC, err = pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		offerPC.Close()
		answerPC.Close()
	})
	if _, err := offerPC.CreateDataChannel(dataChannelID, nil); err != nil {
		t.Fatal(err.Error())
	}
	offer, err := offerPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := offerPC.SetLocalDescription(offer); err != nil {
		t.Fatal(err.Error())
	}
	if err := answerPC.SetRemoteDescription(offer); err != nil {
		t.Fatal(err.Error())
	}
	answer, err := answerPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := answerPC.SetLocalDescription(answer); err != nil {
		t.Fatal(err.Error())
	}
	return offerPC, answerPC, &offer, &answer
}

// TestSignalIngressRejectsStaleGenerationAnswer asserts the answer seam: an
// answer whose offer_id does not match the active generation must be dropped
// before Pion state is touched, leaving the remote description unapplied.
func TestSignalIngressRejectsStaleGenerationAnswer(t *testing.T) {
	offerPC, _, _, answerDesc := newNegotiatedPair(t)

	tracker := &sessionTracker{
		w:       &WebRTC{conf: &Config{}},
		le:      newFenceTestLogger(),
		offerer: true,
	}
	sess := &session{pc: offerPC}
	f := &fenceIngest{
		tracker: tracker,
		sess:    sess,
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error { return nil },
		},
	}

	staleID := sha256.Sum256([]byte("retired-generation-offer"))
	err := f.ingest(&WebRtcSdp{
		SdpType: "answer",
		Sdp:     answerDesc.SDP,
		OfferId: staleID[:],
	}, nil)
	if err != nil {
		t.Fatalf("stale-generation answer returned %v, want silent drop", err)
	}
	if sess.pc.RemoteDescription() != nil {
		t.Fatalf("stale-generation answer reached Pion: remote description was applied")
	}
}

// TestSignalIngressDropsStaleGenerationCandidate asserts the candidate seam:
// after an offer is active, a candidate tagged with a different offer_id must
// be dropped before it reaches the remote ICE applier.
func TestSignalIngressDropsStaleGenerationCandidate(t *testing.T) {
	answerPC, offerDesc := newOfferForAnswerer(t)

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
	if err := f.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: offerDigest(offerDesc.SDP),
	}, nil); err != nil {
		t.Fatal(err.Error())
	}

	otherSum := sha256.Sum256([]byte("some-other-generation"))
	err := f.ingest(nil, newTestIceSignal(t, "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host", otherSum[:]))
	if err != nil {
		t.Fatalf("stale-generation candidate returned %v, want silent drop", err)
	}
	if applied != 0 {
		t.Fatalf("stale-generation candidate reached the remote ICE applier: %d applied", applied)
	}
}

// TestSignalIngressMatchingGenerationFatalControls asserts the invariants the
// fence must not disturb: role enforcement stays fatal, and a
// matching-generation candidate still reaches the ICE applier.
func TestSignalIngressMatchingGenerationFatalControls(t *testing.T) {
	t.Run("role_violating_offer_remains_fatal", func(t *testing.T) {
		offerPC, _, _, _ := newNegotiatedPair(t)
		tracker := &sessionTracker{
			w:       &WebRTC{conf: &Config{}},
			le:      newFenceTestLogger(),
			offerer: true,
		}
		f := &fenceIngest{
			tracker: tracker,
			sess:    &session{pc: offerPC},
			applier: &remoteICECandidateApplier{
				add: func(pion_webrtc.ICECandidateInit) error { return nil },
			},
		}
		err := f.ingest(&WebRtcSdp{SdpType: "offer", Sdp: "v=0\r\n"}, nil)
		if err == nil {
			t.Fatalf("role-violating offer was not fatal")
		}
	})

	t.Run("matching_generation_candidate_reaches_the_ICE_applier", func(t *testing.T) {
		answerPC, offerDesc := newOfferForAnswerer(t)

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
		if err := f.ingest(&WebRtcSdp{
			SdpType: "offer",
			Sdp:     offerDesc.SDP,
			OfferId: offerDigest(offerDesc.SDP),
		}, nil); err != nil {
			t.Fatal(err.Error())
		}
		err := f.ingest(nil, newTestIceSignal(
			t,
			"candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host",
			offerDigest(offerDesc.SDP),
		))
		if err != nil {
			t.Fatal(err.Error())
		}
		if applied != 1 {
			t.Fatalf("matching-generation candidate applied %d times, want exactly 1", applied)
		}
	})
}
