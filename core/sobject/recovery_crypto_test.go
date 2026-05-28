package sobject

import (
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func TestBuildAndUnlockSOEntityRecoveryEnvelope(t *testing.T) {
	entityPriv, entityPub, err := bifrost_crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	entityPeerID, err := peer.IDFromPublicKey(entityPub)
	if err != nil {
		t.Fatalf("IDFromPublicKey: %v", err)
	}

	material := &SOEntityRecoveryMaterial{
		EntityId: entityPeerID.String(),
		Role:     SOParticipantRole_SOParticipantRole_OWNER,
		GrantInner: &SOGrantInner{
			TransformConf: &block_transform.Config{},
		},
	}
	cfg := &SharedObjectConfig{
		ConfigChainSeqno: 7,
		ConfigChainHash:  []byte("cfg-hash"),
	}

	env, err := BuildSOEntityRecoveryEnvelope(
		entityPeerID.String(),
		3,
		cfg,
		material,
		[]bifrost_crypto.PubKey{entityPub},
	)
	if err != nil {
		t.Fatalf("BuildSOEntityRecoveryEnvelope: %v", err)
	}

	got, err := UnlockSOEntityRecoveryEnvelope([]bifrost_crypto.PrivKey{entityPriv}, env)
	if err != nil {
		t.Fatalf("UnlockSOEntityRecoveryEnvelope: %v", err)
	}
	if got.GetEntityId() != material.GetEntityId() {
		t.Fatalf("expected entity id %q, got %q", material.GetEntityId(), got.GetEntityId())
	}
	if got.GetRole() != material.GetRole() {
		t.Fatalf("expected role %v, got %v", material.GetRole(), got.GetRole())
	}
}
