package identity

import (
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	uuid "github.com/satori/go.uuid"
)

// buildTestEntity builds a valid entity with one keypair.
func buildTestEntity(t *testing.T) (*Entity, crypto.PrivKey) {
	// Generate the private key for the test entity.
	t.Helper()
	privKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the test entity with one signed keypair.
	ent, err := EntityWithPrivKey("test-domain", "test-entity", uuid.NewV4().String(), privKey, "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	return ent, privKey
}

// TestAppendKeypairDuplicate tests that appending an existing keypair errors.
func TestAppendKeypairDuplicate(t *testing.T) {
	// Reconstruct the keypair already stored in the test entity.
	ent, privKey := buildTestEntity(t)
	ekp, err := EntityKeypairWithPubKey("test-domain", "test-entity", privKey.GetPublic(), "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the entity rejects the duplicate and keeps its original keypair.
	if err := ent.AppendKeypair(privKey, ekp); err == nil {
		t.Fatal("expected error appending duplicate keypair")
	}
	if n := len(ent.GetEntityKeypairSet().GetEntityKeypairs()); n != 1 {
		t.Fatalf("expected 1 keypair but got %d", n)
	}
}

// TestUnmarshalVerifyKeypairsMissingPubKey tests a signature without a public key.
func TestUnmarshalVerifyKeypairsMissingPubKey(t *testing.T) {
	// Build an entity with one signature whose public key can be removed.
	ent, _ := buildTestEntity(t)
	sigs := ent.GetEntityKeypairSet().GetEntityKeypairSignatures()
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signature but got %d", len(sigs))
	}

	// Remove the public key from the entity signature.
	sigs[0].PubKey = nil

	// Verify that the entity rejects a signature without its public key.
	if _, err := ent.UnmarshalVerifyKeypairs(); err == nil {
		t.Fatal("expected error for missing signature pubkey")
	}
}
