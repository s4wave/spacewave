package sobject

import (
	"context"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

type fakeSharedObjectRecoveryProvider struct {
	entityID string
	env      *SOEntityRecoveryEnvelope
	dec      SharedObjectRecoveryDecoder
}

func (p *fakeSharedObjectRecoveryProvider) GetSelfEntityID(ctx context.Context) (string, error) {
	return p.entityID, ctx.Err()
}

func (p *fakeSharedObjectRecoveryProvider) ReadSharedObjectRecoveryEnvelope(ctx context.Context, ref *SharedObjectRef) (*SOEntityRecoveryEnvelope, error) {
	return p.env, ctx.Err()
}

func (p *fakeSharedObjectRecoveryProvider) GetSharedObjectRecoveryDecoder(ctx context.Context) (SharedObjectRecoveryDecoder, error) {
	return p.dec, ctx.Err()
}

type fakeSharedObjectRecoveryDecoder struct {
	material *SOEntityRecoveryMaterial
	err      error
}

func (d *fakeSharedObjectRecoveryDecoder) DecryptSharedObjectRecoveryEnvelope(ctx context.Context, env *SOEntityRecoveryEnvelope) (*SOEntityRecoveryMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d.material, d.err
}

type fakeProviderAccount struct {
	recoveryProv SharedObjectRecoveryProvider
}

func (a *fakeProviderAccount) GetProviderAccountFeature(ctx context.Context, feature provider.ProviderFeature) (provider.ProviderAccountFeature, error) {
	if feature == provider.ProviderFeature_ProviderFeature_SHARED_OBJECT_RECOVERY {
		return a.recoveryProv, nil
	}
	return nil, provider.ErrUnimplementedProviderFeature
}

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

func TestResolveSharedObjectRecoveryMaterial(t *testing.T) {
	ctx := context.Background()
	entityID := "entity-1"
	expected := &SOEntityRecoveryMaterial{
		EntityId: entityID,
		Role:     SOParticipantRole_SOParticipantRole_WRITER,
	}
	provAcc := &fakeProviderAccount{
		recoveryProv: &fakeSharedObjectRecoveryProvider{
			entityID: entityID,
			env: &SOEntityRecoveryEnvelope{
				EntityId: entityID,
			},
			dec: &fakeSharedObjectRecoveryDecoder{
				material: expected,
			},
		},
	}

	got, err := ResolveSharedObjectRecoveryMaterial(ctx, provAcc, &SharedObjectRef{})
	if err != nil {
		t.Fatalf("ResolveSharedObjectRecoveryMaterial: %v", err)
	}
	if got.GetEntityId() != expected.GetEntityId() {
		t.Fatalf("expected entity id %q, got %q", expected.GetEntityId(), got.GetEntityId())
	}
}
