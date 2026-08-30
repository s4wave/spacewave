package webrtc

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

func TestAllPeersChoosesOneOfferer(t *testing.T) {
	first, second := peer.ID("first-peer"), peer.ID("second-peer")
	if bytes.Compare([]byte(first), []byte(second)) > 0 {
		first, second = second, first
	}
	lower := &WebRTC{peerID: first, conf: &Config{AllPeers: true, AllPeersLowerPeerOffers: true}}
	upper := &WebRTC{peerID: second, conf: &Config{AllPeers: true, AllPeersLowerPeerOffers: true}}
	got, err := lower.GetPeerDialer(t.Context(), second)
	if err != nil || got == nil {
		t.Fatalf("lower peer did not offer: opts=%v err=%v", got, err)
	}
	got, err = upper.GetPeerDialer(t.Context(), first)
	if err != nil || got != nil {
		t.Fatalf("upper peer also offered: opts=%v err=%v", got, err)
	}

	explicit := &dialer.DialerOpts{Address: "explicit"}
	upper.conf.Dialers = map[string]*dialer.DialerOpts{first.String(): explicit}
	got, err = upper.GetPeerDialer(t.Context(), first)
	if err != nil || got != explicit {
		t.Fatalf("explicit dialer was not honored: opts=%v err=%v", got, err)
	}
}
