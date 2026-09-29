package pairing

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func TestResultRequiresDurableEnrollment(t *testing.T) {
	// Fail Result when no exchange is active.
	remote := newEngineTestPeer(t)
	engine := &Engine{}
	if _, err := engine.Result(remote); !errors.Is(err, ErrExchangeMissing) {
		t.Fatalf("missing exchange returned %v", err)
	}

	// Fail Result while the exchange is confirmed but not durably enrolled.
	engine.active = &attempt{snapshot: Snapshot{RemotePeerID: remote, Status: StatusBothConfirmed, Receiving: true}}
	if _, err := engine.Result(remote); !errors.Is(err, ErrExchangeUnconfirmed) {
		t.Fatalf("completed status without enrollment returned %v", err)
	}

	// Fail Result for a peer other than the confirmed exchange.
	if _, err := engine.Result(newEngineTestPeer(t)); !errors.Is(err, ErrExchangePeerMismatch) {
		t.Fatalf("different peer returned %v", err)
	}

	// Return an independent copy of the durably enrolled Session.
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: "account", Id: "session"}}
	engine.active.result = ref
	result, err := engine.Result(remote)
	if err != nil {
		t.Fatal(err)
	}
	if !result.EqualVT(ref) || result == ref {
		t.Fatal("result did not return an independent copy of the enrolled Session")
	}
}

func newEngineTestPeer(t *testing.T) peer.ID {
	// Generate an Ed25519 keypair and derive its peer ID.
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRetainedPairingResources releases both the prepared enrollment and the
// final migrated Session, including a late result from a replaced attempt.
func TestRetainedPairingResources(t *testing.T) {
	// Start an offering attempt and retain two resources under it.
	engine := &Engine{ctx: t.Context()}
	_, active := engine.begin(true, true, "", "", "", StatusPeerConnected)
	var released int
	for range 2 {
		if !engine.retain(active, func() { released++ }) {
			t.Fatal("active pairing did not retain its resource")
		}
	}

	// Clear the engine and require both retained resources released.
	engine.Clear()
	if released != 2 {
		t.Fatalf("released %d of 2 pairing resources", released)
	}

	// Reject a late retain on the replaced attempt.
	if engine.retain(active, func() { released++ }) || released != 3 {
		t.Fatal("replaced pairing retained a late resource")
	}
}
