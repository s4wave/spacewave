package sobject

import (
	"context"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
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

func TestResolveSharedObjectRecoveryMaterial(t *testing.T) {
	// Build a provider-backed recovery fixture.
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

	// Resolve recovery material through the provider feature.
	got, err := ResolveSharedObjectRecoveryMaterial(ctx, provAcc, &SharedObjectRef{})
	if err != nil {
		t.Fatalf("ResolveSharedObjectRecoveryMaterial: %v", err)
	}

	// Verify the recovered entity identity.
	if got.GetEntityId() != expected.GetEntityId() {
		t.Fatalf("expected entity id %q, got %q", expected.GetEntityId(), got.GetEntityId())
	}
}
