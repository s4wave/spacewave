package webrtc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"
)

func TestCompleteExecutionWaitsForChildCompletion(t *testing.T) {
	tpt := &WebRTC{}
	execution := &sessionTrackerExecution{}
	tkr := &sessionTracker{w: tpt, execution: execution}
	linkDone := make(chan struct{})
	xmitDone := make(chan struct{})
	completed := make(chan struct{})

	go func() {
		tkr.completeExecution(execution, linkDone, xmitDone)
		close(completed)
	}()

	linkDone <- struct{}{}
	var retired bool
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		retired = tkr.execution != execution
	})
	if retired {
		t.Fatal("execution retired before transmit child completed")
	}
	select {
	case <-completed:
		t.Fatal("execution completion returned before transmit child completed")
	default:
	}

	xmitDone <- struct{}{}
	<-completed
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if tkr.execution != nil {
			t.Error("execution remained published after both children completed")
		}
	})
}

func TestCreateDataChannelRegistersNegotiationCallbackFirst(t *testing.T) {
	tpt := &WebRTC{
		conf: &Config{},
		le:   logrus.NewEntry(logrus.New()),
	}
	tkr := &sessionTracker{
		w:       tpt,
		le:      tpt.le,
		offerer: false,
	}
	sess := &session{t: tkr}

	var onNegotiationNeeded func()
	_, err := sess.createDataChannel(
		func(cb func()) {
			onNegotiationNeeded = cb
		},
		func(string, *pion_webrtc.DataChannelInit) (*pion_webrtc.DataChannel, error) {
			if onNegotiationNeeded == nil {
				t.Fatal("data channel created before negotiation callback registration")
			}
			onNegotiationNeeded()
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	var localSeqno uint64
	sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		localSeqno = sess.localSeqno
	})
	if localSeqno != 1 {
		t.Fatalf("local sequence %d, want 1", localSeqno)
	}

	var signals []*WebRtcSignal
	xmit := func(sig *WebRtcSignal) {
		signals = append(signals, sig)
	}
	lastLocalSeqno, transmitted, err := tkr.transmitLocalNegotiation(
		sess,
		tkr.le,
		localSeqno,
		0,
		xmit,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !transmitted {
		t.Fatal("initial request_offer was not transmitted")
	}
	if lastLocalSeqno != 1 {
		t.Fatalf("last local sequence %d, want 1", lastLocalSeqno)
	}

	lastLocalSeqno, transmitted, err = tkr.transmitLocalNegotiation(
		sess,
		tkr.le,
		localSeqno,
		lastLocalSeqno,
		xmit,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if transmitted {
		t.Fatal("unchanged local sequence retransmitted request_offer")
	}
	if lastLocalSeqno != 1 {
		t.Fatalf("last local sequence %d after recheck, want 1", lastLocalSeqno)
	}
	if len(signals) != 1 {
		t.Fatalf("request_offer count %d, want 1", len(signals))
	}
	if signals[0].GetRequestOffer() != 1 {
		t.Fatalf("request_offer sequence %d, want 1", signals[0].GetRequestOffer())
	}
}

func TestRemoteICECandidateApplierStopsAtCompletion(t *testing.T) {
	mlineIndex := uint16(0)
	candidates := []pion_webrtc.ICECandidateInit{
		{Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host"},
		{SDPMLineIndex: &mlineIndex},
		{Candidate: "candidate:2 1 udp 2130706431 192.0.2.2 5001 typ host"},
	}
	var applied []pion_webrtc.ICECandidateInit
	applier := remoteICECandidateApplier{add: func(candidate pion_webrtc.ICECandidateInit) error {
		applied = append(applied, candidate)
		return nil
	}}
	err := applier.apply(candidates)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(applied) != 2 {
		t.Fatalf("applied candidate count %d, want 2", len(applied))
	}
	if applied[0].Candidate == "" {
		t.Fatal("end-of-candidates applied before the candidate")
	}
	if applied[1].Candidate != "" {
		t.Fatal("final application was not end-of-candidates")
	}
}

func TestRemoteICECandidateApplierPropagatesFailure(t *testing.T) {
	wantErr := errors.New("candidate rejected")
	candidates := []pion_webrtc.ICECandidateInit{
		{Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host"},
		{},
		{Candidate: "candidate:2 1 udp 2130706431 192.0.2.2 5001 typ host"},
	}
	var calls int
	applier := remoteICECandidateApplier{add: func(candidate pion_webrtc.ICECandidateInit) error {
		calls++
		if candidate.Candidate == "" {
			return wantErr
		}
		return nil
	}}
	err := applier.apply(candidates)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Fatalf("AddICECandidate calls %d, want 2", calls)
	}
}

func TestSessionOfferIdentityAdmission(t *testing.T) {
	offer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeOffer, SDP: "v=0\r\no=fresh\r\n"}
	offerID := OfferID(offer.SDP)
	sdp := NewWebRtcSdp(1, &offer, offerID)
	sess := new(session)

	admitted, fresh := sess.admitRemoteSDP(sdp)
	if !admitted || !fresh {
		t.Fatalf("first offer admission = %v, fresh = %v", admitted, fresh)
	}
	sess.beginOfferGeneration(offerID)
	admitted, fresh = sess.admitRemoteSDP(sdp)
	if !admitted || fresh {
		t.Fatalf("duplicate offer admission = %v, fresh = %v", admitted, fresh)
	}

	mismatched := NewWebRtcSdp(2, &offer, [sha256.Size]byte{})
	if err := (&WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: mismatched},
	}).Validate(); err == nil {
		t.Fatal("mismatched offer identity validated")
	}
	missing := &WebRtcSdp{TxSeqno: 2, SdpType: offer.Type.String(), Sdp: offer.SDP}
	if err := (&WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: missing},
	}).Validate(); err == nil {
		t.Fatal("missing offer identity validated")
	}
}

func TestCorrelatedICEBuffersReordersAndCompletesWithinGeneration(t *testing.T) {
	offer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeOffer, SDP: "v=0\r\no=offer\r\n"}
	offerID := OfferID(offer.SDP)
	sess := &session{signaling: sessionSignalingState{offerID: offerID, offerEstablished: true}}
	mline := uint16(0)
	candidate, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		Candidate:     "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host",
		SDPMLineIndex: &mline,
	}, offerID)
	if err != nil {
		t.Fatal(err)
	}
	eoc, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{SDPMLineIndex: &mline}, offerID)
	if err != nil {
		t.Fatal(err)
	}
	staleID := OfferID("other offer")
	stale, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		Candidate:     "candidate:stale 1 udp 1 192.0.2.2 5001 typ host",
		SDPMLineIndex: &mline,
	}, staleID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.admitsRemoteICE(stale) {
		t.Fatal("stale reordered candidate entered the pending buffer")
	}
	if !sess.admitsRemoteICE(candidate) || !sess.admitsRemoteICE(eoc) {
		t.Fatal("active generation candidate or EOC was rejected")
	}

	// Candidate and EOC may arrive before the answer and are flushed in arrival
	// order after the correlated description. Duplicate and reordered material
	// after EOC cannot reopen the completed generation.
	var pending []pion_webrtc.ICECandidateInit
	for _, signal := range []*WebRtcIce{candidate, eoc} {
		ice, err := signal.ParseICECandidateInit()
		if err != nil {
			t.Fatal(err)
		}
		pending = append(pending, *ice)
	}
	var applied []pion_webrtc.ICECandidateInit
	applier := remoteICECandidateApplier{add: func(candidate pion_webrtc.ICECandidateInit) error {
		applied = append(applied, candidate)
		return nil
	}}
	if err := applier.apply(pending); err != nil {
		t.Fatal(err)
	}
	if err := applier.apply([]pion_webrtc.ICECandidateInit{pending[0]}); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 || applied[0].Candidate == "" || applied[1].Candidate != "" {
		t.Fatalf("applied correlated ICE sequence = %#v", applied)
	}

	freshOffer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeOffer, SDP: "v=0\r\no=fresh\r\n"}
	freshID := OfferID(freshOffer.SDP)
	admitted, fresh := sess.admitRemoteSDP(NewWebRtcSdp(2, &freshOffer, freshID))
	if !admitted || !fresh {
		t.Fatal("fresh offer did not replace the completed generation")
	}
	sess.beginOfferGeneration(freshID)
	if sess.admitsRemoteICE(eoc) {
		t.Fatal("old EOC remained admissible after fresh offer")
	}
}

func TestBeginOfferGenerationResetsAllCorrelatedStateAndEmitsOneFreshEOC(t *testing.T) {
	firstID := OfferID("first")
	freshID := OfferID("fresh")
	sess := &session{
		signaling: sessionSignalingState{
			offerID:              firstID,
			offerEstablished:     true,
			pendingRemoteICE:     []pion_webrtc.ICECandidateInit{{Candidate: "old"}},
			remoteDescriptionSet: true,
			lastAppliedRemoteSDP: "old",
			remoteICE:            remoteICECandidateApplier{complete: true},
			lastSentICE:          4,
			sentICEComplete:      true,
		},
		localIceCandidates:         []*pion_webrtc.ICECandidateInit{{Candidate: "old"}},
		localIceCandidatesComplete: true,
	}
	sess.beginOfferGeneration(freshID)

	state := &sess.signaling
	if state.offerID != freshID || !state.offerEstablished || len(state.pendingRemoteICE) != 0 ||
		state.remoteDescriptionSet || state.lastAppliedRemoteSDP != "" ||
		state.remoteICE.complete || state.lastSentICE != 0 || state.sentICEComplete ||
		len(sess.localIceCandidates) != 0 || sess.localIceCandidatesComplete {
		t.Fatalf("fresh generation retained stale state: %#v", state)
	}

	first, err := sess.nextLocalEndOfCandidates(true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sess.nextLocalEndOfCandidates(true)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || second != nil {
		t.Fatalf("EOC first=%v second=%v", first, second)
	}
	if !bytes.Equal(first.GetIce().GetOfferId(), freshID[:]) {
		t.Fatal("fresh EOC carried the prior offer identity")
	}
}

func TestPendingRemoteICECandidateLimitChecksNextCountBeforeAppend(t *testing.T) {
	candidate := pion_webrtc.ICECandidateInit{
		Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host",
	}
	sess := &session{signaling: sessionSignalingState{
		pendingRemoteICE: make([]pion_webrtc.ICECandidateInit, maxPendingRemoteICECandidates-1),
	}}
	if err := sess.bufferRemoteICE("x", candidate); err != nil {
		t.Fatalf("exact candidate limit rejected: %v", err)
	}
	if got := len(sess.signaling.pendingRemoteICE); got != maxPendingRemoteICECandidates {
		t.Fatalf("pending candidate count %d, want %d", got, maxPendingRemoteICECandidates)
	}
	if err := sess.bufferRemoteICE("x", candidate); err == nil {
		t.Fatal("candidate above limit was buffered")
	}
	if got := len(sess.signaling.pendingRemoteICE); got != maxPendingRemoteICECandidates {
		t.Fatalf("rejected candidate changed pending count to %d", got)
	}
}

func TestPendingRemoteICEByteLimitChecksNextSizeBeforeAppend(t *testing.T) {
	candidate := pion_webrtc.ICECandidateInit{
		Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host",
	}
	sess := new(session)
	if err := sess.bufferRemoteICE(strings.Repeat("x", maxPendingRemoteICEBytes), candidate); err != nil {
		t.Fatalf("exact byte limit rejected: %v", err)
	}
	if got := sess.signaling.pendingRemoteICEBytes; got != maxPendingRemoteICEBytes {
		t.Fatalf("pending bytes %d, want %d", got, maxPendingRemoteICEBytes)
	}
	if err := sess.bufferRemoteICE("x", candidate); err == nil {
		t.Fatal("candidate above byte limit was buffered")
	}
	if got := len(sess.signaling.pendingRemoteICE); got != 1 {
		t.Fatalf("rejected candidate changed pending count to %d", got)
	}
	if got := sess.signaling.pendingRemoteICEBytes; got != maxPendingRemoteICEBytes {
		t.Fatalf("rejected candidate changed pending bytes to %d", got)
	}
}
