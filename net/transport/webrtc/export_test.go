package webrtc

import (
	pion "github.com/pion/webrtc/v4"
	"github.com/s4wave/spacewave/net/link"
)

// LinkPeerConnection returns the PeerConnection carrying lnk, or nil when lnk
// is not a WebRTC link.
func LinkPeerConnection(lnk link.Link) *pion.PeerConnection {
	if l, ok := lnk.(*Link); ok {
		return l.pc
	}
	return nil
}
