package webrtc

import (
	"errors"
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
