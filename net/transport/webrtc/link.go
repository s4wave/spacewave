package webrtc

import (
	"github.com/pion/webrtc/v4"
	"github.com/s4wave/spacewave/net/link"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
)

// Link is a QUIC link carried by a WebRTC data channel. It reports whether its
// peer connection reaches the remote peer directly or through a relay.
type Link struct {
	*transport_quic.Link

	pc *webrtc.PeerConnection
}

// GetPath returns the path of the peer connection's selected candidate pair,
// or PathUnknown before ICE selects one.
func (l *Link) GetPath() link.Path {
	return candidatePairPath(l.pc)
}

// candidatePairPath classifies the selected candidate pair of pc.
func candidatePairPath(pc *webrtc.PeerConnection) link.Path {
	// Find the ICE transport under the data channel's association.
	ice := iceTransport(pc)
	if ice == nil {
		return link.PathUnknown
	}

	// Read the pair ICE selected; there is none before a check succeeds.
	pair, err := ice.GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
		return link.PathUnknown
	}

	// Either side using a relay candidate makes the pair relayed.
	if pair.Local.Typ == webrtc.ICECandidateTypeRelay || pair.Remote.Typ == webrtc.ICECandidateTypeRelay {
		return link.PathRelay
	}
	return link.PathDirect
}

// iceTransport returns the ICE transport under pc's SCTP association, or nil
// before the browser build starts the association.
func iceTransport(pc *webrtc.PeerConnection) *webrtc.ICETransport {
	// Reach the SCTP association.
	sctp := pc.SCTP()
	if sctp == nil {
		return nil
	}

	// Reach the DTLS transport under it, then its ICE transport.
	dtls := sctp.Transport()
	if dtls == nil {
		return nil
	}
	return dtls.ICETransport()
}

// _ is a type assertion.
var (
	_ link.MessageLink = (*Link)(nil)
	_ link.PathLink    = (*Link)(nil)
)
