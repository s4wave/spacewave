package webrtc

import (
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
)

func TestStaleSignalingGenerationRejectedBeforeFreshPeerConnection(t *testing.T) {
	firstOffer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeOffer, SDP: "v=0\r\no=first\r\n"}
	firstID := OfferID(firstOffer.SDP)
	firstAnswer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeAnswer, SDP: "v=0\r\no=first-answer\r\n"}
	firstSDP := NewWebRtcSdp(1, &firstAnswer, firstID)
	mline := uint16(0)
	firstICE, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		Candidate:     "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host",
		SDPMLineIndex: &mline,
	}, firstID)
	if err != nil {
		t.Fatal(err)
	}
	firstEOC, err := NewWebRtcIce(
		&pion_webrtc.ICECandidateInit{SDPMLineIndex: &mline},
		firstID,
	)
	if err != nil {
		t.Fatal(err)
	}

	freshOffer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeOffer, SDP: "v=0\r\no=fresh\r\n"}
	freshID := OfferID(freshOffer.SDP)
	sess := &session{signaling: sessionSignalingState{offerID: freshID, offerEstablished: true}}

	if admitted, _ := sess.admitRemoteSDP(firstSDP); admitted {
		t.Fatal("stale answer reached the fresh PeerConnection generation")
	}
	if sess.admitsRemoteICE(firstICE) {
		t.Fatal("stale ICE reached the fresh PeerConnection generation")
	}
	if sess.admitsRemoteICE(firstEOC) {
		t.Fatal("stale end-of-candidates reached the fresh PeerConnection generation")
	}

	freshAnswer := pion_webrtc.SessionDescription{Type: pion_webrtc.SDPTypeAnswer, SDP: "v=0\r\no=fresh-answer\r\n"}
	if admitted, _ := sess.admitRemoteSDP(NewWebRtcSdp(2, &freshAnswer, freshID)); !admitted {
		t.Fatal("fresh answer was rejected")
	}

	if sess.beginsLocalOffer(freshID) {
		t.Fatal("retransmitted identical offer reset the active generation")
	}
	other := OfferID("v=0\r\no=other\r\n")
	if !sess.beginsLocalOffer(other) {
		t.Fatal("changed offer did not start a fresh generation")
	}
}
