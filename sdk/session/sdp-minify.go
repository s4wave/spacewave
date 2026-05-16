package s4wave_session

import (
	"strings"
)

// sdpKeepPrefixes lists SDP line prefixes that are essential for a
// data-channel-only WebRTC connection.
var sdpKeepPrefixes = []string{
	"v=",
	"o=",
	"s=",
	"t=",
	"a=group:",
	"a=ice-ufrag:",
	"a=ice-pwd:",
	"a=ice-options:",
	"a=fingerprint:",
	"a=setup:",
	"a=mid:",
	"a=sctp-port:",
	"a=max-message-size:",
	"a=candidate:",
	"a=end-of-candidates",
	"m=application",
	"c=",
}

// MinifySDP strips SDP lines not needed for a data-channel-only WebRTC
// connection. Preserves session-level fields, ICE credentials, DTLS
// fingerprint, and candidate lines. Drops bandwidth, ssrc, rtpmap, fmtp,
// extmap, rtcp, and other media-specific attributes.
func MinifySDP(sdp string) string {
	lines := strings.Split(sdp, "\r\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		for _, prefix := range sdpKeepPrefixes {
			if strings.HasPrefix(line, prefix) {
				kept = append(kept, line)
				break
			}
		}
	}
	if len(kept) == 0 {
		return sdp
	}
	return strings.Join(kept, "\r\n") + "\r\n"
}
