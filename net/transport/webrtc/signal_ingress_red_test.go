package webrtc

// These tests pin the per-peer signal-ingress generation fence. SDP and ICE
// signals carry the SHA-256 of the offer SDP bytes they belong to, and
// material whose digest does not match the active generation is dropped
// before Pion state is touched. Matching-generation behavior, including the
// existing fatal role and Pion error paths, stays unchanged.

import (
	"crypto/sha256"
	"errors"
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
// TestAnswerCorrelatesAcrossTrackerRegeneration pins the adoption seam across
// tracker regeneration. The successor adopts the handed-over session, clears
// any predecessor fatal error, and continues negotiation with a fresh offer;
// answers from earlier generations stay fenced out.
func TestAnswerCorrelatesAcrossTrackerRegeneration(t *testing.T) {
	w := &WebRTC{conf: &Config{}}

	newTrackerSess := func(pc *pion_webrtc.PeerConnection) (*sessionTracker, *session) {
		tracker := &sessionTracker{
			w:       w,
			le:      newFenceTestLogger(),
			offerer: true,
			key:     "regen-peer",
		}
		return tracker, &session{t: tracker, pc: pc}
	}

	xmit := func(*WebRtcSignal) {}
	le := newFenceTestLogger()

	// Generation one: the first tracker transmits its local offer.
	pcA, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { pcA.Close() })
	trackerA, sessA := newTrackerSess(pcA)
	seqA, _, err := trackerA.transmitLocalNegotiation(sessA, le, 1, 0, xmit)
	if err != nil {
		t.Fatal(err.Error())
	}
	genOneID := append([]byte(nil), sessA.pendingOfferID...)
	if len(genOneID) == 0 {
		t.Fatal("first generation recorded no pending offer id")
	}

	// The remote answered generation one before the tracker retired: the
	// session carries a live connection worth handing over.
	_, _, _, answerDesc := newNegotiatedPair(t)
	if err := pcA.SetRemoteDescription(*answerDesc); err != nil {
		t.Fatal(err.Error())
	}

	// The tracker retires: the in-flight session is handed to the peer's
	// ingress lease for adoption instead of disposed.
	w.incomingSessions = map[string]*signalIngress{"regen-peer": {}}
	sessA.close()

	// A successor regenerates on the same peer key and adopts the handed-over
	// session.
	trackerB := &sessionTracker{
		w:       w,
		le:      le,
		offerer: true,
		key:     "regen-peer",
	}
	sessB := w.takeAdoptableSession("regen-peer")
	if sessB == nil {
		t.Fatal("successor found no adoptable session after regeneration")
	}
	trackerB.adoptSession(sessB)
	if sessB.pc != pcA {
		t.Fatal("adopted session lost its peer connection")
	}

	// Generation two: the successor continues negotiation on the adopted
	// connection with a fresh offer generation.
	seqB, _, err := trackerB.transmitLocalNegotiation(sessB, le, seqA+1, 0, xmit)
	if err != nil {
		t.Fatal(err.Error())
	}
	if seqB <= seqA {
		t.Fatalf("successor seqno %d did not advance past %d", seqB, seqA)
	}
	genTwoID := append([]byte(nil), sessB.pendingOfferID...)

	f := &fenceIngest{
		tracker: trackerB,
		sess:    sessB,
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error { return nil },
		},
	}

	// A late duplicate answer for generation one arrives after the successor's
	// fresh offer. Its offer_id no longer matches the pending generation and
	// must drop before Pion state is touched.
	if err := f.ingest(&WebRtcSdp{
		SdpType: "answer",
		Sdp:     answerDesc.SDP,
		OfferId: genOneID,
	}, nil); err != nil {
		t.Fatalf("stale-generation answer returned %v, want silent drop", err)
	}

	// The answer for the fresh generation correlates and is applied.
	if err := f.ingest(&WebRtcSdp{
		SdpType: "answer",
		Sdp:     answerDesc.SDP,
		OfferId: genTwoID,
	}, nil); err != nil {
		t.Fatalf("correlating answer returned %v, want application", err)
	}
	if sessB.pc.RemoteDescription() == nil {
		t.Fatal("correlating answer was dropped: successor never applied the remote description")
	}
}

// TestCloseSkipsStashOnFatalError asserts a session carrying a fatal error is
// never handed to the peer's ingress lease: its successor would exit on the
// first snapshot and re-stash, wedging the ingress against new signals. The
// session otherwise meets every adoption condition, including an applied
// answer, so the fatal error alone decides the disposition.
func TestCloseSkipsStashOnFatalError(t *testing.T) {
	w := &WebRTC{conf: &Config{}}
	offerPC, _, _, answerDesc := newNegotiatedPair(t)
	if err := offerPC.SetRemoteDescription(*answerDesc); err != nil {
		t.Fatal(err.Error())
	}
	tracker := &sessionTracker{
		w:       w,
		le:      newFenceTestLogger(),
		offerer: true,
		key:     "fatal-peer",
	}
	sess := &session{t: tracker, pc: offerPC, pendingOfferID: []byte("outstanding-offer")}
	sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		sess.fatalErr = errors.New("signal transmit routine failed")
		sess.connState = pion_webrtc.PeerConnectionStateConnecting
		broadcast()
	})

	w.incomingSessions = map[string]*signalIngress{"fatal-peer": {}}
	sess.close()

	if stashed := w.takeAdoptableSession("fatal-peer"); stashed != nil {
		t.Fatal("close stashed a session carrying a fatal error")
	}
	if offerPC.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
		t.Fatalf("fatal session was not disposed: connection state %s", offerPC.ConnectionState().String())
	}
}

// TestCloseDisposesUnansweredOfferSession asserts the adoption boundary of the
// offerer handover: an offer that never drew an answer cannot be re-answered
// (the remote deduplicates byte-identical offers) nor replaced (pion v4 has no
// rollback), so close disposes it and the successor mints a fresh generation.
func TestCloseDisposesUnansweredOfferSession(t *testing.T) {
	w := &WebRTC{conf: &Config{}}
	offerPC, _, _, _ := newNegotiatedPair(t)
	tracker := &sessionTracker{
		w:       w,
		le:      newFenceTestLogger(),
		offerer: true,
		key:     "unanswered-peer",
	}
	sess := &session{t: tracker, pc: offerPC, pendingOfferID: []byte("unanswered-offer")}
	sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		sess.connState = pion_webrtc.PeerConnectionStateConnecting
		broadcast()
	})

	w.incomingSessions = map[string]*signalIngress{"unanswered-peer": {}}
	sess.close()

	if stashed := w.takeAdoptableSession("unanswered-peer"); stashed != nil {
		t.Fatal("close stashed a session whose outstanding offer drew no answer")
	}
	if offerPC.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
		t.Fatalf("unanswered-offer session was not disposed: connection state %s", offerPC.ConnectionState().String())
	}
}

// TestAdoptedSessionClearsFatalError asserts the adoption rebinding: the
// successor clears the predecessor's fatal error, rebinds the tracker under
// the session lock, and arms a fresh offer when no answer was applied.
func TestAdoptedSessionClearsFatalError(t *testing.T) {
	w := &WebRTC{conf: &Config{}}
	pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { pc.Close() })
	trackerA := &sessionTracker{
		w:       w,
		le:      newFenceTestLogger(),
		offerer: true,
		key:     "adopt-peer",
	}
	sess := &session{t: trackerA, pc: pc, pendingOfferID: []byte("outstanding-offer")}
	sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		sess.fatalErr = errors.New("predecessor failed")
		sess.connState = pion_webrtc.PeerConnectionStateConnecting
		broadcast()
	})

	trackerB := &sessionTracker{
		w:       w,
		le:      newFenceTestLogger(),
		offerer: true,
		key:     "adopt-peer",
	}
	waitCh := trackerB.adoptSession(sess)
	if waitCh == nil {
		t.Fatal("adoption returned no wait channel")
	}
	sess.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if sess.fatalErr != nil {
			t.Fatalf("adopted session kept the predecessor fatal error: %v", sess.fatalErr)
		}
		if sess.t != trackerB {
			t.Fatal("adopted session kept the predecessor tracker binding")
		}
	})
}

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
