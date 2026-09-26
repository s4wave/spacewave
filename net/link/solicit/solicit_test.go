package link_solicit

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

// TestSolicitProtocolEquivalenceTransportID verifies the transport constraint
// distinguishes otherwise identical directives.
func TestSolicitProtocolEquivalenceTransportID(t *testing.T) {
	pid := protocol.ID("test/proto")
	a := NewSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 1).(*solicitProtocol)
	if !a.IsEquivalent(NewSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 1)) {
		t.Fatal("identical directives should be equivalent")
	}
	if a.IsEquivalent(NewSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 2)) {
		t.Fatal("directives with different transport ids should not be equivalent")
	}
}
