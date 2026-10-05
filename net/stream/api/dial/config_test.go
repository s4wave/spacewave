package stream_api_dial

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestConfigRefusesLocalPeer checks that a dial to the local peer fails
// validation instead of waiting for a link that cannot form.
func TestConfigRefusesLocalPeer(t *testing.T) {
	// Dial a peer as itself.
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := p.GetPeerID().String()
	conf := &Config{PeerId: id, LocalPeerId: id, ProtocolId: "test/1"}
	if err := conf.Validate(); err == nil {
		t.Fatal("expected a dial to the local peer to fail validation")
	}
}
