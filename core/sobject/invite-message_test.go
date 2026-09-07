package sobject

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestInviteTransportSignature preserves the storage signer while authenticating
// a distinct session endpoint, and rejects endpoint tampering before token use.
func TestInviteTransportSignature(t *testing.T) {
	owner, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := owner.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	invite, _, err := BuildSOInviteMessage("space", key, SOParticipantRole_SOParticipantRole_WRITER, "local", "", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := invite.VerifyTransportPeer()
	if err != nil || resolved != owner.GetPeerID() {
		t.Fatalf("original invitation route: %s, %v", resolved, err)
	}

	// The owner authorizes a separately authenticated transport endpoint.
	invite.TransportPeerId = endpoint.GetPeerID().String()
	if err := invite.Sign(key); err != nil {
		t.Fatal(err)
	}
	resolved, err = invite.VerifyTransportPeer()
	if err != nil || resolved != endpoint.GetPeerID() {
		t.Fatalf("session invitation route: %s, %v", resolved, err)
	}
	if invite.GetOwnerPeerId() != owner.GetPeerID().String() {
		t.Fatal("transport route changed the Space signer")
	}
	invite.TransportPeerId = owner.GetPeerID().String()
	if _, err := invite.VerifyTransportPeer(); err == nil {
		t.Fatal("tampered endpoint accepted")
	}
}
