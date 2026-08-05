package identity

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	uuid "github.com/satori/go.uuid"
)

// TestBuildEntity tests creating an entity and adding some keypairs.
func TestBuildEntity(t *testing.T) {
	entityID := "test-entity"
	entityUUID := uuid.NewV4().String()
	domainID := "test-domain"

	ent := NewEntity(domainID, entityID, entityUUID)

	// Generate two private keys and their entity keypair records.
	p1, _ := peer.NewPeer(nil)
	p2, _ := peer.NewPeer(nil)
	kp1, err := EntityKeypairWithPubKey(
		domainID, entityID,
		p1.GetPubKey(),
		"", nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	kp2, err := EntityKeypairWithPubKey(
		domainID, entityID,
		p2.GetPubKey(),
		"", nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Append both signed keypairs to the entity.
	ctx := context.Background()
	p1Priv, err := p1.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = ent.AppendKeypair(p1Priv, kp1)
	if err != nil {
		t.Fatal(err.Error())
	}
	p2Priv, err := p2.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = ent.AppendKeypair(p2Priv, kp2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Validate the completed entity and its keypair signatures.
	if err := ent.Validate(); err != nil {
		t.Fatal(err.Error())
	}

	t.Logf("successfully created entity with %d keypairs", len(ent.GetEntityKeypairSet().GetEntityKeypairs()))
}
