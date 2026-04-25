package s4wave_session

import (
	"strings"
	"testing"
)

// realisticSDP is a full SDP offer from a browser WebRTC session including
// media-specific attributes that are not needed for data-channel-only use.
const realisticSDP = "v=0\r\n" +
	"o=- 4611731400430051336 2 IN IP4 127.0.0.1\r\n" +
	"s=-\r\n" +
	"t=0 0\r\n" +
	"a=group:BUNDLE 0\r\n" +
	"a=extmap-allow-mixed\r\n" +
	"a=msid-semantic: WMS\r\n" +
	"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n" +
	"c=IN IP4 0.0.0.0\r\n" +
	"a=ice-ufrag:abcd\r\n" +
	"a=ice-pwd:efghijklmnopqrstuvwxyz12\r\n" +
	"a=ice-options:trickle\r\n" +
	"a=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99\r\n" +
	"a=setup:actpass\r\n" +
	"a=mid:0\r\n" +
	"a=sctp-port:5000\r\n" +
	"a=max-message-size:262144\r\n" +
	"a=candidate:1 1 udp 2130706431 192.168.1.100 50000 typ host\r\n" +
	"a=candidate:2 1 udp 1694498815 203.0.113.5 50001 typ srflx raddr 192.168.1.100 rport 50000\r\n" +
	"a=end-of-candidates\r\n"

func TestMinifySDP(t *testing.T) {
	result := MinifySDP(realisticSDP)
	if result == "" {
		t.Fatal("minified SDP is empty")
	}
	if len(result) >= len(realisticSDP) {
		t.Errorf("minified SDP (%d bytes) not smaller than original (%d bytes)", len(result), len(realisticSDP))
	}
	t.Logf("original: %d bytes, minified: %d bytes (%.0f%% reduction)",
		len(realisticSDP), len(result),
		100*(1-float64(len(result))/float64(len(realisticSDP))))

	// Essential fields must be preserved.
	for _, required := range []string{
		"v=0",
		"o=-",
		"s=-",
		"t=0 0",
		"a=group:BUNDLE",
		"a=ice-ufrag:",
		"a=ice-pwd:",
		"a=fingerprint:",
		"a=setup:",
		"a=mid:",
		"a=sctp-port:",
		"a=max-message-size:",
		"a=candidate:",
		"a=end-of-candidates",
		"m=application",
		"c=IN IP4",
	} {
		if !strings.Contains(result, required) {
			t.Errorf("minified SDP missing required field %q", required)
		}
	}

	// Non-essential fields must be stripped.
	for _, stripped := range []string{
		"a=extmap-allow-mixed",
		"a=msid-semantic",
	} {
		if strings.Contains(result, stripped) {
			t.Errorf("minified SDP should not contain %q", stripped)
		}
	}
}

func TestMinifySDP_PreservesCandidates(t *testing.T) {
	result := MinifySDP(realisticSDP)
	count := strings.Count(result, "a=candidate:")
	if count != 2 {
		t.Errorf("expected 2 candidates, got %d", count)
	}
}

func TestMinifySDP_EmptyInput(t *testing.T) {
	result := MinifySDP("")
	if result != "" {
		t.Errorf("expected empty output for empty input, got %q", result)
	}
}

func TestMinifySDP_NoMatchingLines(t *testing.T) {
	sdp := "a=rtpmap:111 opus/48000/2\r\na=fmtp:111 minptime=10;useinbandfec=1\r\n"
	result := MinifySDP(sdp)
	if result != sdp {
		t.Errorf("expected original SDP returned when no lines match, got %q", result)
	}
}
