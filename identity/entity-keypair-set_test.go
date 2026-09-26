package identity

import (
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	uuid "github.com/satori/go.uuid"
)

// buildTestEntity builds a valid entity with one keypair.
func buildTestEntity(t *testing.T) (*Entity, crypto.PrivKey) {
	t.Helper()
	privKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ent, err := EntityWithPrivKey("test-domain", "test-entity", uuid.NewV4().String(), privKey, "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ent, privKey
}

// TestAppendKeypairDuplicate tests that appending an existing keypair errors.
func TestAppendKeypairDuplicate(t *testing.T) {
	ent, privKey := buildTestEntity(t)
	ekp, err := EntityKeypairWithPubKey("test-domain", "test-entity", privKey.GetPublic(), "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ent.AppendKeypair(privKey, ekp); err == nil {
		t.Fatal("expected error appending duplicate keypair")
	}
	if n := len(ent.GetEntityKeypairSet().GetEntityKeypairs()); n != 1 {
		t.Fatalf("expected 1 keypair but got %d", n)
	}
}

// TestUnmarshalVerifyKeypairsMissingPubKey tests a signature without a public key.
func TestUnmarshalVerifyKeypairsMissingPubKey(t *testing.T) {
	ent, _ := buildTestEntity(t)
	sigs := ent.GetEntityKeypairSet().GetEntityKeypairSignatures()
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signature but got %d", len(sigs))
	}
	sigs[0].PubKey = nil
	if _, err := ent.UnmarshalVerifyKeypairs(); err == nil {
		t.Fatal("expected error for missing signature pubkey")
	}
}
