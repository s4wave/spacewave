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

// TestUniqueSolicitProtocolNeverEquivalent verifies a unique offer is merged
// with neither an identical plain offer nor another unique offer, in either
// direction, so each one gets its own incarnation.
func TestUniqueSolicitProtocolNeverEquivalent(t *testing.T) {
	pid := protocol.ID("test/proto")
	plain := NewSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 0).(*solicitProtocol)
	unique := NewUniqueSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 0).(*solicitProtocol)
	if unique.IsEquivalent(plain) || plain.IsEquivalent(unique) {
		t.Fatal("a unique offer should not be equivalent to a plain one")
	}
	if unique.IsEquivalent(NewUniqueSolicitProtocol(pid, []byte("ctx"), peer.ID("peer"), 0)) {
		t.Fatal("two unique offers should not be equivalent")
	}
	if unique.IsEquivalent(unique) {
		t.Fatal("a unique offer should not be equivalent even to itself")
	}
}
