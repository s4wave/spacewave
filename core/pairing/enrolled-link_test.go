package pairing

import (
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestEnrolledLinkIdentityBinding proves that the independently enrolled key
// can replace its setup identity only on the connection named by both proofs.
func TestEnrolledLinkIdentityBinding(t *testing.T) {
	// Generate the enrolled key, storage key, and their peer IDs.
	source, receiving := newEngineTestPeer(t), newEngineTestPeer(t)

	// Generate the enrolled key and its independent storage key.
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	storage, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	// Build the offer and identity over both peers.
	offer := &AccountOffer{ProviderId: "local", AccountId: "account", OperationId: "operation"}
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: "account", Id: "session"}}
	identity, err := BuildIdentity(offer, ref, key, storage, source, receiving)
	if err != nil {
		t.Fatal(err)
	}

	// Bind the link from each side and require the mapped peers.
	for _, localSource := range []bool{true, false} {
		// Bind the link from this side and require the mapped peers.
		connection := &enrolledLink{local: source, remote: receiving}
		wantLocal, wantRemote := source, enrolled
		if !localSource {
			connection.local, connection.remote = receiving, source
			wantLocal, wantRemote = enrolled, source
		}
		bound, err := BindEnrolledLink(connection, offer, identity, source, receiving)
		if err != nil {
			t.Fatal(err)
		}
		if bound.GetLocalPeer() != wantLocal || bound.GetRemotePeer() != wantRemote {
			t.Fatal("enrollment mapped the wrong authenticated peer")
		}

		// Reject a different operation, reversed roles, and another connection.
		changed := offer.CloneVT()
		changed.OperationId = "another operation"
		if _, err := BindEnrolledLink(connection, changed, identity, source, receiving); err == nil {
			t.Fatal("accepted another pairing operation")
		}
		if _, err := BindEnrolledLink(connection, offer, identity, receiving, source); err == nil {
			t.Fatal("accepted reversed proof roles")
		}
		connection.remote = newEngineTestPeer(t)
		if _, err := BindEnrolledLink(connection, offer, identity, source, receiving); err == nil {
			t.Fatal("accepted a proof on another authenticated connection")
		}
	}
}
