package s4wave_session

import (
	"testing"
)

func TestLocalPairingOfferRoundTrip(t *testing.T) {
	offer := &LocalPairingOffer{
		Sdp:    "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=candidate:1 1 udp 2130706431 192.168.1.100 50000 typ host\r\na=ice-ufrag:abcd\r\na=ice-pwd:efghijklmnopqrstuvwxyz\r\na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99\r\na=setup:actpass\r\na=mid:0\r\na=sctp-port:5000\r\n",
		PeerId: "QmTestPeerIdBase58Encoded",
	}

	encoded, err := EncodeLocalPairingOffer(offer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded string is empty")
	}
	t.Logf("encoded offer length: %d chars (from %d byte SDP)", len(encoded), len(offer.Sdp))

	decoded, err := DecodeLocalPairingOffer(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.GetSdp() != offer.GetSdp() {
		t.Errorf("SDP mismatch:\n  got:  %q\n  want: %q", decoded.GetSdp(), offer.GetSdp())
	}
	if decoded.GetPeerId() != offer.GetPeerId() {
		t.Errorf("peer_id mismatch: got %q, want %q", decoded.GetPeerId(), offer.GetPeerId())
	}
}

func TestLocalPairingAnswerRoundTrip(t *testing.T) {
	answer := &LocalPairingAnswer{
		Sdp:    "v=0\r\no=- 654321 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=candidate:1 1 udp 2130706431 10.0.0.50 40000 typ host\r\na=ice-ufrag:wxyz\r\na=ice-pwd:abcdefghijklmnopqrstuv\r\na=fingerprint:sha-256 11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00\r\na=setup:active\r\na=mid:0\r\na=sctp-port:5000\r\n",
		PeerId: "QmAnotherPeerIdBase58",
	}

	encoded, err := EncodeLocalPairingAnswer(answer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeLocalPairingAnswer(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.GetSdp() != answer.GetSdp() {
		t.Errorf("SDP mismatch:\n  got:  %q\n  want: %q", decoded.GetSdp(), answer.GetSdp())
	}
	if decoded.GetPeerId() != answer.GetPeerId() {
		t.Errorf("peer_id mismatch: got %q, want %q", decoded.GetPeerId(), answer.GetPeerId())
	}
}

func TestDecodeLocalPairingOffer_InvalidBase58(t *testing.T) {
	_, err := DecodeLocalPairingOffer("not-valid-base58!@#$")
	if err == nil {
		t.Fatal("expected error for invalid base58")
	}
}

func TestDecodeLocalPairingOffer_InvalidCompressedPayload(t *testing.T) {
	_, err := DecodeLocalPairingOffer("3yzbKBqMvjdGhDL5P") // valid base58, invalid flate
	if err == nil {
		t.Fatal("expected error for invalid compressed payload")
	}
}
