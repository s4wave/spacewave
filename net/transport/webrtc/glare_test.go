package webrtc

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// TestAllPeersAcceptsEitherInitiator preserves one-sided peer discovery while
// the negotiation owner still selects exactly one SDP offerer.
func TestAllPeersAcceptsEitherInitiator(t *testing.T) {
	// Verify either peer can initiate while negotiation chooses one offerer.
	first, second := peer.ID("first-peer"), peer.ID("second-peer")
	lower := &WebRTC{peerID: first, conf: &Config{AllPeers: true}}
	upper := &WebRTC{peerID: second, conf: &Config{AllPeers: true}}
	got, err := lower.GetPeerDialer(t.Context(), second)
	if err != nil || got == nil {
		t.Fatalf("first peer cannot initiate: opts=%v err=%v", got, err)
	}
	got, err = upper.GetPeerDialer(t.Context(), first)
	if err != nil || got == nil {
		t.Fatalf("second peer cannot initiate: opts=%v err=%v", got, err)
	}
	if isOfferer(first.String(), second.String()) == isOfferer(second.String(), first.String()) {
		t.Fatal("negotiation did not select exactly one SDP offerer")
	}

	// Verify explicit peer dialing options override the default policy.
	explicit := &dialer.DialerOpts{Address: "explicit"}
	upper.conf.Dialers = map[string]*dialer.DialerOpts{first.String(): explicit}
	got, err = upper.GetPeerDialer(t.Context(), first)
	if err != nil || got != explicit {
		t.Fatalf("explicit dialer was not honored: opts=%v err=%v", got, err)
	}
}
