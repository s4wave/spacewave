//go:build js

package webrtc

// These tests pin the signal-ingress generation-fence decisions that compile
// and run under GOOS=js GOARCH=wasm, the build the transport actually runs in
// browsers. They cover isGenerationFencedBody and the digest admission rules
// in sessionTracker.ingestRemoteSignal against real Pion peer connections.
// The fencedEra arm/drop switch inside deliverSignal stays unexercised here:
// it sits behind acquireSignalIngressLocked and the keyed sessionTracker
// executor built by NewWebRTC, which cannot be driven without either source
// changes or rebuilding that owner state in a test.

import (
	"crypto/sha256"
	"testing"

	pion_webrtc "github.com/pion/webrtc/v4"
)

// TestSignalIngressJsBodyFence pins the body classification that decides
// whether a signal carries offer generation material at all.
func TestSignalIngressJsBodyFence(t *testing.T) {
	cases := []struct {
		name string
		sig  *WebRtcSignal
		want bool
	}{
		{
			name: "sdp_carries_generation_material",
			sig:  &WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{}}},
			want: true,
		},
		{
			name: "ice_carries_generation_material",
			sig:  &WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: &WebRtcIce{}}},
			want: true,
		},
		{
			name: "request_offer_is_exempt",
			sig:  &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 1}},
			want: false,
		},
		{
			name: "empty_body_is_not_fenced",
			sig:  &WebRtcSignal{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGenerationFencedBody(tc.sig); got != tc.want {
				t.Fatalf("isGenerationFencedBody(%v) = %v, want %v", tc.sig, got, tc.want)
			}
		})
	}
}

// TestSignalIngressJsDigestFence drives ingestRemoteSignal under js/wasm with
// real Pion peer connections and pins the digest admission decisions: an
// offer whose digest matches its SDP bytes admits, a mismatched digest drops
// before Pion state changes, and candidates must carry the active generation.
func TestSignalIngressJsDigestFence(t *testing.T) {
	t.Run("matching_offer_digest_admits", func(t *testing.T) {
		answerPC, offerDesc := newOfferForAnswerer(t)

		f := &fenceIngest{
			tracker: &sessionTracker{
				w:       &WebRTC{conf: &Config{}},
				le:      newFenceTestLogger(),
				offerer: false,
			},
			sess: &session{pc: answerPC},
			applier: &remoteICECandidateApplier{
				add: func(pion_webrtc.ICECandidateInit) error { return nil },
			},
		}
		if err := f.ingest(&WebRtcSdp{
			SdpType: "offer",
			Sdp:     offerDesc.SDP,
			OfferId: offerDigest(offerDesc.SDP),
		}, nil); err != nil {
			t.Fatal(err.Error())
		}
		if answerPC.RemoteDescription() == nil {
			t.Fatalf("matching-generation offer was dropped: remote description missing")
		}
		if string(f.sess.rxOfferID) != string(offerDigest(offerDesc.SDP)) {
			t.Fatalf("accepted offer did not record its active generation digest")
		}
	})

	t.Run("mismatched_offer_digest_drops", func(t *testing.T) {
		answerPC, offerDesc := newOfferForAnswerer(t)

		f := &fenceIngest{
			tracker: &sessionTracker{
				w:       &WebRTC{conf: &Config{}},
				le:      newFenceTestLogger(),
				offerer: false,
			},
			sess: &session{pc: answerPC},
			applier: &remoteICECandidateApplier{
				add: func(pion_webrtc.ICECandidateInit) error { return nil },
			},
		}
		staleSum := sha256.Sum256([]byte("retired-js-generation"))
		err := f.ingest(&WebRtcSdp{
			SdpType: "offer",
			Sdp:     offerDesc.SDP,
			OfferId: staleSum[:],
		}, nil)
		if err != nil {
			t.Fatalf("mismatched-digest offer returned %v, want silent drop", err)
		}
		if answerPC.RemoteDescription() != nil {
			t.Fatalf("mismatched-digest offer reached Pion")
		}
		if len(f.sess.rxOfferID) != 0 {
			t.Fatalf("dropped offer recorded a generation digest")
		}
	})

	t.Run("candidate_matching_active_generation_applies", func(t *testing.T) {
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
		err := f.ingest(nil, newTestIceSignal(t, "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host", offerDigest(offerDesc.SDP)))
		if err != nil {
			t.Fatalf("active-generation candidate returned %v", err)
		}
		if applied != 1 {
			t.Fatalf("active-generation candidate applied %d times, want 1", applied)
		}
	})

	t.Run("candidate_with_other_digest_drops", func(t *testing.T) {
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
		otherSum := sha256.Sum256([]byte("other-js-generation"))
		err := f.ingest(nil, newTestIceSignal(t, "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host", otherSum[:]))
		if err != nil {
			t.Fatalf("stale candidate returned %v, want silent drop", err)
		}
		if applied != 0 {
			t.Fatalf("stale-generation candidate reached the applier: %d applied", applied)
		}
	})
}
