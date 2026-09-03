package webrtc

// These tests pin the per-peer signal-ingress generation fence. SDP and ICE
// signals carry the SHA-256 of the offer SDP bytes they belong to, and
// material whose digest does not match the active generation is dropped
// before Pion state is touched. Matching-generation behavior, including the
// existing fatal role and Pion error paths, stays unchanged.

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

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
	pendingICE  []pendingRemoteCandidate
	applier     *remoteICECandidateApplier
	xmit        func(*WebRtcSignal)
}

// ingest applies one received SDP/candidate signal pair.
func (f *fenceIngest) ingest(sdp *WebRtcSdp, ice *WebRtcIce) error {
	phase := ""
	xmit := f.xmit
	if xmit == nil {
		xmit = func(*WebRtcSignal) {}
	}
	return f.tracker.ingestRemoteSignal(
		f.sess,
		sdp,
		ice,
		0,
		&f.lastApplied,
		f.applier,
		&f.pendingICE,
		xmit,
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

func TestAdoptedOfferCarriesIceAcrossTrackerGeneration(t *testing.T) {
	offerID := []byte("active-offer")
	sess := &session{pendingOfferID: append([]byte(nil), offerID...)}
	w := &WebRTC{
		conf: &Config{},
		incomingSessions: map[string]*signalIngress{
			"peer": {adoptedSession: sess},
		},
	}

	candidate := &WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: &WebRtcIce{OfferId: offerID}}}
	stale := &WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: &WebRtcIce{OfferId: []byte("stale")}}}
	ingress := w.incomingSessions["peer"]
	if !carriesAdoptedOffer(ingress, nil, candidate) {
		t.Fatal("matching candidate was not carried while the successor was detached")
	}
	if carriesAdoptedOffer(ingress, nil, stale) {
		t.Fatal("unrelated candidate crossed the detached generation")
	}

	execution := &sessionTrackerExecution{}
	if got := w.takeAdoptableSession("peer", execution); got != sess {
		t.Fatal("successor did not adopt the stashed session")
	}
	if !carriesAdoptedOffer(ingress, execution, candidate) {
		t.Fatal("matching candidate was not carried into the successor execution")
	}
	if carriesAdoptedOffer(ingress, execution, stale) {
		t.Fatal("unrelated candidate crossed into the successor execution")
	}
}

// TestSignalIngressRejectsStaleGenerationAnswer asserts the answer seam: an
// answer whose offer_id does not match the active generation must be dropped
// before Pion state is touched, leaving the remote description unapplied.
// TestAnswerCorrelatesAcrossTrackerRegeneration pins the adoption seam across
// tracker regeneration. The successor adopts the handed-over session with its
// outstanding local offer, retransmits the identical offer generation, drops
// an answer for any other generation before Pion state is touched, and
// applies the answer that matches the outstanding generation.
func TestAnswerCorrelatesAcrossTrackerRegeneration(t *testing.T) {
	w := &WebRTC{conf: &Config{}}

	newTracker := func() *sessionTracker {
		return &sessionTracker{
			w:       w,
			le:      newFenceTestLogger(),
			offerer: true,
			key:     "regen-peer",
		}
	}

	var xmitted []*WebRtcSdp
	xmit := func(sig *WebRtcSignal) {
		xmitted = append(xmitted, sig.GetBody().(*WebRtcSignal_Sdp).Sdp)
	}
	le := newFenceTestLogger()

	// Generation one: the first tracker transmits its local offer and retires
	// with the offer still outstanding.
	pcA, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { pcA.Close() })
	if _, err := pcA.CreateDataChannel(dataChannelID, nil); err != nil {
		t.Fatal(err.Error())
	}
	trackerA := newTracker()
	sessA := &session{t: trackerA, pc: pcA}
	seqA, _, err := trackerA.transmitLocalNegotiation(sessA, le, 1, 0, xmit)
	if err != nil {
		t.Fatal(err.Error())
	}
	genOneID := append([]byte(nil), sessA.pendingOfferID...)
	offerSDP := xmitted[0].GetSdp()
	if offerSDP == "" || !strings.Contains(offerSDP, "a=ice-ufrag") {
		t.Fatalf("first generation transmitted no usable offer: n=%d body=%q type=%q", len(xmitted), offerSDP, xmitted[0].GetSdpType())
	}
	if len(genOneID) == 0 || offerSDP == "" {
		t.Fatal("first generation recorded no outstanding offer")
	}

	// The tracker retires mid-handshake: the in-flight negotiation is handed
	// to the peer's ingress lease for adoption instead of disposed.
	w.incomingSessions = map[string]*signalIngress{"regen-peer": {}}
	sessA.close()

	// A successor regenerates on the same peer key and adopts the handed-over
	// session.
	trackerB := newTracker()
	sessB := w.takeAdoptableSession("regen-peer", nil)
	if sessB == nil {
		t.Fatal("successor found no adoptable session after regeneration")
	}
	trackerB.adoptSession(sessB)
	if sessB.pc != pcA {
		t.Fatal("adopted session lost its peer connection")
	}

	// The successor retransmits the identical outstanding offer instead of
	// minting a new generation, so an in-flight or replayed answer still
	// correlates.
	before := len(xmitted)
	if _, _, err := trackerB.transmitLocalNegotiation(sessB, le, seqA+1, 0, xmit); err != nil {
		t.Fatal(err.Error())
	}
	if len(xmitted) != before+1 {
		t.Fatalf("successor transmitted %d offers, want exactly one retransmission", len(xmitted)-before)
	}
	retrans := xmitted[len(xmitted)-1]
	if retrans.GetSdpType() != "offer" || retrans.GetSdp() != offerSDP {
		t.Fatal("successor did not retransmit the identical outstanding offer")
	}
	if !bytes.Equal(retrans.GetOfferId(), genOneID) {
		t.Fatal("retransmitted offer changed the generation identity")
	}

	f := &fenceIngest{
		tracker: trackerB,
		sess:    sessB,
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error { return nil },
		},
	}

	// A late duplicate answer for a retired generation arrives after the
	// handover. Its offer_id no longer matches the outstanding generation and
	// must drop before Pion state is touched.
	staleID := sha256.Sum256([]byte("retired-generation-offer"))
	if err := f.ingest(&WebRtcSdp{
		SdpType: "answer",
		Sdp:     "stale-answer-sdp",
		OfferId: staleID[:],
	}, nil); err != nil {
		t.Fatalf("stale-generation answer returned %v, want silent drop", err)
	}

	// The answer for the outstanding generation correlates and is applied.
	answerPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { answerPC.Close() })
	if err := answerPC.SetRemoteDescription(pion_webrtc.SessionDescription{
		Type: pion_webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}); err != nil {
		t.Fatal(err.Error())
	}
	answerDesc, err := answerPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := f.ingest(&WebRtcSdp{
		SdpType: "answer",
		Sdp:     answerDesc.SDP,
		OfferId: genOneID,
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

	if stashed := w.takeAdoptableSession("fatal-peer", nil); stashed != nil {
		t.Fatal("close stashed a session carrying a fatal error")
	}
	if offerPC.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
		t.Fatalf("fatal session was not disposed: connection state %s", offerPC.ConnectionState().String())
	}
}

// waitForSignalingState bounds the asynchronous application of pion
// description operations before the test asserts on the signaling state.
func waitForSignalingState(t *testing.T, pc *pion_webrtc.PeerConnection, want pion_webrtc.SignalingState) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for pc.SignalingState() != want {
		select {
		case <-deadline:
			t.Fatalf("signaling state %v, want %v", pc.SignalingState(), want)
		case <-time.After(time.Millisecond):
		}
	}
}

// newOutstandingOfferSession builds an offerer session holding a live
// outstanding local offer in the have-local-offer signaling state, the shape a
// tracker hands over when it retires mid-negotiation.
func newOutstandingOfferSession(t *testing.T, w *WebRTC, key string) (*sessionTracker, *session, *pion_webrtc.PeerConnection) {
	t.Helper()
	pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { pc.Close() })
	if _, err := pc.CreateDataChannel(dataChannelID, nil); err != nil {
		t.Fatal(err.Error())
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err.Error())
	}
	tracker := &sessionTracker{w: w, le: newFenceTestLogger(), offerer: true, key: key}
	sess := &session{
		t:               tracker,
		pc:              pc,
		pendingOfferID:  offerDigest(offer.SDP),
		pendingOfferSDP: offer.SDP,
	}
	sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		sess.connState = pion_webrtc.PeerConnectionStateConnecting
		broadcast()
	})
	return tracker, sess, pc
}

// TestCloseStashesOutstandingOfferForAdoption asserts the offerer handover:
// retiring with a live outstanding local offer hands the session to the peer's
// ingress lease with its connection, description, and generation identity
// intact, so the successor can retransmit the identical offer.
func TestCloseStashesOutstandingOfferForAdoption(t *testing.T) {
	w := &WebRTC{conf: &Config{}}
	tracker, sess, pc := newOutstandingOfferSession(t, w, "handover-peer")
	pendingID := append([]byte(nil), sess.pendingOfferID...)

	w.incomingSessions = map[string]*signalIngress{"handover-peer": {}}
	sess.close()

	stashed := w.takeAdoptableSession("handover-peer", nil)
	if stashed == nil {
		t.Fatal("close disposed an outstanding-offer session instead of handing it over")
	}
	if stashed.pc != pc {
		t.Fatal("handed-over session lost its peer connection")
	}
	if !bytes.Equal(stashed.pendingOfferID, pendingID) {
		t.Fatal("handed-over session lost its outstanding generation identity")
	}
	if state := stashed.pc.SignalingState(); state != pion_webrtc.SignalingStateHaveLocalOffer {
		t.Fatalf("handed-over session signaling state %v, want have-local-offer", state)
	}
	if stashed.pc.ConnectionState() == pion_webrtc.PeerConnectionStateClosed {
		t.Fatal("handed-over session's peer connection was disposed")
	}
	_ = tracker
}

// TestCloseDisposesNonAdoptableSessions asserts every non-adoptable
// disposition of the offerer handover: a fatal error, a finished or unusable
// connection, and sessions without an outstanding local offer are disposed so
// the successor mints a fresh generation on a new connection.
func TestCloseDisposesNonAdoptableSessions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*session)
	}{
		{"fatal_error", func(s *session) {
			s.fatalErr = errors.New("signal transmit routine failed")
		}},
		{"connected", func(s *session) {
			s.connState = pion_webrtc.PeerConnectionStateConnected
		}},
		{"failed", func(s *session) {
			s.connState = pion_webrtc.PeerConnectionStateFailed
		}},
		{"closed", func(s *session) {
			s.connState = pion_webrtc.PeerConnectionStateClosed
		}},
		{"no_outstanding_offer", func(s *session) {
			s.pendingOfferID = nil
		}},
		{"already_answered", func(s *session) {
			answerPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
			if err != nil {
				t.Fatal(err.Error())
			}
			t.Cleanup(func() { answerPC.Close() })
			if err := answerPC.SetRemoteDescription(pion_webrtc.SessionDescription{
				Type: pion_webrtc.SDPTypeOffer,
				SDP:  s.pendingOfferSDP,
			}); err != nil {
				t.Fatal(err.Error())
			}
			answer, err := answerPC.CreateAnswer(nil)
			if err != nil {
				t.Fatal(err.Error())
			}
			if err := s.pc.SetRemoteDescription(answer); err != nil {
				t.Fatal(err.Error())
			}
			waitForSignalingState(t, s.pc, pion_webrtc.SignalingStateStable)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &WebRTC{conf: &Config{}}
			tracker, sess, pc := newOutstandingOfferSession(t, w, "dispose-peer")
			tc.mutate(sess)

			w.incomingSessions = map[string]*signalIngress{"dispose-peer": {}}
			sess.close()

			if stashed := w.takeAdoptableSession("dispose-peer", nil); stashed != nil {
				t.Fatalf("close handed over a non-adoptable session: %s", tc.name)
			}
			if pc.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
				t.Fatalf("non-adoptable session was not disposed: connection state %s", pc.ConnectionState().String())
			}
			_ = tracker
		})
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

// TestAnswererReplaysRetainedAnswerOnDuplicateOffer pins the answerer side of
// the regeneration handover: when the byte-identical offer this session
// already answered arrives again, the session replays its retained local
// answer carrying the same generation identity, and its Pion state stays
// untouched.
func TestAnswererReplaysRetainedAnswerOnDuplicateOffer(t *testing.T) {
	answerPC, offerDesc := newOfferForAnswerer(t)
	gatherComplete := pion_webrtc.GatheringCompletePromise(answerPC)

	var xmitted []*WebRtcSdp
	tracker := &sessionTracker{
		w:       &WebRTC{conf: &Config{}},
		le:      newFenceTestLogger(),
		offerer: false,
	}
	sess := &session{pc: answerPC}
	f := &fenceIngest{
		tracker: tracker,
		sess:    sess,
		applier: &remoteICECandidateApplier{
			add: func(pion_webrtc.ICECandidateInit) error { return nil },
		},
		xmit: func(sig *WebRtcSignal) {
			xmitted = append(xmitted, sig.GetBody().(*WebRtcSignal_Sdp).Sdp)
		},
	}

	// First delivery of the offer: apply it and transmit the fresh answer.
	if err := f.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: offerDigest(offerDesc.SDP),
	}, nil); err != nil {
		t.Fatal(err.Error())
	}
	if len(xmitted) != 1 || xmitted[0].GetSdpType() != "answer" {
		t.Fatalf("first offer produced %d answers, want exactly one answer", len(xmitted))
	}
	firstAnswer := xmitted[0]

	select {
	case <-gatherComplete:
	case <-time.After(5 * time.Second):
		t.Fatal("answer ICE gathering did not complete")
	}
	stateBefore := answerPC.SignalingState()
	localBefore := answerPC.LocalDescription().SDP

	// Byte-identical redelivery of the same generation: replay the retained
	// answer instead of re-running SetRemote/CreateAnswer/SetLocal.
	if err := f.ingest(&WebRtcSdp{
		SdpType: "offer",
		Sdp:     offerDesc.SDP,
		OfferId: offerDigest(offerDesc.SDP),
	}, nil); err != nil {
		t.Fatal(err.Error())
	}
	if len(xmitted) != 2 {
		t.Fatalf("duplicate offer produced %d answers, want exactly one replay", len(xmitted))
	}
	replayed := xmitted[1]
	if replayed.GetSdp() != firstAnswer.GetSdp() {
		t.Fatal("replayed answer changed the originally transmitted SDP bytes")
	}
	if !bytes.Equal(replayed.GetOfferId(), sess.rxOfferID) {
		t.Fatal("replayed answer changed the generation identity")
	}
	if answerPC.SignalingState() != stateBefore {
		t.Fatal("replay mutated the signaling state")
	}
	if answerPC.LocalDescription().SDP != localBefore {
		t.Fatal("replay mutated the local description")
	}
}

// TestCloseSignalIngressDisposesStashedSessionOnLastResolver pins that the
// last resolver out detaches the lease and disposes a session handed over for
// adoption exactly once: the stash is gone and the peer connection is closed.
func TestCloseSignalIngressDisposesStashedSessionOnLastResolver(t *testing.T) {
	w := &WebRTC{conf: &Config{}}
	pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { pc.Close() })
	sess := &session{pc: pc, pendingOfferID: []byte("outstanding-offer")}
	resolver := &handleSignalPeerResolver{t: w}
	w.incomingSessions = map[string]*signalIngress{"detach-peer": {
		adoptedSession: sess,
		resolvers:      map[*handleSignalPeerResolver]struct{}{resolver: {}},
	}}

	w.closeSignalIngress("detach-peer", resolver)

	if w.incomingSessions["detach-peer"] != nil {
		t.Fatal("ingress lease survived its last resolver")
	}
	if taken := w.takeAdoptableSession("detach-peer", nil); taken != nil {
		t.Fatal("stash survived the last-resolver detach")
	}
	if pc.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
		t.Fatalf("detached stash was not disposed: connection state %s", pc.ConnectionState().String())
	}
}

// TestCloseSignalIngressTakeVsDisposeExactlyOnce pins the concurrent handover
// boundary: a successor taking the stashed session races the last-resolver
// cleanup, and the take-and-clear under the transport lock yields exactly one
// adopter or one disposer, never both.
func TestCloseSignalIngressTakeVsDisposeExactlyOnce(t *testing.T) {
	for i := range 50 {
		w := &WebRTC{conf: &Config{}}
		pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
		if err != nil {
			t.Fatal(err.Error())
		}
		sess := &session{pc: pc, pendingOfferID: []byte("outstanding-offer")}
		resolver := &handleSignalPeerResolver{t: w}
		w.incomingSessions = map[string]*signalIngress{"race-peer": {
			adoptedSession: sess,
			resolvers:      map[*handleSignalPeerResolver]struct{}{resolver: {}},
		}}

		start := make(chan struct{})
		taken := make(chan *session, 1)
		closeDone := make(chan struct{})
		go func() {
			<-start
			taken <- w.takeAdoptableSession("race-peer", nil)
		}()
		go func() {
			defer close(closeDone)
			<-start
			w.closeSignalIngress("race-peer", resolver)
		}()
		close(start)

		got := <-taken
		<-closeDone
		if got != nil {
			// The adopter won: the disposer observed an empty stash and left
			// the connection open for the adopted negotiation.
			if got.pc != pc {
				t.Fatal("adopter received a foreign session")
			}
			if second := w.takeAdoptableSession("race-peer", nil); second != nil {
				t.Fatal("stash was handed over twice")
			}
			_ = pc.Close()
		} else if pc.ConnectionState() != pion_webrtc.PeerConnectionStateClosed {
			t.Fatalf("iteration %d: disposer won but did not close the connection", i)
		}
	}
}
