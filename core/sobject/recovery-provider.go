package sobject

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
)

// SharedObjectRecoveryProvider implements ProviderFeature_SHARED_OBJECT_RECOVERY.
type SharedObjectRecoveryProvider interface {
	provider.ProviderAccountFeature

	// GetSelfEntityID returns the stable entity ID for the mounted provider account.
	GetSelfEntityID(ctx context.Context) (string, error)

	// ReadSharedObjectRecoveryEnvelope reads the current recovery envelope for the entity on an SO.
	ReadSharedObjectRecoveryEnvelope(ctx context.Context, ref *SharedObjectRef) (*SOEntityRecoveryEnvelope, error)

	// GetSharedObjectRecoveryDecoder returns a provider-owned decoder for recovery envelopes.
	GetSharedObjectRecoveryDecoder(ctx context.Context) (SharedObjectRecoveryDecoder, error)
}

// SharedObjectRecoveryDecoder decrypts shared object recovery envelopes.
type SharedObjectRecoveryDecoder interface {
	// DecryptSharedObjectRecoveryEnvelope decrypts the envelope into recovery material.
	DecryptSharedObjectRecoveryEnvelope(ctx context.Context, env *SOEntityRecoveryEnvelope) (*SOEntityRecoveryMaterial, error)
}

// GetSharedObjectRecoveryProviderAccountFeature returns the SharedObjectRecoveryProvider for a ProviderAccount.
func GetSharedObjectRecoveryProviderAccountFeature(ctx context.Context, provAcc provider.ProviderAccount) (SharedObjectRecoveryProvider, error) {
	return provider.GetProviderAccountFeature[SharedObjectRecoveryProvider](
		ctx,
		provAcc,
		provider.ProviderFeature_ProviderFeature_SHARED_OBJECT_RECOVERY,
	)
}
