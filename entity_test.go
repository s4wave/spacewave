package identity

import (
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
	uuid "github.com/satori/go.uuid"
)

// TestBuildEntity tests creating an entity and adding some keypairs.
func TestBuildEntity(t *testing.T) {
	entityID := "test-entity"
	entityUUID := uuid.NewV4().String()
	domainID := "test-domain"

	ent := NewEntity(entityID, entityUUID, domainID)

	// generate 2 private keys + keypair objects
	p1, _ := peer.NewPeer(nil)
	p2, _ := peer.NewPeer(nil)
	kp1, err := EntityKeypairWithPubKey(
		entityID, domainID,
		p1.GetPubKey(),
		"", nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	kp2, err := EntityKeypairWithPubKey(
		entityID, domainID,
		p2.GetPubKey(),
		"", nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// append them
	err = ent.AppendKeypair(p1.GetPrivKey(), kp1)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = ent.AppendKeypair(p2.GetPrivKey(), kp2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify
	if err := ent.Validate(); err != nil {
		t.Fatal(err.Error())
	}

	// done
	t.Logf("successfully created entity with %d keypairs", len(ent.GetEntityKeypairs()))
}
