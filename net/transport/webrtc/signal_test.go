package webrtc

import (
	"crypto/rand"
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
	spacecrypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const (
	testOfferSDP  = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
	testAnswerSDP = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
)

func signalingTestKeys(t *testing.T) (spacecrypto.PrivKey, spacecrypto.PubKey) {
	t.Helper()
	priv, pub, err := spacecrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func encryptRawSignal(
	t *testing.T,
	pub spacecrypto.PubKey,
	signal *WebRtcSignal,
) []byte {
	t.Helper()
	plaintext, err := signal.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := peer.EncryptToPubKey(pub, SignalingCryptContext, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return ciphertext
}

func TestSignalingRoundTrip(t *testing.T) {
	priv, pub := signalingTestKeys(t)
	signal := &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 7}}
	ciphertext, err := EncodeWebRtcSignal(signal, pub)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeWebRtcSignal(ciphertext, priv)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.GetRequestOffer() != 7 {
		t.Fatalf("request offer %d, want 7", decoded.GetRequestOffer())
	}
}

func TestSignalingRejectsMalformedGenerationMaterialBeforeIngress(t *testing.T) {
	priv, pub := signalingTestKeys(t)
	offerID := OfferID(testOfferSDP)
	for name, signal := range map[string]*WebRtcSignal{
		"missing-body": {},
		"missing-offer-id": {
			Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
				SdpType: "offer",
				Sdp:     testOfferSDP,
			}},
		},
		"mismatched-offer-id": {
			Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
				SdpType: "offer",
				Sdp:     testOfferSDP,
				OfferId: make([]byte, 32),
			}},
		},
		"malformed-sdp": {
			Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
				SdpType: "answer",
				Sdp:     "not sdp",
				OfferId: offerID[:],
			}},
		},
		"malformed-ice": {
			Body: &WebRtcSignal_Ice{Ice: &WebRtcIce{
				Candidate: "not json",
				OfferId:   offerID[:],
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			decoded, err := DecodeWebRtcSignal(encryptRawSignal(t, pub, signal), priv)
			if err == nil || decoded != nil {
				t.Fatalf("decoded=%v err=%v", decoded, err)
			}
		})
	}
}

func TestOfferIdentityHashesExactTransmittedSDP(t *testing.T) {
	offer := pion_webrtc.SessionDescription{
		Type: pion_webrtc.SDPTypeOffer,
		SDP:  testOfferSDP,
	}
	offerID := OfferID(offer.SDP)
	signal := &WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: NewWebRtcSdp(
			1,
			&offer,
			offerID,
		)},
	}
	if err := signal.Validate(); err != nil {
		t.Fatal(err)
	}

	signal.GetSdp().Sdp += "\r\n"
	if err := signal.Validate(); err == nil {
		t.Fatal("offer_id validated after the transmitted SDP bytes changed")
	}
}

func TestGenerationMaterialRequiresExactOfferIdentity(t *testing.T) {
	for name, body := range map[string]isWebRtcSignal_Body{
		"offer": &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
			SdpType: "offer",
			Sdp:     testOfferSDP,
		}},
		"answer": &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
			SdpType: "answer",
			Sdp:     testAnswerSDP,
		}},
		"candidate": &WebRtcSignal_Ice{Ice: &WebRtcIce{
			Candidate: `{}`,
		}},
		"end-of-candidates": &WebRtcSignal_Ice{Ice: &WebRtcIce{
			Candidate: `{"candidate":""}`,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			signal := &WebRtcSignal{
				Body: body,
			}
			if err := signal.Validate(); err == nil {
				t.Fatal("generation material without offer_id validated")
			}
			switch typed := signal.GetBody().(type) {
			case *WebRtcSignal_Sdp:
				typed.Sdp.OfferId = make([]byte, 31)
			case *WebRtcSignal_Ice:
				typed.Ice.OfferId = make([]byte, 33)
			}
			if err := signal.Validate(); err == nil {
				t.Fatal("generation material with non-32-byte offer_id validated")
			}
		})
	}
}

func TestSignalingCryptContextRejectsPriorSchema(t *testing.T) {
	priv, pub := signalingTestKeys(t)
	plaintext, err := (&WebRtcSignal{
		Body: &WebRtcSignal_RequestOffer{RequestOffer: 1},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := peer.EncryptToPubKey(
		pub,
		"github.com/s4wave/spacewave/net 2024-01-15 17:58:55 webrtc signaling",
		plaintext,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeWebRtcSignal(ciphertext, priv); err == nil || decoded != nil {
		t.Fatalf("prior schema decoded=%v err=%v", decoded, err)
	}
}

func TestICEValidationRequiresCandidateGrammarOrCanonicalEndOfCandidates(t *testing.T) {
	offerID := OfferID(testOfferSDP)
	mlineIndex := uint16(0)
	canonicalEOC, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		SDPMLineIndex: &mlineIndex,
	}, offerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := canonicalEOC.Validate(); err != nil {
		t.Fatalf("canonical end-of-candidates rejected: %v", err)
	}

	for name, candidate := range map[string]string{
		"empty-object":             `{}`,
		"null":                     `null`,
		"missing-candidate-prefix": `1 1 udp 2130706431 192.0.2.1 5000 typ host`,
		"invalid-grammar":          `candidate:not-an-ice-candidate`,
		"missing-eoc-index":        `{"candidate":""}`,
		"nonzero-eoc-index":        `{"candidate":"","sdpMLineIndex":1}`,
		"eoc-mid":                  `{"candidate":"","sdpMid":"0","sdpMLineIndex":0}`,
		"eoc-username-fragment":    `{"candidate":"","sdpMLineIndex":0,"usernameFragment":"ufrag"}`,
	} {
		t.Run(name, func(t *testing.T) {
			ice := &WebRtcIce{Candidate: candidate, OfferId: offerID[:]}
			if err := ice.Validate(); err == nil {
				t.Fatal("invalid ICE material validated")
			}
		})
	}

	candidate := &WebRtcIce{
		Candidate: `{"candidate":"candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host"}`,
		OfferId:   offerID[:],
	}
	if err := candidate.Validate(); err != nil {
		t.Fatalf("valid ICE candidate rejected: %v", err)
	}
}
