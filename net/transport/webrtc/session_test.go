package webrtc

import (
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"
)

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
